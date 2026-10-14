package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"orchestrator/backend/internal/providers"
	"orchestrator/backend/internal/providers/llm"
	"orchestrator/backend/internal/router"
	"orchestrator/backend/internal/settings"
	"orchestrator/backend/internal/trace"
)

// ErrUnknownFamily means the LLM settings have no family with that id.
var ErrUnknownFamily = errors.New("unknown LLM family")

const llmTestTimeout = 20 * time.Second

// TestLLM sends one short request to the family's first model. The returned
// error is safe to show: it never contains the key.
func TestLLM(ctx context.Context, data, familyID string) error {
	cfg, secrets, err := settings.Load(data)
	if err != nil {
		return err
	}
	for _, family := range cfg.LLM.Families {
		if family.ID != familyID {
			continue
		}
		if len(family.Models) == 0 {
			return fmt.Errorf("family %s has no models", familyID)
		}
		key := secrets.LLMKeys[family.ID]
		p, err := llm.ForFamily(family, key)
		if err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(ctx, llmTestTimeout)
		defer cancel()
		_, err = p.Complete(ctx, llm.WithOptions(providers.Request{
			Model:     family.Models[0].ID,
			System:    "Reply with the single word OK.",
			User:      "ping",
			MaxTokens: 16,
			TaskClass: "test",
		}, family.Models[0].ModelOptions))
		if err != nil {
			return errors.New(trace.Redact(err.Error(), []string{key}))
		}
		return nil
	}
	return ErrUnknownFamily
}

// RoutedProvider sends each request to primary, retries it once, then sends it
// to fallback (router.Call). A rate-limited call waits as long as the server
// asked, up to rateLimitWait, and is sent again before it counts as a failure.
// The error after both models fail wraps ErrProvider.
func RoutedProvider(cfg settings.Config, secrets settings.Secrets, primary, fallback string) providers.Provider {
	return routedProvider{cfg: cfg, secrets: secrets, primary: primary, fallback: fallback}
}

const (
	rateLimitWait    = time.Minute
	rateLimitDefault = 10 * time.Second
)

type routedProvider struct {
	cfg      settings.Config
	secrets  settings.Secrets
	primary  string
	fallback string
}

func (p routedProvider) Complete(ctx context.Context, req providers.Request) (providers.Response, error) {
	var resp providers.Response
	err := router.Call(p.primary, p.fallback, func(model string) error {
		client, _, _, err := llm.ForModel(p.cfg, p.secrets, model)
		if err != nil {
			return err
		}
		next := llm.WithOptions(req, llm.Options(p.cfg, model))
		next.Model = model
		resp, err = client.Complete(ctx, next)
		var limited *providers.RateLimited
		if errors.As(err, &limited) {
			if werr := sleepCtx(ctx, rateWait(limited.Wait)); werr != nil {
				return werr
			}
			resp, err = client.Complete(ctx, next)
		}
		resp.Model = model
		return noRetryFull(err)
	})
	if err != nil && ctx.Err() == nil {
		return resp, providerErr(err)
	}
	return resp, err
}

// Stream is Complete for a streaming reply. The rate-limit wait, the retry,
// and the fallback apply only until the first text reaches onText; after that
// an error ends the request (RFC-087).
func (p routedProvider) Stream(ctx context.Context, req providers.Request, onText func(string)) (providers.Response, error) {
	return p.streamFrom(ctx, req, nil, onText)
}

// streamFrom is Stream that also calls onStart with the answering model, once,
// right before its first text.
func (p routedProvider) streamFrom(ctx context.Context, req providers.Request, onStart func(model string), onText func(string)) (providers.Response, error) {
	var resp providers.Response
	sent := false
	err := router.Call(p.primary, p.fallback, func(model string) error {
		client, _, _, err := llm.ForModel(p.cfg, p.secrets, model)
		if err != nil {
			return err
		}
		next := llm.WithOptions(req, llm.Options(p.cfg, model))
		next.Model = model
		forward := func(text string) {
			if !sent {
				sent = true
				if onStart != nil {
					onStart(model)
				}
			}
			onText(text)
		}
		resp, err = stream(ctx, client, next, forward)
		var limited *providers.RateLimited
		if !sent && errors.As(err, &limited) {
			if werr := sleepCtx(ctx, rateWait(limited.Wait)); werr != nil {
				return werr
			}
			resp, err = stream(ctx, client, next, forward)
		}
		resp.Model = model
		if err != nil && sent {
			return router.ErrNoRetry{Err: err}
		}
		return noRetryFull(err)
	})
	if err != nil && ctx.Err() == nil {
		return resp, providerErr(err)
	}
	return resp, err
}

// StreamRestart is Complete for a streaming reply that keeps the rate-limit
// wait, the retry, and the fallback after text was sent. Before a reply is
// sent again after text, it calls onRestart (RFC-088).
func (p routedProvider) StreamRestart(ctx context.Context, req providers.Request, onText func(string), onRestart func()) (providers.Response, error) {
	var resp providers.Response
	sent := false
	send := func(client providers.Provider, next providers.Request) (providers.Response, error) {
		if sent {
			sent = false
			if onRestart != nil {
				onRestart()
			}
		}
		return stream(ctx, client, next, func(text string) {
			sent = true
			onText(text)
		})
	}
	err := router.Call(p.primary, p.fallback, func(model string) error {
		client, _, _, err := llm.ForModel(p.cfg, p.secrets, model)
		if err != nil {
			return err
		}
		next := llm.WithOptions(req, llm.Options(p.cfg, model))
		next.Model = model
		resp, err = send(client, next)
		var limited *providers.RateLimited
		if errors.As(err, &limited) {
			if werr := sleepCtx(ctx, rateWait(limited.Wait)); werr != nil {
				return werr
			}
			resp, err = send(client, next)
		}
		resp.Model = model
		return noRetryFull(err)
	})
	if err != nil && ctx.Err() == nil {
		return resp, providerErr(err)
	}
	return resp, err
}

// noRetryFull stops the retry and the fallback for a full context: the same
// request would not fit again, and the loop compacts instead (RFC-117).
func noRetryFull(err error) error {
	if providers.IsContextFull(err) {
		return router.ErrNoRetry{Err: err}
	}
	return err
}

// providerErr is the error after both models failed. A full context keeps
// providers.ErrContextFull so the loop can compact.
func providerErr(err error) error {
	msg := strings.TrimPrefix(err.Error(), "provider_error: ")
	if providers.IsContextFull(err) {
		return fmt.Errorf("%w: %w: %s", ErrProvider, providers.ErrContextFull, msg)
	}
	return fmt.Errorf("%w: %s", ErrProvider, msg)
}

// stream sends req through the client's Stream, or through Complete as one
// piece of text for a client that cannot stream.
func stream(ctx context.Context, client providers.Provider, req providers.Request, onText func(string)) (providers.Response, error) {
	if s, ok := client.(providers.Streamer); ok {
		return s.Stream(ctx, req, onText)
	}
	resp, err := client.Complete(ctx, req)
	if err == nil && resp.Text != "" {
		onText(resp.Text)
	}
	return resp, err
}

func rateWait(asked time.Duration) time.Duration {
	if asked <= 0 {
		return rateLimitDefault
	}
	return min(asked, rateLimitWait)
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// modelKeys lists every LLM key so job errors and events can redact them.
func modelKeys(secrets settings.Secrets) []string {
	var out []string
	for _, key := range secrets.LLMKeys {
		if key != "" {
			out = append(out, key)
		}
	}
	return out
}
