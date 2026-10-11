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
}

// Turn is one earlier model reply and the result of each of its tool calls,
// in the same order. Clients send turns as assistant and tool messages.
type Turn struct {
	Text    string
	Calls   []ToolCall
	Results []string
}

type Request struct {
	Model     string
	System    string
	User      string
	Turns     []Turn
	Tools     []Tool
	TaskClass string
	MaxTokens int
}

type Response struct {
	// Model is the model that answered, which may be the route's fallback.
	Model        string
	Text         string
	ToolCalls    []ToolCall
	InputTokens  int
	OutputTokens int
	StopReason   string
}

type Provider interface {
	Complete(context.Context, Request) (Response, error)
}

// DefaultMaxTokens caps one reply when the request names no limit.
const DefaultMaxTokens = 8192

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
