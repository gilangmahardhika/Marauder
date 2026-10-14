package notify_test

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"orchestrator/backend/internal/board"
	"orchestrator/backend/internal/board/boardtest"
	"orchestrator/backend/internal/identity"
	"orchestrator/backend/internal/notify"
	"orchestrator/backend/internal/settings"
	"orchestrator/backend/internal/trace"
)

func connect(ctx context.Context) (*pgx.Conn, error) {
	return pgx.Connect(ctx, os.Getenv("ORCHESTRATOR_TEST_DATABASE_URL"))
}

// startListener runs a listener with a short debounce and no timed scans,
// so every frame comes from a NOTIFY.
func startListener(t *testing.T, db *sql.DB, hub *notify.Hub, events func(context.Context, []notify.Event)) {
	t.Helper()
	oldDebounce, oldEvery := notify.Debounce, notify.ScanEvery
	notify.Debounce, notify.ScanEvery = 20*time.Millisecond, time.Hour
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	l := &notify.Listener{DB: db, Connect: connect, Hub: hub, Events: events, Logf: t.Logf}
	go func() {
		defer close(done)
		l.Run(ctx)
	}()
	t.Cleanup(func() {
		cancel()
		<-done
		notify.Debounce, notify.ScanEvery = oldDebounce, oldEvery
	})
}

// until repeats write every 100ms until a frame of kind arrives, since the
// listener may not have run LISTEN yet when the first write commits.
func until(t *testing.T, sub *notify.Sub, kind string, write func()) notify.Frame {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		write()
		select {
		case f := <-sub.C:
			if f.Kind == kind {
				return f
			}
		case <-time.After(100 * time.Millisecond):
		case <-deadline:
			t.Fatalf("no %s frame within 5 seconds", kind)
		}
	}
}

func next(t *testing.T, sub *notify.Sub) notify.Frame {
	t.Helper()
	select {
	case f := <-sub.C:
		return f
	case <-time.After(5 * time.Second):
		t.Fatal("no frame within 5 seconds")
	}
	return notify.Frame{}
}

func drain(sub *notify.Sub) {
	for {
		select {
		case <-sub.C:
		case <-time.After(150 * time.Millisecond):
			return
		}
	}
}

