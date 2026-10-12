package fake

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"orchestrator/backend/internal/providers"
)

type Prompt struct {
	TaskClass string
	System    string
	User      string
}

type turn struct {
	text  string
	tool  string
	args  map[string]any
	error string
}

type Fake struct {
	mu      sync.Mutex
	queues  map[string][]turn
	prompts []Prompt
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

func (f *Fake) Complete(_ context.Context, req providers.Request) (providers.Response, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.prompts = append(f.prompts, Prompt{TaskClass: req.TaskClass, System: req.System, User: req.User})
	queue := f.queues[req.TaskClass]
	if len(queue) == 0 {
		return providers.Response{StopReason: "stop"}, nil
	}
	next := queue[0]
	f.queues[req.TaskClass] = queue[1:]
	if next.error != "" {
		return providers.Response{}, fmt.Errorf("%s", next.error)
	}
	if next.tool == "" {
		return providers.Response{Text: next.text, StopReason: "stop"}, nil
	}
	return providers.Response{
		Text:       next.text,
		ToolCalls:  []providers.ToolCall{{Name: next.tool, Args: next.args}},
		StopReason: "tool_use",
	}, nil
}

// turnFrom reads one scripted turn. A tool turn's "say" is the reply text the
// model writes beside the call; every other key is a tool argument.
func turnFrom(fields map[string]any) turn {
	var t turn
	if errText, ok := fields["error"].(string); ok && errText != "" {
		t.error = errText
		return t
	}
	if name, ok := fields["tool"].(string); ok && name != "" {
		t.tool = name
		t.args = map[string]any{}
		for key, value := range fields {
			if key == "say" {
				t.text, _ = value.(string)
				continue
			}
			if key == "tool" {
				continue
			}
			t.args[key] = value
		}
		return t
	}
	if text, ok := fields["text"].(string); ok {
		t.text = text
	}
	return t
}
