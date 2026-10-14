package runner_test

import (
	"context"
	"encoding/hex"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"orchestrator/backend/internal/board"
	"orchestrator/backend/internal/live"
	"orchestrator/backend/internal/providers"
	"orchestrator/backend/internal/runner"
)

// startAttempt claims and starts the card's lease and records an attempt the
// way the loop does, asking for another model and task class.
func startAttempt(t *testing.T, _ *harness, c *runner.Client, id string) (*runner.Board, string) {
	t.Helper()
	ctx := context.Background()
	claimed := claim(t, c)
	job, err := c.Start(ctx, claimed.LeaseID)
	if err != nil || job.Done {
		t.Fatalf("start = %+v %v", job, err)
	}
	b := &runner.Board{Client: c, LeaseID: claimed.LeaseID}
	n, parent, err := b.NextAttempt(ctx, "card_other")
	if err != nil {
		t.Fatal(err)
	}
	if err := b.InsertAttempt(ctx, board.Attempt{
		ID: id, CardID: "card_other", N: n, ParentID: parent, TaskClass: "implement_hard", ModelID: "expensive-model", MaxTurns: 3,
	}); err != nil {
		t.Fatal(err)
	}
	return b, claimed.LeaseID
}

func TestModelProxyBudgets(t *testing.T) {
	h := newHarness(t, `{"implement":[{"text":"one"},{"text":"two"}]}`, "true")
	ctx := context.Background()
	c := h.register(t, "agent-1")
	b, leaseID := startAttempt(t, h, c, "att_proxy0000001")
	var model, class string
	if err := h.db.QueryRow(`SELECT model_id, task_class FROM attempts WHERE id = 'att_proxy0000001' AND card_id = $1`, h.card.ID).Scan(&model, &class); err != nil {
		t.Fatal(err)
	}
	if model != "fake-model" || class != "implement" {
		t.Fatalf("attempt route = %s %s, want the lease's fake-model implement", model, class)
	}
	p := &runner.Provider{Client: c, Board: b}
	resp, err := p.Complete(ctx, providers.Request{Model: "expensive-model", TaskClass: "implement_hard", System: "s", User: "u"})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Text != "one" {
		t.Fatalf("reply = %q", resp.Text)
	}
	if got := h.provider.seen(); !slices.Equal(got, []string{"fake-model/implement"}) {
		t.Fatalf("control plane sent %v, want the lease's route", got)
	}
	if err := b.AddAttemptTokens(ctx, "att_elsewhere", 3_000_000, 0, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Complete(ctx, providers.Request{System: "s", User: "u"}); runner.StatusOf(err) != http.StatusUnprocessableEntity {
		t.Fatalf("call over the token cap = %v, want 422", err)
	}
	if err := c.End(ctx, leaseID); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Complete(ctx, providers.Request{System: "s", User: "u"}); runner.StatusOf(err) != http.StatusNotFound {
		t.Fatalf("call on an ended lease = %v, want 404", err)
	}
	if len(h.provider.seen()) != 1 {
		t.Fatalf("refused calls reached the provider: %v", h.provider.seen())
	}
}

func TestLiveFromRunner(t *testing.T) {
	h := newHarness(t, `{"implement":[{"text":"hello from the runner"}]}`, "true")
	sub, _, err := h.srv.Live.Subscribe(h.card.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()
	c := h.register(t, "agent-1")
	b, _ := startAttempt(t, h, c, "att_live00000001")
	p := &runner.Provider{Client: c, Board: b}
	if _, err := p.Complete(context.Background(), providers.Request{System: "s", User: "u"}); err != nil {
		t.Fatal(err)
	}
	var kinds []string
	var text strings.Builder
	timeout := time.After(5 * time.Second)
	for !slices.Contains(kinds, live.TurnEnd) {
		select {
		case f, ok := <-sub.C:
			if !ok {
				t.Fatalf("stream closed after %v", kinds)
			}
			if f.AttemptID != "att_live00000001" {
				t.Fatalf("frame attempt = %q", f.AttemptID)
			}
			if f.Kind == live.Turn && f.Role != "implementer" {
				t.Fatalf("turn role = %q", f.Role)
			}
			kinds = append(kinds, f.Kind)
			if f.Kind == live.Delta {
				text.WriteString(f.Text)
			}
		case <-timeout:
			t.Fatalf("frames = %v, want turn, delta, turn_end", kinds)
		}
	}
	if kinds[0] != live.Turn || !slices.Contains(kinds, live.Delta) {
		t.Fatalf("frames = %v", kinds)
	}
	if text.String() != "hello from the runner" {
		t.Fatalf("live text = %q", text.String())
	}
}

func TestRunnerNeverGetsSecrets(t *testing.T) {
	h := newHarness(t, commitScript, "true")
	c := h.register(t, "agent-1")
	r, net := h.worker(t, c)
	claimed := claim(t, c)
	if err := r.RunJob(context.Background(), claimed.LeaseID); err != nil {
		t.Fatal(err)
	}
	card := h.status(t)
	if card.Status != "pr_open" || card.PRURL != prURL {
		t.Fatalf("card = %s %q", card.Status, card.PRURL)
	}
	if net.has("push") || net.has("gh") {
		t.Fatal("the runner pushed or called gh")
	}
	if !h.cp.has("push") {
		t.Fatal("the control plane did not push")
	}
	bodies := h.tap.all()
	if len(bodies["board"]) == 0 || len(bodies["model"]) == 0 || len(bodies["bundle"]) == 0 {
		t.Fatalf("routes seen = %v", keys(bodies))
	}
	clone := 0
	for route, list := range bodies {
		for _, body := range list {
			for _, secret := range []string{databaseURL, hex.EncodeToString(tokenKey), modelKey, ownerToken} {
				if strings.Contains(body, secret) {
					t.Fatalf("%s reply carries a secret:\n%s", route, body)
				}
			}
			if n := strings.Count(body, companyToken); n > 0 {
				if route != "start" || !strings.Contains(body, `"cloneToken":"`+companyToken+`"`) || n != 1 {
					t.Fatalf("%s reply carries the company token outside cloneToken:\n%s", route, body)
				}
				clone++
			}
		}
	}
	if clone != 1 {
		t.Fatalf("company token sent %d times, want once as cloneToken", clone)
	}
	if !strings.Contains(strings.Join(bodies["board"], "\n"), "[redacted]") {
		t.Fatal("requirement text was not redacted on the control plane")
	}
}

func keys(m map[string][]string) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}
