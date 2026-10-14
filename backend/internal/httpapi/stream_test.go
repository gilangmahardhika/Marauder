package httpapi_test

import (
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"orchestrator/backend/internal/app"
	"orchestrator/backend/internal/board"
	"orchestrator/backend/internal/httpapi"
	"orchestrator/backend/internal/notify"
	"orchestrator/backend/internal/webpush"
)

func frameCards(t *testing.T, f streamFrame) map[string]map[string]any {
	t.Helper()
	out := map[string]map[string]any{}
	cards, _ := f.data["cards"].([]any)
	for _, c := range cards {
		card := c.(map[string]any)
		out[card["id"].(string)] = card
	}
	return out
}

func frameActions(card map[string]any) []string {
	var out []string
	actions, _ := card["actions"].([]any)
	for _, a := range actions {
		out = append(out, a.(string))
	}
	return out
}

// streamMember adds a tribe member of role and returns their bearer.
func streamMember(t *testing.T, srv *httpapi.Server, email, role string) (app.User, string) {
	t.Helper()
	user := seedUser(t, srv, email, "password-long-9", role, false)
	if _, err := srv.DB.Exec(`INSERT INTO tribe_members (tribe_id, user_id) VALUES ('tribe_default', $1)`, user.ID); err != nil {
		t.Fatal(err)
	}
	return user, loginToken(t, srv, email, "password-long-9")
}

func TestStreamCards(t *testing.T) {
	srv, _, card, lead := approvalServer(t)
	_, dev := streamMember(t, srv, "dev@example.com", "developer")
	seedUser(t, srv, "outsider@example.com", "password-long-8", "lead_developer", false)
	outsider := loginToken(t, srv, "outsider@example.com", "password-long-8")

	if rec := getAuth(t, srv, "/api/stream?project=proj_default", outsider); rec.Code != 404 {
		t.Fatalf("outsider stream = %d %s", rec.Code, rec.Body.String())
	}

	leadStream := openTraceStream(t, srv, "/api/stream?project=proj_default", lead)
	if ready := leadStream.next(t); ready.kind != "ready" || ready.data["unread"] != float64(0) {
		t.Fatalf("ready frame = %+v", ready)
	}
	first := leadStream.next(t)
	if first.kind != "cards" || first.data["full"] != true {
		t.Fatalf("first frame = %+v", first)
	}
	if got := frameActions(frameCards(t, first)[card.ID]); !slices.Contains(got, "approve") {
		t.Fatalf("lead actions = %v", got)
	}
	if b := leadStream.next(t); b.kind != "board" || b.data["readyCount"] != float64(0) {
		t.Fatalf("board frame = %+v", b)
	}
	devStream := openTraceStream(t, srv, "/api/stream?project=proj_default", dev)
	if got := frameActions(frameCards(t, devStream.nextKind(t, "cards"))[card.ID]); slices.Contains(got, "approve") {
		t.Fatalf("developer actions = %v", got)
	}
	devStream.nextKind(t, "board")

	// Only the changed card is sent, and the summary when it changed.
	if _, err := srv.DB.Exec(`UPDATE cards SET title = 'Renamed' WHERE id = $1`, card.ID); err != nil {
		t.Fatal(err)
	}
	srv.Notify.Changed("proj_default")
	f := leadStream.next(t)
	if f.kind != "cards" || f.data["full"] != false || frameCards(t, f)[card.ID]["title"] != "Renamed" {
		t.Fatalf("rename frame = %+v", f)
	}
	ready, err := board.InsertCard(context.Background(), srv.DB, board.Card{Status: "ready", Title: "Next"})
	if err != nil {
		t.Fatal(err)
	}
	srv.Notify.Changed("proj_default")
	f = leadStream.next(t)
	if got := frameCards(t, f); len(got) != 1 || got[ready.ID] == nil {
		t.Fatalf("new card frame = %+v", f)
	}
	if b := leadStream.next(t); b.kind != "board" || b.data["readyCount"] != float64(1) || b.data["canRun"] != true {
		t.Fatalf("board after a ready card = %+v", b)
	}
	if _, err := srv.DB.Exec(`DELETE FROM cards WHERE id = $1`, ready.ID); err != nil {
		t.Fatal(err)
	}
	srv.Notify.Changed("proj_default")
	f = leadStream.next(t)
	if removed, _ := f.data["removed"].([]any); len(removed) != 1 || removed[0] != ready.ID {
		t.Fatalf("removed frame = %+v", f)
	}
	leadStream.nextKind(t, "board")

	srv.Notify.Changed("proj_other")
	srv.Notify.Resync()
	if f := leadStream.next(t); f.kind != "resync" {
		t.Fatalf("frame after another project's change = %+v", f)
	}
	if f := leadStream.next(t); f.kind != "cards" || f.data["full"] != true {
		t.Fatalf("frame after resync = %+v", f)
	}
}

