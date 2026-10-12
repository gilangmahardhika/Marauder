package fake

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"unicode"

	"orchestrator/backend/internal/providers"
)

// Prompt is one request the fake received. Turns is a copy of the request's
// turns, so a test can check what the model was sent back.
type Prompt struct {
	TaskClass string
	System    string
	User      string
	MaxTokens int
	Turns     []providers.Turn
	Tools     []string
}

type call struct {
	tool    string
	args    map[string]any
	badArgs string
}

type turn struct {
	text  string
	calls []call
	error string
	stop  string
	// streamError fails a Stream after its first after pieces of text.
	streamError string
	after       int
	// hold keeps a Stream open after its text until the context ends.
	hold bool
	// chunks, when set, are the pieces a Stream sends instead of words.
	chunks []string
}

type Fake struct {
	mu       sync.Mutex
	queues   map[string][]turn
	prompts  []Prompt
	canceled int
	// Usage is added to every response, so a test can reach a token cap.
	Usage providers.Response
}

func Load(data []byte) (*Fake, error) {
	var raw map[string][]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	queues := make(map[string][]turn, len(raw))
	for class, items := range raw {
		for _, item := range items {
			var fields map[string]any
			if err := json.Unmarshal(item, &fields); err != nil {
				return nil, err
			}
			queues[class] = append(queues[class], turnFrom(fields))
		}
	}
	return &Fake{queues: queues}, nil
}

func (f *Fake) Prompts() []Prompt {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]Prompt, len(f.prompts))
	copy(out, f.prompts)
	return out
}

// Canceled counts the streams that ended because their context did.
func (f *Fake) Canceled() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.canceled
}

func (f *Fake) Complete(_ context.Context, req providers.Request) (providers.Response, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	next, ok := f.pop(req)
	if !ok {
		return f.respond(turn{}), nil
	}
	if next.error != "" {
		return providers.Response{}, fmt.Errorf("%s", next.error)
	}
	return f.respond(next), nil
}

// Stream replies like Complete and sends the reply text through onText one
// word at a time, each with the whitespace after it.
func (f *Fake) Stream(ctx context.Context, req providers.Request, onText func(string)) (providers.Response, error) {
	f.mu.Lock()
	next, ok := f.pop(req)
	resp := f.respond(next)
	f.mu.Unlock()
	if ok && next.error != "" {
		return providers.Response{}, fmt.Errorf("%s", next.error)
	}
	pieces := next.chunks
	if pieces == nil {
		pieces = words(resp.Text)
	}
	for i, piece := range pieces {
		if next.streamError != "" && i == next.after {
			return providers.Response{}, fmt.Errorf("%s", next.streamError)
		}
		if err := ctx.Err(); err != nil {
			f.cancel()
			return providers.Response{}, err
		}
		onText(piece)
	}
	if next.streamError != "" {
		return providers.Response{}, fmt.Errorf("%s", next.streamError)
	}
	if next.hold {
		<-ctx.Done()
		f.cancel()
		return providers.Response{}, ctx.Err()
	}
	return resp, nil
}

func (f *Fake) cancel() {
	f.mu.Lock()
	f.canceled++
	f.mu.Unlock()
}

// pop records req and takes the next scripted turn of its task class.
func (f *Fake) pop(req providers.Request) (turn, bool) {
	prompt := Prompt{TaskClass: req.TaskClass, System: req.System, User: req.User, MaxTokens: req.MaxTokens, Turns: copyTurns(req.Turns)}
	for _, tool := range req.Tools {
		prompt.Tools = append(prompt.Tools, tool.Name)
	}
	f.prompts = append(f.prompts, prompt)
	queue := f.queues[req.TaskClass]
	if len(queue) == 0 {
		return turn{}, false
	}
	f.queues[req.TaskClass] = queue[1:]
	return queue[0], true
}

