package httpapi_test

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"orchestrator/backend/internal/board"
	"orchestrator/backend/internal/httpapi"
	"orchestrator/backend/internal/live"
	"orchestrator/backend/internal/loop"
	"orchestrator/backend/internal/providers/fake"
	"orchestrator/backend/internal/trace"
)

type streamFrame struct {
	kind string
	data map[string]any
}

type traceStream struct {
	resp   *http.Response
	frames chan streamFrame
	pings  atomic.Int32
}

// openTraceStream connects to path on a real listener, so the test reads the
// frames as the handler flushes them.
func openTraceStream(t *testing.T, srv *httpapi.Server, path, token string) *traceStream {
	t.Helper()
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Host = "127.0.0.1:8787"
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	s := &traceStream{resp: resp, frames: make(chan streamFrame, 64)}
	go func() {
		defer close(s.frames)
		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 1<<20), 1<<20)
		kind := ""
		for scanner.Scan() {
			line := scanner.Text()
			switch {
			case strings.HasPrefix(line, ": ping"):
				s.pings.Add(1)
			case strings.HasPrefix(line, "event: "):
				kind = strings.TrimPrefix(line, "event: ")
			case strings.HasPrefix(line, "data: "):
				var data map[string]any
				_ = json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &data)
				s.frames <- streamFrame{kind: kind, data: data}
			}
		}
	}()
	return s
}

func (s *traceStream) next(t *testing.T) streamFrame {
	t.Helper()
	select {
	case f, ok := <-s.frames:
		if !ok {
			t.Fatal("the stream ended")
		}
		return f
	case <-time.After(5 * time.Second):
		t.Fatal("no frame within 5 seconds")
	}
	return streamFrame{}
}

// nextKind skips frames until one of kind arrives.
func (s *traceStream) nextKind(t *testing.T, kind string) streamFrame {
	t.Helper()
	for {
		if f := s.next(t); f.kind == kind {
			return f
		}
	}
}

