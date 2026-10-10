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
