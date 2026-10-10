package providers

import "context"

type Tool struct {
	Name string
}

type ToolCall struct {
	Name string
	Args map[string]any
}

type Request struct {
	Model     string
	System    string
	User      string
	Tools     []Tool
	TaskClass string
}

type Response struct {
	Text         string
	ToolCalls    []ToolCall
	InputTokens  int
	OutputTokens int
	StopReason   string
}

type Provider interface {
	Complete(context.Context, Request) (Response, error)
}
