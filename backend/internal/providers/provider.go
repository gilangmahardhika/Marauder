package providers

import (
	"context"
	"net/http"
	"strconv"
	"time"
)

// Tool is one function the model may call. Parameters is a JSON schema object.
type Tool struct {
	Name        string
	Description string
	Parameters  map[string]any
}

type ToolCall struct {
	ID   string
	Name string
	Args map[string]any
	// BadArgs is the parse error of arguments that were not valid JSON. Args
	// is then empty, and the call goes back to the model as a tool error.
	BadArgs string
}

// ErrorPrefix starts a tool result that reports an error. The anthropic
// client sends such a result with is_error.
const ErrorPrefix = "error: "

// Turn is one earlier model reply and the result of each of its tool calls,
// in the same order. Clients send turns as assistant and tool messages. Note
// is harness text sent after the results as a user message: a continue nudge,
// the todo list, or the turns left.
type Turn struct {
	Text      string
	Calls     []ToolCall
	Results   []string
	Reasoning []Reasoning
	Note      string
}

// Reasoning is one piece of a reply's hidden reasoning, kept in memory so the
// next request of the same attempt can send it back to the model that wrote it.
// Kind is thinking or redacted_thinking on the anthropic protocol, and
// reasoning_content on the openai protocol. It is never stored.
type Reasoning struct {
	Model     string
	Kind      string
	Text      string
	Signature string
	Data      string
}

type Request struct {
	Model     string
	System    string
	User      string
	Turns     []Turn
	Tools     []Tool
	TaskClass string
	MaxTokens int
	// Thinking, ThinkingBudget, and Effort are the model's settings (RFC-079).
	// Empty values send nothing.
	Thinking       string
	ThinkingBudget int
	Effort         string
}

type Response struct {
	// Model is the model that answered, which may be the route's fallback.
	Model     string
	Text      string
	ToolCalls []ToolCall
	// InputTokens excludes cached reads, which are CachedInputTokens.
	InputTokens       int
	OutputTokens      int
	CachedInputTokens int
	StopReason        string
	Reasoning         []Reasoning
}

type Provider interface {
	Complete(context.Context, Request) (Response, error)
}

// Streamer is a provider that can hand back reply text as it arrives. onText
// gets each piece in order; the Response is the whole reply.
type Streamer interface {
	Stream(ctx context.Context, req Request, onText func(string)) (Response, error)
}

// DefaultMaxTokens caps one reply when the request names no limit.
const DefaultMaxTokens = 8192

// DefaultThinkingBudget is the thinking budget of a model with thinking
// enabled and no budget of its own.
const DefaultThinkingBudget = 8192

// Replay is the reasoning of turn to send back with a request to model: none
// when another model answered that turn.
func Replay(turn Turn, model string) []Reasoning {
	var out []Reasoning
	for _, r := range turn.Reasoning {
		if r.Model == model {
			out = append(out, r)
		}
	}
	return out
}

// Truncated reports a reply cut at the token limit: max_tokens on the
// anthropic protocol, length on the openai protocol.
func Truncated(stopReason string) bool {
	return stopReason == "max_tokens" || stopReason == "length"
}

// RateLimited is a 429 reply. Wait is how long the server asked the caller to
// wait, or zero when it did not say.
type RateLimited struct {
	Message string
	Wait    time.Duration
}

func (e *RateLimited) Error() string { return e.Message }

// RetryWait reads retry-after-ms, then retry-after in seconds or as an HTTP date.
func RetryWait(header http.Header) time.Duration {
	if header == nil {
		return 0
	}
	if ms, err := strconv.ParseFloat(header.Get("Retry-After-Ms"), 64); err == nil && ms > 0 {
		return time.Duration(ms * float64(time.Millisecond))
	}
	value := header.Get("Retry-After")
	if secs, err := strconv.ParseFloat(value, 64); err == nil && secs > 0 {
		return time.Duration(secs * float64(time.Second))
	}
	if at, err := http.ParseTime(value); err == nil {
		if wait := time.Until(at); wait > 0 {
			return wait
		}
	}
	return 0
}
