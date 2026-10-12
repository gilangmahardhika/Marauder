package live_test

import (
	"strings"
	"testing"

	"orchestrator/backend/internal/live"
)

func drain(c <-chan live.Frame) []live.Frame {
	var out []live.Frame
	for {
		select {
		case f, ok := <-c:
			if !ok {
				return out
			}
			out = append(out, f)
		default:
			return out
		}
	}
}

func TestHubSnapshotAndSlowSubscriber(t *testing.T) {
	hub := live.New()
	hub.Publish("card_1", live.Frame{Kind: live.Delta, AttemptID: "att_1", Turn: 0, Text: "ignored before a turn"})
	hub.Publish("card_1", live.Frame{Kind: live.Turn, AttemptID: "att_1", Turn: 2, Role: "implementer", ModelID: "m"})
	hub.Publish("card_1", live.Frame{Kind: live.Delta, AttemptID: "att_1", Turn: 2, Text: "Reading "})
	hub.Publish("card_1", live.Frame{Kind: live.Delta, AttemptID: "att_1", Turn: 2, Text: "the file."})

	sub, snapshot, err := hub.Subscribe("card_1")
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot) != 2 || snapshot[0].Kind != live.Turn || snapshot[0].ModelID != "m" || snapshot[1].Text != "Reading the file." {
		t.Fatalf("snapshot = %+v", snapshot)
	}
	hub.Publish("card_1", live.Frame{Kind: live.Reset, AttemptID: "att_1", Turn: 2})
	hub.Publish("card_1", live.Frame{Kind: live.Delta, AttemptID: "att_1", Turn: 2, Text: "Again."})
	hub.Publish("card_2", live.Frame{Kind: live.Turn, AttemptID: "att_9", Turn: 0})
	got := drain(sub.C)
	if len(got) != 2 || got[0].Kind != live.Reset || got[1].Text != "Again." {
		t.Fatalf("frames = %+v", got)
	}
	_, snapshot, _ = hub.Subscribe("card_1")
	if snapshot[1].Text != "Again." {
		t.Fatalf("snapshot after reset = %+v", snapshot)
	}
	hub.Publish("card_1", live.Frame{Kind: live.TurnEnd, AttemptID: "att_1", Turn: 2})
	if _, snapshot, _ = hub.Subscribe("card_1"); len(snapshot) != 0 {
		t.Fatalf("snapshot after turn_end = %+v", snapshot)
	}

	slow, _, _ := hub.Subscribe("card_3")
	hub.Publish("card_3", live.Frame{Kind: live.Turn, AttemptID: "att_3", Turn: 0})
	for i := 0; i <= live.Buffer; i++ {
		hub.Publish("card_3", live.Frame{Kind: live.Delta, AttemptID: "att_3", Turn: 0, Text: "x"})
	}
	if frames := drain(slow.C); len(frames) != live.Buffer || !slow.Dropped() {
		t.Fatalf("slow viewer got %d frames, dropped %v", len(frames), slow.Dropped())
	}
	if _, ok := <-slow.C; ok {
		t.Fatal("a dropped viewer's channel stays open")
	}

	big := strings.Repeat("é", live.MaxText)
	long, _, _ := hub.Subscribe("card_4")
	hub.Publish("card_4", live.Frame{Kind: live.Turn, AttemptID: "att_4", Turn: 0})
	hub.Publish("card_4", live.Frame{Kind: live.Delta, AttemptID: "att_4", Turn: 0, Text: big})
	hub.Publish("card_4", live.Frame{Kind: live.Delta, AttemptID: "att_4", Turn: 0, Text: "more"})
	frames := drain(long.C)
	if len(frames) != 2 || len(frames[1].Text) > live.MaxText || !strings.HasSuffix(frames[1].Text, "é") {
		t.Fatalf("capped text: %d frames, %d bytes", len(frames), len(frames[len(frames)-1].Text))
	}

	hub.Close()
	drain(sub.C)
	if _, ok := <-sub.C; ok {
		t.Fatal("Close left a viewer open")
	}
	if _, _, err := hub.Subscribe("card_1"); err != live.ErrFull {
		t.Fatalf("subscribe after close = %v", err)
	}
	var none *live.Hub
	none.Publish("card_1", live.Frame{Kind: live.Turn})
}

func TestHubCaps(t *testing.T) {
	hub := live.New()
	for i := 0; i < live.PerCard; i++ {
		if _, _, err := hub.Subscribe("card_1"); err != nil {
			t.Fatal(err)
		}
	}
	if _, _, err := hub.Subscribe("card_1"); err != live.ErrFull {
		t.Fatalf("over the card cap = %v", err)
	}
	subs := []*live.Sub{}
	for i := live.PerCard; i < live.Total; i++ {
		s, _, err := hub.Subscribe("card_" + strings.Repeat("x", i))
		if err != nil {
			t.Fatal(err)
		}
		subs = append(subs, s)
	}
	if _, _, err := hub.Subscribe("card_other"); err != live.ErrFull {
		t.Fatalf("over the total cap = %v", err)
	}
	subs[0].Close()
	subs[0].Close()
	if _, _, err := hub.Subscribe("card_other"); err != nil {
		t.Fatalf("after a close = %v", err)
	}
}
