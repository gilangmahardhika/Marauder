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
		return err
	})
	if err != nil && ctx.Err() == nil {
		return resp, fmt.Errorf("%w: %s", ErrProvider, strings.TrimPrefix(err.Error(), "provider_error: "))
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