// startNotify runs the listener and notifier serve starts, against srv.
func startNotify(t *testing.T, srv *httpapi.Server, push func(context.Context, []notify.Row)) {
	t.Helper()
	oldDebounce := notify.Debounce
	notify.Debounce = 20 * time.Millisecond
	n := &notify.Notifier{DB: srv.DB, Data: srv.Data, Push: push, Logf: t.Logf}
	l := &notify.Listener{
		DB: srv.DB, Hub: srv.Notify, Events: n.Fanout, Logf: t.Logf,
		Connect: func(ctx context.Context) (*pgx.Conn, error) {
			return pgx.Connect(ctx, os.Getenv("ORCHESTRATOR_TEST_DATABASE_URL"))
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		l.Run(ctx)
	}()
	t.Cleanup(func() {
		cancel()
		<-done
		notify.Debounce = oldDebounce
	})
}

func TestStreamNotification(t *testing.T) {
	srv, _, card, lead := approvalServer(t)
	owner, ownerToken := streamMember(t, srv, "owner@example.com", "developer")
	if _, err := srv.DB.Exec(`UPDATE cards SET owner_id = $2, title = 'Export CSV' WHERE id = $1`, card.ID, owner.ID); err != nil {
		t.Fatal(err)
	}
	pushed := make(chan notify.Row, 4)
	startNotify(t, srv, func(_ context.Context, rows []notify.Row) {
		for _, row := range rows {
			pushed <- row
		}
	})
	stream := openTraceStream(t, srv, "/api/stream", ownerToken)
	time.Sleep(300 * time.Millisecond)

	path := "/api/projects/proj_default/cards/" + card.ID + "/request-changes"
	if rec := postJSON(t, srv, path, lead, map[string]string{"reason": "secret detail in the note"}); rec.Code != 200 {
		t.Fatalf("request changes = %d %s", rec.Code, rec.Body.String())
	}
	f := stream.nextKind(t, "notification")
	n, _ := f.data["notification"].(map[string]any)
	if n["title"] != "Changes requested" || n["body"] != "Export CSV · Default" || n["cardId"] != card.ID ||
		n["url"] != "/cards/"+card.ID+"?project=proj_default" || n["readAt"] != nil || f.data["unread"] != float64(1) {
		t.Fatalf("notification frame = %+v", f.data)
	}
	select {
	case row := <-pushed:
		if row.UserID != owner.ID || row.Title != "Changes requested" {
			t.Fatalf("pushed = %+v", row)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("nothing pushed")
	}

	leadStream := openTraceStream(t, srv, "/api/stream", lead)
	if ready := leadStream.next(t); ready.kind != "ready" || ready.data["unread"] != float64(0) {
		t.Fatalf("the actor's ready frame = %+v", ready)
	}
}

func TestPushRoutes(t *testing.T) {
	srv, _, _, _ := approvalServer(t)
	user, token := streamMember(t, srv, "dev@example.com", "developer")
	if rec := getAuth(t, srv, "/api/push/key", token); rec.Code != 503 {
		t.Fatalf("push key without a key = %d", rec.Code)
	}
	key, err := webpush.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	srv.Push = &webpush.Pusher{DB: srv.DB, Sender: &webpush.Sender{Key: key}}
	srv.PushKey, _ = webpush.PublicKey(key)

	ua, _ := ecdh.P256().GenerateKey(rand.Reader)
	p256dh := base64.RawURLEncoding.EncodeToString(ua.PublicKey().Bytes())
	auth := base64.RawURLEncoding.EncodeToString(make([]byte, 16))
	sub := func(endpoint, p256dh string) map[string]any {
		return map[string]any{"endpoint": endpoint, "keys": map[string]string{"p256dh": p256dh, "auth": auth}}
	}
	if rec := postJSON(t, srv, "/api/push/subscriptions", token, sub("https://169.254.169.254/latest", p256dh)); rec.Code != 422 || !strings.Contains(rec.Body.String(), `"endpoint"`) {
		t.Fatalf("metadata endpoint = %d %s", rec.Code, rec.Body.String())
	}
	if rec := postJSON(t, srv, "/api/push/subscriptions", token, sub("https://fcm.googleapis.com/fcm/send/a", "short")); rec.Code != 422 || !strings.Contains(rec.Body.String(), `"keys"`) {
		t.Fatalf("bad keys = %d %s", rec.Code, rec.Body.String())
	}
	if rec := postJSON(t, srv, "/api/push/subscriptions", token, sub("https://fcm.googleapis.com/fcm/send/a", p256dh)); rec.Code != 204 {
		t.Fatalf("save = %d %s", rec.Code, rec.Body.String())
	}
	if n, _ := webpush.Count(context.Background(), srv.DB, user.ID); n != 1 {
		t.Fatalf("subscriptions = %d", n)
	}
	rec := getAuth(t, srv, "/api/push/key", token)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"subscriptions":1`) {
		t.Fatalf("push key = %d %s", rec.Code, rec.Body.String())
	}
	if rec := sendJSON(t, srv, http.MethodDelete, "/api/push/subscriptions", token, map[string]string{"endpoint": "https://fcm.googleapis.com/fcm/send/a"}); rec.Code != 204 {
		t.Fatalf("delete = %d %s", rec.Code, rec.Body.String())
	}
	if n, _ := webpush.Count(context.Background(), srv.DB, user.ID); n != 0 {
		t.Fatalf("subscriptions after delete = %d", n)
	}
}

func TestNotificationsRead(t *testing.T) {
	srv, _, card, _ := approvalServer(t)
	owner, ownerToken := streamMember(t, srv, "owner@example.com", "developer")
	_, otherToken := streamMember(t, srv, "other@example.com", "developer")
	insert := func(id, project, created string) {
		t.Helper()
		if _, err := srv.DB.Exec(`
			INSERT INTO notifications (id, event_id, user_id, kind, project_id, card_id, title, body, url, push, created_at)
			VALUES ($1, $1, $2, 'review', $3, $4, 'Review needed', 'card · Default', '/cards/x', true, $5)`,
			id, owner.ID, project, card.ID, created); err != nil {
			t.Fatal(err)
		}
	}
	insert("ntf_a", "proj_default", "2026-10-14T01:00:00Z")
	insert("ntf_b", "proj_default", "2026-10-14T02:00:00Z")
	insert("ntf_c", "proj_default", "2026-10-14T03:00:00Z")
	if _, err := srv.DB.Exec(`
		INSERT INTO tribes (id, name, created_at) VALUES ('tribe_x', 'X', '2026-10-14T00:00:00Z');
		INSERT INTO projects (id, tribe_id, name, repo_path, created_at) VALUES ('proj_x', 'tribe_x', 'X', '', '2026-10-14T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	insert("ntf_hidden", "proj_x", "2026-10-14T04:00:00Z")

	type listBody struct {
		Notifications []struct {
			ID     string  `json:"id"`
			ReadAt *string `json:"readAt"`
		} `json:"notifications"`
		Unread int `json:"unread"`
	}
	list := func(token, query string) listBody {
		t.Helper()
		rec := getAuth(t, srv, "/api/notifications"+query, token)
		if rec.Code != 200 {
			t.Fatalf("list = %d %s", rec.Code, rec.Body.String())
		}
		var body listBody
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		return body
	}
	ids := func(b listBody) []string {
		var out []string
		for _, n := range b.Notifications {
			out = append(out, n.ID)
		}
		return out
	}
	got := list(ownerToken, "")
	if !slices.Equal(ids(got), []string{"ntf_c", "ntf_b", "ntf_a"}) || got.Unread != 3 {
		t.Fatalf("list = %+v", got)
	}
	if other := list(otherToken, ""); len(other.Notifications) != 0 || other.Unread != 0 {
		t.Fatalf("another user's list = %+v", other)
	}
	if rec := postJSON(t, srv, "/api/notifications/read", otherToken, map[string]any{"ids": []string{"ntf_a"}}); rec.Code != 200 {
		t.Fatalf("other read = %d", rec.Code)
	}
	if rec := postJSON(t, srv, "/api/notifications/read", ownerToken, map[string]any{}); rec.Code != 422 {
		t.Fatalf("empty read = %d", rec.Code)
	}
	rec := postJSON(t, srv, "/api/notifications/read", ownerToken, map[string]any{"ids": []string{"ntf_a"}})
	if rec.Code != 200 || rec.Body.String() != "{\"unread\":2}\n" {
		t.Fatalf("read one = %d %s", rec.Code, rec.Body.String())
	}
	if unread := list(ownerToken, "?unread=1"); !slices.Equal(ids(unread), []string{"ntf_c", "ntf_b"}) {
		t.Fatalf("unread = %+v", unread)
	}
	rec = postJSON(t, srv, "/api/notifications/read", ownerToken, map[string]any{"all": true})
	if rec.Code != 200 || rec.Body.String() != "{\"unread\":0}\n" {
		t.Fatalf("read all = %d %s", rec.Code, rec.Body.String())
	}
	if all := list(ownerToken, ""); all.Unread != 0 || all.Notifications[0].ReadAt == nil {
		t.Fatalf("after read all = %+v", all)
	}
}