func TestNotifyTriggers(t *testing.T) {
	db := boardtest.Open(t)
	ctx := context.Background()
	hub := notify.NewHub()
	user, err := identity.Insert(ctx, db, "lead@example.com", "password-long-1", "lead_developer", false)
	if err != nil {
		t.Fatal(err)
	}
	sub, err := hub.Subscribe(user.ID, "proj_default")
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var got []notify.Event
	startListener(t, db, hub, func(_ context.Context, events []notify.Event) {
		mu.Lock()
		defer mu.Unlock()
		got = append(got, events...)
	})
	card, err := board.InsertCard(ctx, db, board.Card{Status: "ready"})
	if err != nil {
		t.Fatal(err)
	}

	f := until(t, sub, notify.Changed, func() {
		if _, err := db.Exec(`UPDATE cards SET updated_at = $2 WHERE id = $1`, card.ID, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
			t.Fatal(err)
		}
	})
	if f.ProjectID != "proj_default" {
		t.Fatalf("card update frame = %+v", f)
	}
	drain(sub)

	if _, err := db.Exec(`INSERT INTO runs (id, trigger, created_at, project_id, card_id) VALUES ('run_1', 'api', '2026-10-14T00:00:00Z', 'proj_default', $1)`, card.ID); err != nil {
		t.Fatal(err)
	}
	if f := next(t, sub); f.Kind != notify.Changed || f.ProjectID != "proj_default" {
		t.Fatalf("run frame = %+v", f)
	}
	drain(sub)

	id, err := trace.Append(ctx, db, trace.Event{CardID: card.ID, Kind: "transition", Payload: map[string]string{"from": "ready", "to": "in_progress"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if f := next(t, sub); f.Kind != notify.Changed {
		t.Fatalf("event frame = %+v", f)
	}
	mu.Lock()
	if len(got) != 1 || got[0].ID != id || got[0].ProjectID != "proj_default" || got[0].CardID != card.ID {
		t.Fatalf("events handed on = %+v", got)
	}
	mu.Unlock()
	drain(sub)

	if _, err := db.Exec(`
		INSERT INTO notifications (id, event_id, user_id, kind, project_id, card_id, title, body, url, created_at)
		VALUES ('ntf_1', $1, $2, 'review', 'proj_default', $3, 'Review needed', 'card · Default', '/cards/x', '2026-10-14T00:00:00Z')`,
		id, user.ID, card.ID); err != nil {
		t.Fatal(err)
	}
	if f := next(t, sub); f.Kind != notify.Notified || f.NotificationID != "ntf_1" {
		t.Fatalf("notification frame = %+v", f)
	}

	other, err := hub.Subscribe(user.ID, "proj_other")
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	if _, err := db.Exec(`UPDATE cards SET title = 'renamed' WHERE id = $1`, card.ID); err != nil {
		t.Fatal(err)
	}
	if f := next(t, sub); f.Kind != notify.Changed {
		t.Fatalf("rename frame = %+v", f)
	}
	select {
	case f := <-other.C:
		t.Fatalf("another project's stream got %+v", f)
	case <-time.After(200 * time.Millisecond):
	}
}

func TestNotifyCursor(t *testing.T) {
	db := boardtest.Open(t)
	ctx := context.Background()
	var got []string
	collect := func(_ context.Context, events []notify.Event) {
		for _, ev := range events {
			got = append(got, ev.ID)
		}
	}
	l := &notify.Listener{DB: db, Events: collect, Logf: t.Logf}
	appendEvent := func(ts string) string {
		t.Helper()
		id, err := trace.Append(ctx, db, trace.Event{Kind: "gate", TS: ts}, nil)
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	cursor := func() int64 {
		t.Helper()
		var seq int64
		if err := db.QueryRow(`SELECT seq FROM notify_cursor WHERE id = 1`).Scan(&seq); err != nil {
			t.Fatal(err)
		}
		return seq
	}

	first, second := appendEvent(""), appendEvent("")
	l.Scan(ctx)
	if strings.Join(got, ",") != first+","+second {
		t.Fatalf("first scan = %v", got)
	}
	var maxSeq int64
	if err := db.QueryRow(`SELECT max(seq) FROM events`).Scan(&maxSeq); err != nil {
		t.Fatal(err)
	}
	if cursor() != maxSeq {
		t.Fatalf("cursor = %d, want %d", cursor(), maxSeq)
	}
	got = nil
	l.Scan(ctx)
	if len(got) != 0 {
		t.Fatalf("second scan handed on again: %v", got)
	}

	// An event below the cursor that committed late is read within the
	// overlap; one older than the overlap is not.
	late := appendEvent("")
	old := appendEvent(time.Now().UTC().Add(-time.Hour).Format(time.RFC3339))
	if _, err := db.Exec(`UPDATE notify_cursor SET seq = seq + 1000`); err != nil {
		t.Fatal(err)
	}
	l.Scan(ctx)
	if strings.Join(got, ",") != late {
		t.Fatalf("overlap scan = %v, want only %s (old %s)", got, late, old)
	}
	got = nil
	l.Scan(ctx)
	if len(got) != 0 {
		t.Fatalf("overlap read twice: %v", got)
	}

	// The cursor never moves back.
	before := cursor()
	fresh := &notify.Listener{DB: db, Events: collect}
	fresh.Scan(ctx)
	if cursor() != before {
		t.Fatalf("cursor moved from %d to %d", before, cursor())
	}
}

type recipientsFixture struct {
	db    *sql.DB
	data  string
	users map[string]string
	card  board.Card
}

// newRecipients seeds one member of each role in proj_default's tribe, a
// developer who owns the card, a disabled lead, and a lead outside the tribe.
func newRecipients(t *testing.T) *recipientsFixture {
	t.Helper()
	db := boardtest.Open(t)
	ctx := context.Background()
	f := &recipientsFixture{db: db, data: t.TempDir(), users: map[string]string{}}
	for _, u := range []struct{ name, role string }{
		{"lead", "lead_developer"}, {"senior", "senior_developer"}, {"dev", "developer"},
		{"qalead", "qa_lead"}, {"qa", "qa"}, {"po", "product_owner"}, {"owner", "developer"},
		{"disabled", "lead_developer"}, {"outsider", "lead_developer"},
	} {
		user, err := identity.Insert(ctx, db, u.name+"@example.com", "password-long-1", u.role, false)
		if err != nil {
			t.Fatal(err)
		}
		f.users[u.name] = user.ID
		if u.name != "outsider" {
			if _, err := db.Exec(`INSERT INTO tribe_members (tribe_id, user_id) VALUES ('tribe_default', $1)`, user.ID); err != nil {
				t.Fatal(err)
			}
		}
	}
	if _, err := db.Exec(`UPDATE users SET disabled = true WHERE id = $1`, f.users["disabled"]); err != nil {
		t.Fatal(err)
	}
	card, err := board.InsertCard(ctx, db, board.Card{Status: "human_review", Title: "Export CSV", OwnerID: f.users["owner"], RequirementIDs: []string{"REQ-1"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE cards SET owner_id = $2 WHERE id = $1`, card.ID, f.users["owner"]); err != nil {
		t.Fatal(err)
	}
	card.OwnerID = f.users["owner"]
	f.card = card
	return f
}

func (f *recipientsFixture) names(notes []notify.Note) string {
	byID := map[string]string{}
	for name, id := range f.users {
		byID[id] = name
	}
	var out []string
	for _, n := range notes {
		push := ""
		if n.Push {
			push = "+push"
		}
		out = append(out, fmt.Sprintf("%s:%s:%s%s", byID[n.UserID], n.Kind, n.Title, push))
	}
	sort.Strings(out)
	return strings.Join(out, " | ")
}

func (f *recipientsFixture) recipients(t *testing.T, from, to, actor string) string {
	t.Helper()
	ev := notify.Event{ID: "evt_x", CardID: f.card.ID, ProjectID: "proj_default", ActorID: f.users[actor], Kind: "transition",
		Payload: fmt.Sprintf(`{"from":%q,"to":%q}`, from, to)}
	notes, err := notify.Recipients(context.Background(), f.db, f.data, ev, f.card)
	if err != nil {
		t.Fatal(err)
	}
	return f.names(notes)
}

func (f *recipientsFixture) setDeploy(t *testing.T, on, solo bool) {
	t.Helper()
	d := settings.Deploy{DeployEnabled: &on, SoloApproval: solo}
	if _, _, err := settings.SaveProjectDeploy(f.data, "proj_default", settings.ProjectPaths{Data: f.data}, d, nil, false); err != nil {
		t.Fatal(err)
	}
}

func TestRecipients(t *testing.T) {
	f := newRecipients(t)
	approvers := "lead:review:Review needed+push | po:review:Review needed+push | qalead:review:Review needed+push"
	if got := f.recipients(t, "agent_review", "human_review", ""); got != approvers {
		t.Fatalf("review, no reviewers = %s", got)
	}
	if got := f.recipients(t, "agent_review", "human_review", "po"); strings.Contains(got, "po:") {
		t.Fatalf("the actor is notified: %s", got)
	}

	if _, err := f.db.Exec(`
		INSERT INTO requirements (id, project_id, source_kind, source_path, heading, statement, acceptance_json, non_goals_json, content_hash)
		VALUES ('REQ-1', 'proj_default', 'draft', '', 'REQ-1', 'x', '[]', '[]', 'h')`); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"senior", "owner"} {
		if _, err := f.db.Exec(`
			INSERT INTO requirement_reviewers (project_id, requirement_id, user_id, added_by, created_at)
			VALUES ('proj_default', 'REQ-1', $1, $1, '2026-10-14T00:00:00Z')`, f.users[name]); err != nil {
			t.Fatal(err)
		}
	}
	if got := f.recipients(t, "agent_review", "human_review", ""); got != "senior:review:Review needed+push" {
		t.Fatalf("review with a reviewer = %s", got)
	}

	if got := f.recipients(t, "human_review", "approved", "lead"); got != "owner:approved:Approved" {
		t.Fatalf("approved, deploy on, by the lead = %s", got)
	}
	if got := f.recipients(t, "human_review", "approved", "po"); got != "lead:ship:Ready to deploy+push | owner:approved:Approved" {
		t.Fatalf("approved, deploy on = %s", got)
	}
	f.setDeploy(t, false, true)
	if got := f.recipients(t, "human_review", "approved", "po"); got != "lead:ship:Ready to merge+push | owner:ship:Ready to merge+push" {
		t.Fatalf("approved, deploy off, solo = %s", got)
	}
	f.setDeploy(t, true, false)

	if got := f.recipients(t, "human_review", "changes_requested", "lead"); got != "owner:changes_requested:Changes requested+push" {
		t.Fatalf("changes requested = %s", got)
	}

	f.card.Status, f.card.BlockedReason = "blocked", "deploy_failed"
	if got := f.recipients(t, "deploying_dev", "blocked", ""); got != "lead:blocked:Blocked: Deploy failed+push | owner:blocked:Blocked: Deploy failed+push" {
		t.Fatalf("blocked, deploy failed = %s", got)
	}
	f.card.BlockedReason = "owner_github"
	if got := f.recipients(t, "pr_open", "blocked", ""); got != "owner:owner_github:Connect your GitHub account+push" {
		t.Fatalf("blocked, owner github = %s", got)
	}
	f.card.BlockedReason = "tests crashed with token abc123"
	if got := f.recipients(t, "in_progress", "blocked", ""); got != "owner:blocked:Blocked+push" {
		t.Fatalf("blocked, free-text reason = %s", got)
	}
	if got := f.recipients(t, "wont_fix", "blocked", "po"); got != "owner:reopened:Reopened" {
		t.Fatalf("reopened = %s", got)
	}
	if got := f.recipients(t, "blocked", "wont_fix", "owner"); got != "" {
		t.Fatalf("won't fix by the owner = %s", got)
	}
	if got := f.recipients(t, "blocked", "wont_fix", "po"); got != "owner:wont_fix:Closed as won't fix" {
		t.Fatalf("won't fix = %s", got)
	}
	if got := f.recipients(t, "verifying", "done", ""); got != "owner:done:Done" {
		t.Fatalf("done = %s", got)
	}
	if got := f.recipients(t, "ready", "in_progress", ""); got != "" {
		t.Fatalf("in progress = %s", got)
	}

	ev := notify.Event{ID: "evt_y", CardID: f.card.ID, ProjectID: "proj_default", Kind: "review_failed", Payload: `{}`}
	notes, err := notify.Recipients(context.Background(), f.db, f.data, ev, f.card)
	if err != nil {
		t.Fatal(err)
	}
	want := "dev:review_failed:AI review gave no verdict+push | lead:review_failed:AI review gave no verdict+push | " +
		"owner:review_failed:AI review gave no verdict+push | qa:review_failed:AI review gave no verdict+push | " +
		"senior:review_failed:AI review gave no verdict+push"
	if got := f.names(notes); got != want {
		t.Fatalf("review failed = %s", got)
	}
}

func TestFanoutOnce(t *testing.T) {
	f := newRecipients(t)
	ctx := context.Background()
	if _, err := f.db.Exec(`UPDATE cards SET status = 'changes_requested' WHERE id = $1`, f.card.ID); err != nil {
		t.Fatal(err)
	}
	id, err := trace.Append(ctx, f.db, trace.Event{CardID: f.card.ID, ActorID: f.users["lead"], Kind: "transition",
		Payload: map[string]string{"from": "human_review", "to": "changes_requested"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	var pushed []notify.Row
	n := &notify.Notifier{DB: f.db, Data: f.data, Push: func(_ context.Context, rows []notify.Row) { pushed = append(pushed, rows...) }}
	ev := notify.Event{ID: id, CardID: f.card.ID, ProjectID: "proj_default", ActorID: f.users["lead"], Kind: "transition",
		Payload: `{"from":"human_review","to":"changes_requested"}`}
	n.Fanout(ctx, []notify.Event{ev})
	n.Fanout(ctx, []notify.Event{ev})
	if len(pushed) != 1 || pushed[0].UserID != f.users["owner"] || pushed[0].Title != "Changes requested" {
		t.Fatalf("pushed = %+v", pushed)
	}
	row := pushed[0]
	if row.Body != "Export CSV · Default" || row.URL != "/cards/"+f.card.ID+"?project=proj_default" {
		t.Fatalf("row = %+v", row)
	}
	var count int
	if err := f.db.QueryRow(`SELECT count(*) FROM notifications WHERE event_id = $1`, id).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("rows = %d", count)
	}
}