func (f *Fake) respond(next turn) providers.Response {
	usage := providers.Response{
		InputTokens: f.Usage.InputTokens, OutputTokens: f.Usage.OutputTokens, CachedInputTokens: f.Usage.CachedInputTokens,
	}
	if next.stop != "" {
		usage.Text = next.text
		usage.StopReason = next.stop
		return usage
	}
	usage.Text = next.text
	if len(next.calls) == 0 {
		usage.StopReason = "stop"
		return usage
	}
	for _, c := range next.calls {
		usage.ToolCalls = append(usage.ToolCalls, providers.ToolCall{Name: c.tool, Args: c.args, BadArgs: c.badArgs})
	}
	usage.StopReason = "tool_use"
	return usage
}

// words splits text after each run of whitespace, so the pieces join back to
// text.
func words(text string) []string {
	var out []string
	start := 0
	runes := []rune(text)
	for i := 1; i < len(runes); i++ {
		if unicode.IsSpace(runes[i-1]) && !unicode.IsSpace(runes[i]) {
			out = append(out, string(runes[start:i]))
			start = i
		}
	}
	if start < len(runes) {
		out = append(out, string(runes[start:]))
	}
	return out
}

func copyTurns(turns []providers.Turn) []providers.Turn {
	out := make([]providers.Turn, len(turns))
	for i, t := range turns {
		out[i] = providers.Turn{
			Text: t.Text, Note: t.Note, Reasoning: t.Reasoning,
			Calls:   append([]providers.ToolCall(nil), t.Calls...),
			Results: append([]string(nil), t.Results...),
		}
	}
	return out
}

// turnFrom reads one scripted turn. A tool turn's "say" is the reply text the
// model writes beside the call; every other key is a tool argument, except
// "badArgs", which stands for arguments that were not valid JSON. "calls" holds
// several tool calls of one reply, each shaped like a tool turn. A "stop" turn
// returns that stop reason, such as a reply cut at max_tokens, with its "text".
// A "streamError" turn streams its "text" and fails after "after" words; a
// "hold" turn streams its "text" and stays open until the request ends. A
// "chunks" turn streams those pieces as given, and its text is their join.
func turnFrom(fields map[string]any) turn {
	var t turn
	if list, ok := fields["chunks"].([]any); ok {
		for _, item := range list {
			piece, _ := item.(string)
			t.chunks = append(t.chunks, piece)
		}
		t.text = strings.Join(t.chunks, "")
		return t
	}
	if errText, ok := fields["streamError"].(string); ok && errText != "" {
		t.streamError = errText
		t.text, _ = fields["text"].(string)
		if after, ok := fields["after"].(float64); ok {
			t.after = int(after)
		}
		return t
	}
	if hold, ok := fields["hold"].(bool); ok && hold {
		t.hold = true
		t.text, _ = fields["text"].(string)
		return t
	}
	if errText, ok := fields["error"].(string); ok && errText != "" {
		t.error = errText
		return t
	}
	if stop, ok := fields["stop"].(string); ok && stop != "" {
		t.stop = stop
		t.text, _ = fields["text"].(string)
		return t
	}
	if list, ok := fields["calls"].([]any); ok {
		t.text, _ = fields["say"].(string)
		for _, item := range list {
			if m, ok := item.(map[string]any); ok {
				if c, ok := callFrom(m); ok {
					t.calls = append(t.calls, c)
				}
			}
		}
		return t
	}
	if c, ok := callFrom(fields); ok {
		t.text, _ = fields["say"].(string)
		t.calls = []call{c}
		return t
	}
	if text, ok := fields["text"].(string); ok {
		t.text = text
	}
	return t
}

func callFrom(fields map[string]any) (call, bool) {
	name, ok := fields["tool"].(string)
	if !ok || name == "" {
		return call{}, false
	}
	c := call{tool: name, args: map[string]any{}}
	for key, value := range fields {
		switch key {
		case "say", "tool":
		case "badArgs":
			c.badArgs, _ = value.(string)
		default:
			c.args[key] = value
		}
	}
	return c, true
}
