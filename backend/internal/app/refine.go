package app

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"orchestrator/backend/internal/contextpack"
	"orchestrator/backend/internal/intake"
	"orchestrator/backend/internal/loop"
	"orchestrator/backend/internal/providers"
	"orchestrator/backend/internal/providers/llm"
	"orchestrator/backend/internal/settings"
	"orchestrator/backend/internal/trace"
)

var (
	// ErrRefineRoute means the implement route has no usable model (RFC-086).
	ErrRefineRoute = errors.New("set a model for the implement route in Router settings to refine requirements")
	// ErrRefineProposal means the model's finished text fails the save rules.
	ErrRefineProposal = errors.New("the model did not return a usable refinement")
	// ErrRefineProvider means the request failed on both models or timed out.
	ErrRefineProvider = errors.New("the refine request to the model failed")
)

const (
	refineTimeout   = 2 * time.Minute
	refineMaxTokens = 4096
	maxRefineGuide  = 1000
	// refineMaxTurns and refineMaxCalls bound a refine with MCP tools (RFC-095).
	refineMaxTurns = 3
	refineMaxCalls = 3
)

// RefineTools are what a refine needs to offer the tribe's MCP tools: the
// token key that opens their secrets, the dialer, and the service secrets to
// redact (databaseUrl and tokenKey).
type RefineTools struct {
	Key     []byte
	MCP     MCPDialer
	Secrets []string
}

// RefineInput is the draft and the field to refine. RequirementID is empty for
// a new requirement.
type RefineInput struct {
	Field         string
	RequirementID string
	Statement     string
	Acceptance    []string
	Instructions  string
}

// RefineDone is the finished text of one field. It is never saved here.
// Acceptance holds the cleaned lines when Field is acceptance.
type RefineDone struct {
	Text       string   `json:"text"`
	Acceptance []string `json:"acceptance,omitempty"`
	ModelID    string   `json:"modelId"`
}

// modelStreamer is a provider that also names the model that answers, right
// before its first text.
type modelStreamer interface {
	streamFrom(ctx context.Context, req providers.Request, onStart func(model string), onText func(string)) (providers.Response, error)
}

