package loop

import (
	"context"
	"log"

	"orchestrator/backend/internal/live"
	"orchestrator/backend/internal/providers"
	"orchestrator/backend/internal/trace"
)

// CompleteLive sends one model request of a remote attempt and publishes its
// turn to pub as a local attempt does, ending the turn when the reply is in
// (RFC-108).
func CompleteLive(ctx context.Context, p providers.Provider, req providers.Request, pub live.Publisher, cardID string, turn live.Frame, secrets []string) (providers.Response, error) {
	resp, end, err := complete(ctx, p, req, pub, cardID, turn, secrets, nil)
	end()
	return resp, err
}

// complete sends one model request. With pub set, it publishes the turn and
// the reply text as it arrives, redacted, and a reset when the provider sends
// the reply again (RFC-088). Thinking deltas are published and passed to
// logf when set. Tool arguments are never published. The caller calls end
// after the model_call event is stored; on an error the turn has already ended.
func complete(ctx context.Context, p providers.Provider, req providers.Request, pub live.Publisher, cardID string, turn live.Frame, secrets []string, logf func(string, ...any)) (resp providers.Response, end func(), err error) {
	frame := func(kind, text string) live.Frame {
		return live.Frame{Kind: kind, AttemptID: turn.AttemptID, Turn: turn.Turn, Text: text}
	}
	publish := func(kind, text string) {
		if text == "" {
			return
		}
		if pub != nil {
			pub.Publish(cardID, frame(kind, text))
		}
		if kind == live.Thinking {
			if logf != nil {
				logf("thinking: %s", text)
			} else {
				log.Printf("thinking: %s", text)
			}
		}
	}
	if pub == nil && logf == nil {
		resp, err = p.Complete(ctx, req)
		if err == nil && logf != nil {
			logThinking(logf, resp.Reasoning, secrets)
		}
		return resp, func() {}, err
	}
	if pub != nil {
		start := turn
		start.Kind, start.ModelID = live.Turn, req.Model
		pub.Publish(cardID, start)
		end = func() { pub.Publish(cardID, frame(live.TurnEnd, "")) }
	} else {
		end = func() {}
	}
	out := trace.NewStreamRedactor(secrets, func(text string) { publish(live.Delta, text) })
	streamedThinking := false
	think := trace.NewStreamRedactor(secrets, func(text string) {
		streamedThinking = true
		publish(live.Thinking, text)
	})
	switch s := p.(type) {
	case providers.ThinkingStreamer:
		resp, err = s.StreamWithThinking(ctx, req, out.Write, think.Write)
	case providers.RestartStreamer:
		resp, err = s.StreamRestart(ctx, req, out.Write, func() {
			out.Reset()
			think.Reset()
			if pub != nil {
				pub.Publish(cardID, frame(live.Reset, ""))
			}
		})
	case providers.Streamer:
		resp, err = s.Stream(ctx, req, out.Write)
	default:
		resp, err = p.Complete(ctx, req)
		if err == nil {
			out.Write(resp.Text)
			for _, r := range resp.Reasoning {
				if r.Text != "" {
					think.Write(r.Text)
				}
			}
		}
	}
	if err != nil {
		end()
		return resp, func() {}, err
	}
	out.Flush()
	think.Flush()
	if logf != nil && !streamedThinking {
		logThinking(logf, resp.Reasoning, secrets)
	}
	return resp, end, nil
}

func logThinking(logf func(string, ...any), reasoning []providers.Reasoning, secrets []string) {
	for _, r := range reasoning {
		if r.Kind == "redacted_thinking" || r.Text == "" {
			continue
		}
		logf("thinking: %s", trace.Redact(r.Text, secrets))
	}
}