func (s *traceStream) ended(t *testing.T) {
	t.Helper()
	select {
	case f, ok := <-s.frames:
		if ok {
			t.Fatalf("frame after the end: %+v", f)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the stream stayed open")
	}
}

func eventIDs(f streamFrame) []string {
	var ids []string
	events, _ := f.data["events"].([]any)
	for _, ev := range events {
		if m, ok := ev.(map[string]any); ok {
			ids = append(ids, m["id"].(string))
		}
	}
	return ids
}

func openAttempt(t *testing.T, srv *httpapi.Server, cardID, id string) {
	t.Helper()
	if err := board.InsertAttempt(context.Background(), srv.DB, board.Attempt{ID: id, CardID: cardID, N: 1, TaskClass: "implement", ModelID: "model-a", MaxTurns: 8}); err != nil {
		t.Fatal(err)
	}
}

func appendEvent(t *testing.T, srv *httpapi.Server, ev trace.Event) {
	t.Helper()
	if _, err := trace.Append(context.Background(), srv.DB, ev, nil); err != nil {
		t.Fatal(err)
	}
}

func TestTraceStreamFrames(t *testing.T) {
	httpapi.SetTraceStreamTiming(t, 50*time.Millisecond, 30*time.Millisecond, time.Hour)
	srv, _, card, token := approvalServer(t)
	openAttempt(t, srv, card.ID, "att_stream000000")
	appendEvent(t, srv, trace.Event{ID: "evt_first0000000", CardID: card.ID, Kind: "transition", Payload: map[string]any{"from": "ready", "to": "in_progress"}})
	srv.Live.Publish(card.ID, live.Frame{Kind: live.Turn, AttemptID: "att_stream000000", Turn: 0, Role: "implementer", ModelID: "model-a"})
	srv.Live.Publish(card.ID, live.Frame{Kind: live.Delta, AttemptID: "att_stream000000", Turn: 0, Text: "Reading the "})

	s := openTraceStream(t, srv, "/api/projects/proj_default/cards/"+card.ID+"/trace/stream", token)
	if s.resp.StatusCode != 200 || s.resp.Header.Get("Content-Type") != "text/event-stream" ||
		s.resp.Header.Get("Cache-Control") != "no-store" || s.resp.Header.Get("X-Accel-Buffering") != "no" {
		t.Fatalf("stream = %d %v", s.resp.StatusCode, s.resp.Header)
	}
	first := s.next(t)
	if first.kind != "trace" || first.data["live"] != true || len(eventIDs(first)) != 1 || len(first.data["attempts"].([]any)) != 1 {
		t.Fatalf("first frame = %+v", first)
	}
	if turn := s.next(t); turn.kind != "turn" || turn.data["role"] != "implementer" || turn.data["modelId"] != "model-a" || turn.data["attemptId"] != "att_stream000000" {
		t.Fatalf("snapshot turn = %+v", turn)
	}
	if delta := s.next(t); delta.kind != "delta" || delta.data["text"] != "Reading the " {
		t.Fatalf("snapshot delta = %+v", delta)
	}

	srv.Live.Publish(card.ID, live.Frame{Kind: live.Delta, AttemptID: "att_stream000000", Turn: 0, Text: "file."})
	if delta := s.nextKind(t, "delta"); delta.data["text"] != "file." {
		t.Fatalf("live delta = %+v", delta)
	}
	appendEvent(t, srv, trace.Event{CardID: card.ID, AttemptID: "att_stream000000", Kind: "model_call", Payload: map[string]any{"role": "implementer", "text": "Reading the file."}})
	srv.Live.Publish(card.ID, live.Frame{Kind: live.TurnEnd, AttemptID: "att_stream000000", Turn: 0})
	s.nextKind(t, "turn_end")
	stored := s.nextKind(t, "trace")
	if ids := eventIDs(stored); len(ids) != 1 || ids[0] == "evt_first0000000" {
		t.Fatalf("events after turn_end = %v, want only the model call", ids)
	}
	if s.pings.Load() == 0 {
		time.Sleep(100 * time.Millisecond)
	}
	if s.pings.Load() == 0 {
		t.Fatal("no ping comment")
	}

	if err := board.FinishAttempt(context.Background(), srv.DB, "att_stream000000", "checks_passed", headSHA); err != nil {
		t.Fatal(err)
	}
	if last := s.nextKind(t, "trace"); last.data["live"] != false {
		t.Fatalf("trace after the attempt ended = %+v", last)
	}
	if done := s.next(t); done.kind != "done" || done.data["live"] != false {
		t.Fatalf("done = %+v", done)
	}
	s.ended(t)

	idle := openTraceStream(t, srv, "/api/projects/proj_default/cards/"+card.ID+"/trace/stream", token)
	if f := idle.next(t); f.kind != "trace" || f.data["live"] != false {
		t.Fatalf("idle trace = %+v", f)
	}
	if done := idle.next(t); done.kind != "done" || done.data["live"] != false {
		t.Fatalf("idle done = %+v", done)
	}
	idle.ended(t)
}

func TestTraceStreamReconnectAndShutdown(t *testing.T) {
	httpapi.SetTraceStreamTiming(t, time.Hour, time.Hour, 100*time.Millisecond)
	srv, _, card, token := approvalServer(t)
	openAttempt(t, srv, card.ID, "att_reconnect000")
	path := "/api/projects/proj_default/cards/" + card.ID + "/trace/stream"
	s := openTraceStream(t, srv, path, token)
	s.nextKind(t, "trace")
	if done := s.next(t); done.kind != "done" || done.data["reconnect"] != true {
		t.Fatalf("done = %+v", done)
	}
	s.ended(t)

	httpapi.SetTraceStreamTiming(t, time.Hour, time.Hour, time.Hour)
	s = openTraceStream(t, srv, path, token)
	s.nextKind(t, "trace")
	srv.Live.Close()
	s.ended(t)
}

func TestTraceStreamRedactsSplitSecret(t *testing.T) {
	httpapi.SetTraceStreamTiming(t, time.Hour, time.Hour, time.Hour)
	srv, _, card, token := approvalServer(t)
	openAttempt(t, srv, card.ID, "att_redact000000")
	secret := "sk-live-0123456789abcdef"
	s := openTraceStream(t, srv, "/api/projects/proj_default/cards/"+card.ID+"/trace/stream", token)
	s.nextKind(t, "trace")

	provider, err := fake.Load([]byte(`{"review":[{"chunks":["The key is sk-li","ve-0123","456789abcdef, so ","rotate it."]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	card.Status = "agent_review"
	_, err = loop.Review(context.Background(), loop.ReviewInput{
		DB: srv.DB, Card: card, AttemptID: "att_redact000000", Model: "fake-model", Provider: provider,
		Diff: "diff --git a/x b/x\n", Secrets: []string{secret}, Live: srv.Live,
		Comment: func(context.Context, string) error { return nil },
	})
	if err != loop.ErrReviewParse {
		t.Fatalf("review = %v", err)
	}
	text := ""
	for {
		f := s.next(t)
		if f.kind == "turn_end" {
			break
		}
		if f.kind == "delta" {
			piece := f.data["text"].(string)
			for i := 5; i <= len(secret); i++ {
				if strings.Contains(piece, secret[:i]) {
					t.Fatalf("delta %q carries part of the secret", piece)
				}
			}
			text += piece
		}
	}
	if text != "The key is [redacted], so rotate it." {
		t.Fatalf("streamed text = %q", text)
	}
}

func TestTraceStreamOtherProcessEvents(t *testing.T) {
	httpapi.SetTraceStreamTiming(t, 50*time.Millisecond, time.Hour, time.Hour)
	srv, _, card, token := approvalServer(t)
	openAttempt(t, srv, card.ID, "att_other0000000")
	appendEvent(t, srv, trace.Event{ID: "evt_before000000", CardID: card.ID, Kind: "transition"})
	s := openTraceStream(t, srv, "/api/projects/proj_default/cards/"+card.ID+"/trace/stream", token)
	if ids := eventIDs(s.nextKind(t, "trace")); len(ids) != 1 {
		t.Fatalf("first events = %v", ids)
	}
	appendEvent(t, srv, trace.Event{ID: "evt_runnext00000", CardID: card.ID, AttemptID: "att_other0000000", Kind: "tool_call",
		Payload: map[string]any{"name": "read_file", "ok": true}})
	for {
		f := s.nextKind(t, "trace")
		ids := eventIDs(f)
		if len(ids) == 0 {
			continue
		}
		if len(ids) != 1 || ids[0] != "evt_runnext00000" {
			t.Fatalf("events = %v, want only the new event", ids)
		}
		break
	}
}

func TestTraceStreamRequiresMember(t *testing.T) {
	httpapi.SetTraceStreamTiming(t, time.Hour, time.Hour, time.Hour)
	srv, _, card, _ := approvalServer(t)
	if _, err := srv.DB.Exec(`
		INSERT INTO projects (id, tribe_id, name, repo_path, created_at)
		VALUES ('proj_other', 'tribe_default', 'Other', '/tmp/other', $1)`, time.Now().UTC().Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
	seedUser(t, srv, "outsider@example.com", "password-long-2", "developer", false)
	outsider := loginToken(t, srv, "outsider@example.com", "password-long-2")
	lead := loginToken(t, srv, "lead@example.com", "password-long-1")
	path := "/api/projects/proj_default/cards/" + card.ID + "/trace/stream"
	if rec := getAuth(t, srv, path, ""); rec.Code != 401 {
		t.Fatalf("no bearer = %d", rec.Code)
	}
	if rec := getAuth(t, srv, path, outsider); rec.Code != 404 && rec.Code != 403 {
		t.Fatalf("outsider = %d %s", rec.Code, rec.Body.String())
	}
	if rec := getAuth(t, srv, "/api/projects/proj_other/cards/"+card.ID+"/trace/stream", lead); rec.Code != 404 {
		t.Fatalf("another project = %d %s", rec.Code, rec.Body.String())
	}
	if rec := getAuth(t, srv, path+"?since=yesterday", lead); rec.Code != 422 {
		t.Fatalf("bad since = %d %s", rec.Code, rec.Body.String())
	}
	if rec := getAuth(t, srv, path, lead); rec.Code != 200 || !strings.Contains(rec.Body.String(), "event: done") {
		t.Fatalf("member = %d %s", rec.Code, rec.Body.String())
	}
}

func TestTraceStreamCap(t *testing.T) {
	srv, _, card, token := approvalServer(t)
	for range live.PerCard {
		if _, _, err := srv.Live.Subscribe(card.ID); err != nil {
			t.Fatal(err)
		}
	}
	rec := getAuth(t, srv, "/api/projects/proj_default/cards/"+card.ID+"/trace/stream", token)
	if rec.Code != 429 || !strings.Contains(rec.Body.String(), "rate_limited") {
		t.Fatalf("over the cap = %d %s", rec.Code, rec.Body.String())
	}
}