// RefineField asks the implement route's model to rewrite one field of a draft
// and streams the text as it arrives (RFC-087). emit gets "start" with the
// modelId before the first text, a "delta" per piece of redacted text, then
// "done" with a RefineDone. Errors return before any emit for bad input or a
// missing route; a provider error, a cancel, or text that fails the save rules
// returns after. It writes no requirement, only a requirement_refined event
// without text. provider, when set, replaces the routed provider in tests.
//
// When the project's tribe has MCP tools, and the draft's mentions leave any
// (RFC-102), the model may call them for at most 3 turns and 3 calls, and the
// last turn asks for text (RFC-095). A turn that ends in tool calls emits
// "reset", which clears its text, and each call emits "tool" with the server
// and tool.
func RefineField(ctx context.Context, data string, db *sql.DB, projectID, actorID string, in RefineInput, provider providers.Provider, tools RefineTools, emit func(kind string, payload any)) error {
	if in.Field != "statement" && in.Field != "acceptance" {
		return &intake.Invalid{Fields: map[string]string{"field": "Choose statement or acceptance to refine."}}
	}
	draft, err := intake.Validate(intake.Draft{Statement: in.Statement, Acceptance: in.Acceptance})
	if err != nil {
		return err
	}
	if utf8.RuneCountInString(in.Instructions) > maxRefineGuide {
		return &intake.Invalid{Fields: map[string]string{
			"instructions": fmt.Sprintf("Keep what to improve to %d characters or fewer.", maxRefineGuide),
		}}
	}
	cfg, secrets, err := settings.Load(data)
	if err != nil {
		return err
	}
	route := cfg.Router.Routes["implement"]
	if route.Model == "" {
		return ErrRefineRoute
	}
	if provider == nil {
		if _, _, _, err := llm.ForModel(cfg, secrets, route.Model); err != nil {
			return ErrRefineRoute
		}
		provider = RoutedProvider(cfg, secrets, route.Model, route.Fallback)
	}
	system := contextpack.RefineStatementSystem
	if in.Field == "acceptance" {
		system = contextpack.RefineAcceptanceSystem
	}
	req := providers.Request{
		Model:     route.Model,
		System:    system,
		User:      contextpack.RefineUser(in.Field, in.RequirementID, draft.Statement, draft.Acceptance, in.Instructions),
		MaxTokens: refineMaxTokens,
		TaskClass: "refine_requirement",
	}

	redact := append(modelKeys(secrets), tools.Secrets...)
	conns, err := LoadProjectMCP(ctx, db, tools.Key, projectID)
	if err != nil {
		return err
	}
	redact = append(redact, MCPSecrets(conns)...)
	conns, _, _ = ScopeMentions(conns, intake.MentionsOf(draft.Statement, draft.Acceptance))

	modelID := route.Model
	started := false
	start := func(model string) {
		if !started {
			started = true
			modelID = model
			emit("start", map[string]string{"modelId": model})
		}
	}
	ctx, cancel := context.WithTimeout(ctx, refineTimeout)
	defer cancel()

	toolset, err := OpenMCPToolset(ctx, db, tools.MCP, conns, MCPScope{ProjectID: projectID}, redact)
	if err != nil {
		return err
	}
	defer toolset.Close()
	allowed := map[string]providers.Tool{}
	maxTurns := 1
	if toolset != nil && len(toolset.Tools()) > 0 {
		maxTurns = refineMaxTurns
		for _, tool := range toolset.Tools() {
			allowed[tool.Name] = tool
		}
	}

	var resp, total providers.Response
	calls := 0
	for turn := 0; turn < maxTurns; turn++ {
		if maxTurns > 1 {
			req.Tools = toolset.Tools()
			req.ToolChoice = ""
			if turn == maxTurns-1 || calls >= refineMaxCalls {
				req.ToolChoice = "none"
			}
		}
		out := trace.NewStreamRedactor(redact, func(text string) {
			start(modelID)
			emit("delta", map[string]string{"text": text})
		})
		resp, err = refineSend(ctx, provider, req, start, out, &modelID)
		if err != nil {
			if parent := context.Cause(ctx); errors.Is(parent, context.Canceled) {
				return parent
			}
			return fmt.Errorf("%w: %s", ErrRefineProvider, trace.Redact(err.Error(), redact))
		}
		total.InputTokens += resp.InputTokens
		total.OutputTokens += resp.OutputTokens
		total.CachedInputTokens += resp.CachedInputTokens
		if resp.Model != "" {
			modelID = resp.Model
		}
		if maxTurns == 1 || len(resp.ToolCalls) == 0 || req.ToolChoice == "none" {
			out.Flush()
			break
		}
		out.Reset()
		start(modelID)
		emit("reset", map[string]any{})
		results := make([]string, len(resp.ToolCalls))
		for i, call := range resp.ToolCalls {
			results[i], err = refineCall(ctx, db, projectID, actorID, toolset, allowed, call, &calls, redact, emit)
			if err != nil {
				return err
			}
		}
		if parent := context.Cause(ctx); errors.Is(parent, context.Canceled) {
			return parent
		}
		req.Turns = append(req.Turns, providers.Turn{
			Text: trace.Redact(resp.Text, redact), Calls: resp.ToolCalls, Results: results, Reasoning: resp.Reasoning,
		})
	}
	if parent := context.Cause(ctx); errors.Is(parent, context.Canceled) {
		return parent
	}
	start(modelID)

	requirementID := in.RequirementID
	if requirementID == "" {
		requirementID = "new"
	}
	_, _ = trace.Append(ctx, db, trace.Event{
		Kind: "requirement_refined", ActorID: actorID, ProjectID: projectID,
		Payload: map[string]any{
			"requirementId": requirementID, "field": in.Field, "modelId": modelID,
			"inputTokens": total.InputTokens, "outputTokens": total.OutputTokens, "cachedInputTokens": total.CachedInputTokens,
		},
	}, redact)

	done, err := finished(in.Field, draft, trace.Redact(resp.Text, redact))
	if err != nil {
		return err
	}
	done.ModelID = modelID
	emit("done", done)
	return nil
}

