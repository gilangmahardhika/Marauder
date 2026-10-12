package loop

import (
	"context"

	"orchestrator/backend/internal/live"
	"orchestrator/backend/internal/providers"
	"orchestrator/backend/internal/trace"
)

// complete sends one model request. With pub set, it publishes the turn and
// the reply text as it arrives, redacted, and a reset when the provider sends
// the reply again (RFC-088). Only reply text is published: no reasoning and no
// tool arguments. The caller calls end after the model_call event is stored;
// on an error the turn has already ended.
func complete(ctx context.Context, p providers.Provider, req providers.Request, pub live.Publisher, cardID string, turn live.Frame, secrets []string) (resp providers.Response, end func(), err error) {
	if pub == nil {
		resp, err = p.Complete(ctx, req)
		return resp, func() {}, err
	}
	frame := func(kind, text string) live.Frame {
		return live.Frame{Kind: kind, AttemptID: turn.AttemptID, Turn: turn.Turn, Text: text}
	}
	start := turn
	start.Kind, start.ModelID = live.Turn, req.Model
	pub.Publish(cardID, start)
	end = func() { pub.Publish(cardID, frame(live.TurnEnd, "")) }
	out := trace.NewStreamRedactor(secrets, func(text string) { pub.Publish(cardID, frame(live.Delta, text)) })
	switch s := p.(type) {
	case providers.RestartStreamer:
		resp, err = s.StreamRestart(ctx, req, out.Write, func() {
			out.Reset()
			pub.Publish(cardID, frame(live.Reset, ""))
		})
	case providers.Streamer:
		resp, err = s.Stream(ctx, req, out.Write)
	default:
		resp, err = p.Complete(ctx, req)
		if err == nil {
			out.Write(resp.Text)
		}
	}
	if err != nil {
		end()
		return resp, func() {}, err
	}
	out.Flush()
	return resp, end, nil
}
