package loop

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"orchestrator/backend/internal/board"
	"orchestrator/backend/internal/contextpack"
	"orchestrator/backend/internal/live"
	"orchestrator/backend/internal/providers"
	"orchestrator/backend/internal/trace"
)

// ErrReviewParse means the reviewer gave no valid verdict: it stopped without
// one, or sent an invalid one twice (RFC-065).
var ErrReviewParse = errors.New("review_parse")

// ReviewMaxTokens is the reply limit of a reviewer request. A model that thinks
// before it answers can spend a smaller limit without calling verdict (RFC-075).
const ReviewMaxTokens = 32 << 10

// ErrReviewCut is a no-verdict review whose replies were cut at the token limit
// twice in a row. It is an ErrReviewParse.
var ErrReviewCut = fmt.Errorf("%w: the reply hit the %d-token limit before a verdict", ErrReviewParse, ReviewMaxTokens)

const reviewTurns = 8

// ReviewDiffCap is the most diff text a review pack carries.
const ReviewDiffCap = 64 << 10

func ReviewerTools() []providers.Tool {
	return []providers.Tool{
		{
			Name:        "read_diff",
			Description: "Read the pull request diff from the base branch to the reviewed commit.",
			Parameters:  object(map[string]any{}),
		},
		{
			Name:        "comment_pr",
			Description: "Post a comment on the pull request.",
			Parameters:  object(map[string]any{"body": str("Markdown comment")}, "body"),
		},
		{
			Name:        "verdict",
			Description: "Finish the review. verdict is recommend_approve or request_changes. body explains it; for request_changes it is what the implementer must fix.",
			Parameters: object(map[string]any{
				"verdict": map[string]any{"type": "string", "enum": []string{"recommend_approve", "request_changes"}},
				"body":    str("Reason for the verdict"),
			}, "verdict", "body"),
		},
	}
}

// ReviewInput is one review of the card's pull request at Head.
type ReviewInput struct {
	DB          *sql.DB
	Card        board.Card
	AttemptID   string
	Model       string
	Provider    providers.Provider
	Diff        string
	CheckOutput string
	PRBody      string
	Comments    string
	Comment     func(ctx context.Context, body string) error
	Secrets     []string
	// Live, when set, gets each turn's reply text as the model writes it.
	Live live.Publisher
}

// ReviewResult is the reviewer's verdict and the hash of the prompt it saw.
type ReviewResult struct {
	Verdict    string
	Body       string
	PromptHash string
}

// ReviewPack builds the reviewer prompt from the card's requirements, rules and
// skills, the diff, the gating check output, and the pull request text.
func ReviewPack(ctx context.Context, in ReviewInput) (system, user string, err error) {
	statements, err := statements(ctx, in.DB, in.Card.ProjectID, in.Card.RequirementIDs)
	if err != nil {
		return "", "", err
	}
	convs, _, err := CardConventions(ctx, in.DB, in.Card.ProjectID, in.Card.ID)
	if err != nil {
		return "", "", err
	}
	system, user, _, err = contextpack.BuildReviewerPack(contextpack.ReviewerInput{
		Requirements: statements, Diff: in.Diff, CheckOutput: in.CheckOutput,
		PRBody: in.PRBody, Comments: in.Comments, Conventions: convs, Secrets: in.Secrets,
	})
	return system, user, err
}

// Review runs the reviewer tool loop until it calls verdict with a valid
// value. It never moves the card.
func Review(ctx context.Context, in ReviewInput) (ReviewResult, error) {
	system, user, err := ReviewPack(ctx, in)
	if err != nil {
		return ReviewResult{}, err
	}
	result := ReviewResult{PromptHash: hashText(user)}
	var turns []providers.Turn
	invalid := 0
	cut := false
	for turn := 0; turn < reviewTurns; turn++ {
		resp, endTurn, err := complete(ctx, in.Provider, providers.Request{
			Model: in.Model, System: system, User: user, Turns: turns,
			Tools: ReviewerTools(), TaskClass: "review", MaxTokens: ReviewMaxTokens,
		}, in.Live, in.Card.ID, live.Frame{AttemptID: in.AttemptID, Turn: turn, Role: "reviewer"}, in.Secrets)
		if err != nil {
			return result, err
		}
		modelID := resp.Model
		if modelID == "" {
			modelID = in.Model
		}
		err = reviewEvent(ctx, in, "model_call", modelCallPayload("reviewer", modelID, resp, in.Secrets))
		endTurn()
		if err != nil {
			return result, err
		}
		if len(resp.ToolCalls) == 0 {
			if !providers.Truncated(resp.StopReason) {
				return result, ErrReviewParse
			}
			if cut {
				return result, ErrReviewCut
			}
			cut = true
			continue
		}
		done := providers.Turn{Text: resp.Text, Reasoning: resp.Reasoning}
		for i, call := range resp.ToolCalls {
			if call.ID == "" {
				call.ID = fmt.Sprintf("call_%d_%d", turn, i)
			}
			summary, rejected := "", false
			switch call.Name {
			case "read_diff":
				summary = in.Diff
				if len(summary) > minResultCap {
					summary = summary[:minResultCap] + "\ntruncated: true"
				}
			case "comment_pr":
				body := trace.Redact(strArg(call.Args, "body"), in.Secrets)
				if body == "" {
					summary = "body is empty"
				} else if err := in.Comment(ctx, body); err != nil {
					summary = "comment failed: " + err.Error()
				} else {
					summary = "commented"
				}
			case "verdict":
				verdict := strArg(call.Args, "verdict")
				if verdict == "recommend_approve" || verdict == "request_changes" {
					result.Verdict = verdict
					result.Body = trace.Redact(strArg(call.Args, "body"), in.Secrets)
					summary = verdict
				} else {
					invalid++
					summary = "verdict must be recommend_approve or request_changes"
				}
			default:
				summary, rejected = "not in profile", true
			}
			payload := map[string]any{"name": call.Name, "ok": !rejected, "rejected": rejected, "summary": summary}
			if args := toolArgs(call, "", in.Secrets); len(args) > 0 {
				payload["args"] = args
			}
			if err := reviewEvent(ctx, in, "tool_call", payload); err != nil {
				return result, err
			}
			if result.Verdict != "" {
				return result, nil
			}
			if invalid >= 2 {
				return result, ErrReviewParse
			}
			done.Calls = append(done.Calls, call)
			done.Results = append(done.Results, summary)
		}
		turns = append(turns, done)
	}
	return result, ErrReviewParse
}

func reviewEvent(ctx context.Context, in ReviewInput, kind string, payload map[string]any) error {
	_, err := trace.Append(ctx, in.DB, trace.Event{
		CardID: in.Card.ID, AttemptID: in.AttemptID, ProjectID: in.Card.ProjectID, Kind: kind, Payload: payload,
	}, in.Secrets)
	return err
}
