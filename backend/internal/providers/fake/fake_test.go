package fake_test

import (
	"context"
	"testing"

	"orchestrator/backend/internal/providers"
	"orchestrator/backend/internal/providers/fake"
)

func TestFakeRecordsPrompts(t *testing.T) {
	provider, err := fake.Load([]byte(`{"implement":[{"text":"ok"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.Complete(context.Background(), providers.Request{
		TaskClass: "implement", System: "system", User: "user",
	}); err != nil {
		t.Fatal(err)
	}
	prompts := provider.Prompts()
	if len(prompts) != 1 || prompts[0].System != "system" || prompts[0].User != "user" {
		t.Fatalf("prompts = %+v", prompts)
	}
}

func TestFakeSeveralCallsAndTurns(t *testing.T) {
	provider, err := fake.Load([]byte(`{"implement":[
		{"say":"looking","calls":[{"tool":"read_file","path":"a.go"},{"tool":"read_file","path":"b.go","badArgs":"unexpected end"}]}
	]}`))
	if err != nil {
		t.Fatal(err)
	}
	turns := []providers.Turn{{Text: "earlier", Results: []string{"r"}, Note: "go on"}}
	resp, err := provider.Complete(context.Background(), providers.Request{TaskClass: "implement", Turns: turns})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Text != "looking" || len(resp.ToolCalls) != 2 || resp.ToolCalls[1].BadArgs != "unexpected end" ||
		resp.ToolCalls[0].Args["path"] != "a.go" {
		t.Fatalf("resp = %+v", resp)
	}
	turns[0].Results[0] = "changed"
	got := provider.Prompts()[0].Turns
	if len(got) != 1 || got[0].Results[0] != "r" || got[0].Note != "go on" {
		t.Fatalf("turns = %+v", got)
	}
}