// refineSend sends one turn and streams its text into out.
func refineSend(ctx context.Context, provider providers.Provider, req providers.Request, start func(string), out *trace.StreamRedactor, modelID *string) (providers.Response, error) {
	switch p := provider.(type) {
	case modelStreamer:
		return p.streamFrom(ctx, req, start, out.Write)
	case providers.Streamer:
		return p.Stream(ctx, req, func(text string) {
			start(*modelID)
			out.Write(text)
		})
	default:
		resp, err := p.Complete(ctx, req)
		if err == nil {
			out.Write(resp.Text)
		}
		return resp, err
	}
}

// refineCall runs one MCP call of a refine and returns the text the model
// sees. It appends a project tool_call event with no card and no result
// text. Only a failed event write is an error.
func refineCall(ctx context.Context, db *sql.DB, projectID, actorID string, toolset *MCPToolset, allowed map[string]providers.Tool,
	call providers.ToolCall, calls *int, redact []string, emit func(string, any)) (string, error) {
	tool, ok := allowed[call.Name]
	var result string
	rejected := false
	switch {
	case !ok:
		result, rejected = providers.ErrorPrefix+"not in profile", true
	case *calls >= refineMaxCalls:
		result, rejected = fmt.Sprintf("%srefine may call MCP tools at most %d times", providers.ErrorPrefix, refineMaxCalls), true
	case call.BadArgs != "":
		result, rejected = providers.ErrorPrefix+"the arguments are not a JSON object", true
	default:
		*calls++
		slug, name, _ := loop.SplitMCPName(call.Name)
		emit("tool", map[string]string{"server": slug, "tool": name})
		text, err := loop.CallMCP(ctx, toolset, tool, call.Args, redact)
		if err != nil {
			text = providers.ErrorPrefix + err.Error()
		}
		result = text
		ok = err == nil
	}
	sum := sha256.Sum256([]byte(result))
	payload := map[string]any{
		"name": trace.Clip(call.Name, redact, 128), "ok": ok && !rejected, "rejected": rejected,
		"bytes": len(result), "sha256": hex.EncodeToString(sum[:]),
	}
	if !rejected {
		payload["args"] = loop.MCPArgs(call.Name, call.Args)
	}
	if _, err := trace.Append(context.WithoutCancel(ctx), db, trace.Event{
		Kind: "tool_call", ActorID: actorID, ProjectID: projectID, Payload: payload,
	}, redact); err != nil {
		return "", err
	}
	return result, nil
}

// finished checks the model's whole text for field with the save rules.
func finished(field string, draft intake.Draft, text string) (RefineDone, error) {
	check := intake.Draft{Statement: text, Acceptance: draft.Acceptance}
	if field == "acceptance" {
		var lines []string
		for _, line := range strings.Split(text, "\n") {
			if strings.TrimSpace(line) != "" {
				lines = append(lines, line)
			}
		}
		if len(lines) == 0 {
			return RefineDone{}, fmt.Errorf("%w: the reply has no acceptance lines", ErrRefineProposal)
		}
		check = intake.Draft{Statement: draft.Statement, Acceptance: lines}
	}
	clean, err := intake.Validate(check)
	if err != nil {
		var invalid *intake.Invalid
		if errors.As(err, &invalid) {
			var msgs []string
			for _, msg := range invalid.Fields {
				msgs = append(msgs, msg)
			}
			sort.Strings(msgs)
			return RefineDone{}, fmt.Errorf("%w: %s", ErrRefineProposal, strings.Join(msgs, " "))
		}
		return RefineDone{}, err
	}
	if field == "acceptance" {
		return RefineDone{Text: strings.Join(clean.Acceptance, "\n"), Acceptance: clean.Acceptance}, nil
	}
	return RefineDone{Text: clean.Statement}, nil
}
