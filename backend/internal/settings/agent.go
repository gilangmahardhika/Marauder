package settings

import (
	"net/url"
	"path/filepath"
	"strings"
)

const (
	AgentBuiltin    = "builtin"
	AgentClaudeCode = "claude_code"

	AuthAuto   = "auto"
	AuthAPIKey = "apiKey"
	AuthBearer = "bearer"

	// MaxAgentInstructions is the most extra system prompt an admin may save.
	MaxAgentInstructions = 8 << 10
)

// AgentConfig is agent in the service config.yaml. Kind is builtin, or
// claude_code to run the Claude Code CLI (RFC-120, RFC-121).
type AgentConfig struct {
	Kind       string            `yaml:"kind,omitempty" json:"kind"`
	ClaudeCode ClaudeCodeOptions `yaml:"claudeCode,omitempty" json:"claudeCode"`
}

// ClaudeCodeOptions are the Claude Code settings. The key is never here: it
// is the family's key in secrets.json.
type ClaudeCodeOptions struct {
	Family       string `yaml:"family,omitempty" json:"family"`
	Model        string `yaml:"model,omitempty" json:"model"`
	SmallModel   string `yaml:"smallModel,omitempty" json:"smallModel"`
	MaxTurns     int    `yaml:"maxTurns,omitempty" json:"maxTurns"`
	Auth         string `yaml:"auth,omitempty" json:"auth"`
	AllowWeb     bool   `yaml:"allowWeb,omitempty" json:"allowWeb"`
	Instructions string `yaml:"instructions,omitempty" json:"instructions"`
	Binary       string `yaml:"binary,omitempty" json:"binary"`
}

// AgentKind is the company's implement agent.
func (c Config) AgentKind() string {
	if c.Agent.Kind == AgentClaudeCode {
		return AgentClaudeCode
	}
	return AgentBuiltin
}

// Family returns the saved family with id.
func (c Config) Family(id string) (Family, bool) {
	for _, family := range c.LLM.Families {
		if family.ID == id {
			return family, true
		}
	}
	return Family{}, false
}

// ResolveAuth is the auth mode Claude Code uses with a family: auto sends
// x-api-key to Anthropic itself and a bearer token to any other base URL.
func ResolveAuth(mode, baseURL string) string {
	switch mode {
	case AuthAPIKey, AuthBearer:
		return mode
	}
	if baseURL == "" {
		return AuthAPIKey
	}
	if u, err := url.Parse(baseURL); err == nil && strings.EqualFold(u.Hostname(), "api.anthropic.com") {
		return AuthAPIKey
	}
	return AuthBearer
}

// SaveAgent stores agent in the service config. A claude_code agent needs an
// anthropic-protocol family with a key and one of its models.
func SaveAgent(data string, agent AgentConfig) (Config, error) {
	current, secrets, err := Load(data)
	if err != nil {
		return current, err
	}
	switch agent.Kind {
	case "", AgentBuiltin:
		agent.Kind = AgentBuiltin
	case AgentClaudeCode:
		if fields := checkClaudeCode(current, secrets, agent.ClaudeCode); len(fields) > 0 {
			return current, &Invalid{Fields: fields}
		}
	default:
		return current, &Invalid{Fields: map[string]string{"kind": "the agent must be builtin or claude_code"}}
	}
	current.Agent = agent
	if err := write(data, current, secrets); err != nil {
		return current, err
	}
	return current, nil
}

func checkClaudeCode(cfg Config, secrets Secrets, cc ClaudeCodeOptions) map[string]string {
	fields := map[string]string{}
	family, ok := cfg.Family(cc.Family)
	switch {
	case cc.Family == "":
		fields["claudeCode.family"] = "choose a model family"
	case !ok:
		fields["claudeCode.family"] = "family " + cc.Family + " is not saved in LLM APIs"
	case family.Protocol != "anthropic":
		fields["claudeCode.family"] = "Claude Code needs a family with the anthropic protocol"
	case secrets.LLMKeys[family.ID] == "":
		fields["claudeCode.family"] = "family " + family.ID + " has no API key"
	}
	inFamily := func(id string) bool {
		for _, model := range family.Models {
			if model.ID == id {
				return true
			}
		}
		return false
	}
	if cc.Model == "" {
		fields["claudeCode.model"] = "choose a model"
	} else if ok && !inFamily(cc.Model) {
		fields["claudeCode.model"] = "model " + cc.Model + " is not one of the family's models"
	}
	if cc.SmallModel != "" && ok && !inFamily(cc.SmallModel) {
		fields["claudeCode.smallModel"] = "model " + cc.SmallModel + " is not one of the family's models"
	}
	if cc.MaxTurns < 0 || cc.MaxTurns > MaxTurns {
		fields["claudeCode.maxTurns"] = "max turns must be from 1 to 200, or empty for the router's budget"
	}
	switch cc.Auth {
	case "", AuthAuto, AuthAPIKey, AuthBearer:
	default:
		fields["claudeCode.auth"] = "the auth header must be auto, apiKey, or bearer"
	}
	if len(cc.Instructions) > MaxAgentInstructions {
		fields["claudeCode.instructions"] = "extra instructions must be at most 8 KiB"
	}
	if cc.Binary != "" && !filepath.IsAbs(cc.Binary) {
		fields["claudeCode.binary"] = "the claude binary must be an absolute path"
	}
	return fields
}
