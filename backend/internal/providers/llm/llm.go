// Package llm builds the provider client for a model from the LLM settings.
package llm

import (
	"errors"
	"fmt"
	"net"
	"net/url"

	"orchestrator/backend/internal/providers"
	"orchestrator/backend/internal/providers/anthropic"
	"orchestrator/backend/internal/providers/openai"
	"orchestrator/backend/internal/settings"
)

var (
	ErrUnknownModel = errors.New("model is not in any LLM family")
	ErrNoKey        = errors.New("LLM family has no API key")
	ErrProtocol     = errors.New("LLM family protocol is not anthropic or openai")
)

// ForModel finds the family that lists model and returns its client and key.
func ForModel(cfg settings.Config, secrets settings.Secrets, model string) (providers.Provider, settings.Family, string, error) {
	for _, family := range cfg.LLM.Families {
		for _, m := range family.Models {
			if m.ID != model {
				continue
			}
			key := secrets.LLMKeys[family.ID]
			p, err := ForFamily(family, key)
			return p, family, key, err
		}
	}
	return nil, settings.Family{}, "", fmt.Errorf("%w: %s", ErrUnknownModel, model)
}

// Options are the thinking and effort settings saved for model, or none.
func Options(cfg settings.Config, model string) settings.ModelOptions {
	for _, family := range cfg.LLM.Families {
		for _, m := range family.Models {
			if m.ID == model {
				return m.ModelOptions
			}
		}
	}
	return settings.ModelOptions{}
}

// WithOptions sets the request's thinking and effort from opts (RFC-079).
func WithOptions(req providers.Request, opts settings.ModelOptions) providers.Request {
	req.Thinking, req.ThinkingBudget, req.Effort = opts.Thinking, opts.ThinkingBudget, opts.Effort
	return req
}

// ForFamily picks the client by protocol. A family without a key is allowed
// only when its baseUrl is a loopback server.
func ForFamily(family settings.Family, key string) (providers.Provider, error) {
	if key == "" && !Loopback(family.BaseURL) {
		return nil, fmt.Errorf("%w: %s", ErrNoKey, family.ID)
	}
	switch family.Protocol {
	case "openai":
		return openai.New(family.BaseURL, key), nil
	case "anthropic":
		return anthropic.New(family.BaseURL, key), nil
	default:
		return nil, fmt.Errorf("%w: %s", ErrProtocol, family.ID)
	}
}

func Loopback(baseURL string) bool {
	if baseURL == "" {
		return false
	}
	u, err := url.Parse(baseURL)
	if err != nil {
		return false
	}
	host := u.Hostname()
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
