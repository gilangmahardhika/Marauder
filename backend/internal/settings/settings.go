package settings

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

var ErrValidation = errors.New("validation")

// Invalid names the fields that failed. It unwraps to ErrValidation.
type Invalid struct {
	Fields map[string]string
}

func (v *Invalid) Error() string {
	for _, msg := range v.Fields {
		return msg
	}
	return "validation"
}

func (v *Invalid) Unwrap() error { return ErrValidation }

type Secrets struct {
	DatabaseURL     string            `json:"databaseUrl"`
	TokenKey        string            `json:"tokenKey,omitempty"`
	VAPIDPrivateKey string            `json:"vapidPrivateKey,omitempty"`
	LLMKeys         map[string]string `json:"llmKeys"`
}

type Model struct {
	ID           string `yaml:"id" json:"id"`
	ModelOptions `yaml:",inline"`
}

// ModelOptions are the thinking and effort settings sent with every request to
// one model (RFC-079). Empty values send nothing. HistoryBudgetKiB is how much
// earlier tool output an implement attempt on this model sends back (RFC-081).
type ModelOptions struct {
	Thinking         string `yaml:"thinking,omitempty" json:"thinking,omitempty"`
	ThinkingBudget   int    `yaml:"thinkingBudget,omitempty" json:"thinkingBudget,omitempty"`
	Effort           string `yaml:"effort,omitempty" json:"effort,omitempty"`
	HistoryBudgetKiB int    `yaml:"historyBudgetKiB,omitempty" json:"historyBudgetKiB,omitempty"`
}

const (
	MinThinkingBudget = 1024
	MaxThinkingBudget = 32768
	MinHistoryKiB     = 4
	MaxHistoryKiB     = 512
	// DefaultHistoryKiB is the history budget of a model that sets none.
	DefaultHistoryKiB = 16

	// DefaultTurns and DefaultTokensPerCard apply when the router budgets
	// leave them zero (RFC-081). MaxTurns is the most an admin may save.
	DefaultTurns         = 40
	MaxTurns             = 200
	DefaultTokensPerCard = 2000000
)

// HistoryBytes is the model's history budget in bytes.
func (o ModelOptions) HistoryBytes() int {
	if o.HistoryBudgetKiB <= 0 {
		return DefaultHistoryKiB << 10
	}
	return o.HistoryBudgetKiB << 10
}

var efforts = map[string][]string{
	"anthropic": {"low", "medium", "high", "xhigh", "max"},
	"openai":    {"none", "minimal", "low", "medium", "high", "xhigh"},
}

// Options lists the settings of each model that has any, keyed by model id.
func (f Family) Options() map[string]ModelOptions {
	out := map[string]ModelOptions{}
	for _, model := range f.Models {
		if model.ModelOptions != (ModelOptions{}) {
			out[model.ID] = model.ModelOptions
		}
	}
	return out
}

// NewModels pairs each id with its options. An option for an id that is not in
// ids is refused on modelOptions.
func NewModels(ids []string, options map[string]ModelOptions) ([]Model, error) {
	models := []Model{}
	listed := map[string]bool{}
	for _, id := range ids {
		listed[id] = true
		models = append(models, Model{ID: id, ModelOptions: options[id]})
	}
	for id := range options {
		if !listed[id] {
			return nil, &Invalid{Fields: map[string]string{"modelOptions": "options for " + id + ", which is not one of the family's models"}}
		}
	}
	return models, nil
}

// checkOptions refuses a value the protocol does not take, and drops a budget
// that thinking does not use.
func checkOptions(protocol string, opts ModelOptions) (ModelOptions, error) {
	bad := func(msg string) (ModelOptions, error) {
		return opts, &Invalid{Fields: map[string]string{"modelOptions": msg}}
	}
	switch opts.Thinking {
	case "", "enabled", "disabled":
	case "adaptive":
		if protocol != "anthropic" {
			return bad("adaptive thinking needs the anthropic protocol")
		}
	default:
		return bad("thinking must be enabled, disabled, or adaptive")
	}
	if opts.Thinking == "enabled" && protocol == "anthropic" {
		if opts.ThinkingBudget != 0 && (opts.ThinkingBudget < MinThinkingBudget || opts.ThinkingBudget > MaxThinkingBudget) {
			return bad("the thinking budget must be from 1024 to 32768 tokens")
		}
	} else {
		opts.ThinkingBudget = 0
	}
	if opts.Effort != "" {
		ok := false
		for _, effort := range efforts[protocol] {
			ok = ok || effort == opts.Effort
		}
		if !ok {
			return bad("effort " + opts.Effort + " is not taken by the " + protocol + " protocol")
		}
	}
	if opts.HistoryBudgetKiB != 0 && (opts.HistoryBudgetKiB < MinHistoryKiB || opts.HistoryBudgetKiB > MaxHistoryKiB) {
		return bad("the history budget must be from 4 to 512 KiB")
	}
	return opts, nil
}

type Family struct {
	ID       string  `yaml:"id" json:"id"`
	Protocol string  `yaml:"protocol" json:"protocol"`
	BaseURL  string  `yaml:"baseUrl" json:"baseUrl"`
	Models   []Model `yaml:"models" json:"models"`
}

type Config struct {
	LLM struct {
		Families []Family `yaml:"families" json:"families"`
	} `yaml:"llm" json:"llm"`
	Router struct {
		Routes map[string]struct {
			Model    string `yaml:"model" json:"model"`
			Fallback string `yaml:"fallback" json:"fallback"`
		} `yaml:"routes" json:"routes"`
		Budgets struct {
			MaxAttempts              int `yaml:"maxAttempts" json:"maxAttempts"`
			MaxModelTurnsPerAttempt  int `yaml:"maxModelTurnsPerAttempt" json:"maxModelTurnsPerAttempt"`
			MaxTokensPerCard         int `yaml:"maxTokensPerCard" json:"maxTokensPerCard"`
			MaxWallMinutesPerAttempt int `yaml:"maxWallMinutesPerAttempt" json:"maxWallMinutesPerAttempt"`
		} `yaml:"budgets" json:"budgets"`
	} `yaml:"router" json:"router"`
	// Jobs.Executor is where implement jobs run: local, the serve process,
	// or runners, the remote runners (RFC-107). Empty means local.
	Jobs struct {
		Executor string `yaml:"executor,omitempty" json:"executor"`
	} `yaml:"jobs,omitempty" json:"jobs"`
}

const (
	ExecutorLocal   = "local"
	ExecutorRunners = "runners"
)

// Executor reports where implement jobs run.
func (c Config) Executor() string {
	if c.Jobs.Executor == ExecutorRunners {
		return ExecutorRunners
	}
	return ExecutorLocal
}

// SaveJobs stores jobs.executor in the service config.
func SaveJobs(data, executor string) (Config, error) {
	current, secrets, err := Load(data)
	if err != nil {
		return current, err
	}
	if executor != ExecutorLocal && executor != ExecutorRunners {
		return current, &Invalid{Fields: map[string]string{"executor": "executor must be local or runners"}}
	}
	current.Jobs.Executor = executor
	if err := write(data, current, secrets); err != nil {
		return current, err
	}
	return current, nil
}

func Load(data string) (Config, Secrets, error) {
	var cfg Config
	var secrets Secrets
	if body, err := os.ReadFile(filepath.Join(data, "config.yaml")); err == nil {
		if err := yaml.Unmarshal(body, &cfg); err != nil {
			return cfg, secrets, err
		}
	}
	if body, err := os.ReadFile(filepath.Join(data, "secrets.json")); err == nil {
		if err := json.Unmarshal(body, &secrets); err != nil {
			return cfg, secrets, err
		}
	}
	if secrets.LLMKeys == nil {
		secrets.LLMKeys = map[string]string{}
	}
	return cfg, secrets, nil
}

func View(cfg Config) Config {
	return cfg
}

func SaveRouter(data string, cfg Config) (Config, error) {
	current, secrets, err := Load(data)
	if err != nil {
		return cfg, err
	}
	known := map[string]bool{}
	for _, family := range current.LLM.Families {
		for _, model := range family.Models {
			if known[model.ID] {
				return current, ErrValidation
			}
			known[model.ID] = true
		}
	}
	for _, route := range cfg.Router.Routes {
		for _, model := range []string{route.Model, route.Fallback} {
			if model == "" || model == "different-family-from-author" {
				continue
			}
			if !known[model] {
				return current, ErrValidation
			}
		}
	}
	if turns := cfg.Router.Budgets.MaxModelTurnsPerAttempt; turns < 0 || turns > MaxTurns {
		return current, &Invalid{Fields: map[string]string{"maxModelTurnsPerAttempt": "model turns must be from 1 to 200"}}
	}
	if cfg.Router.Budgets.MaxTokensPerCard < 0 {
		return current, &Invalid{Fields: map[string]string{"maxTokensPerCard": "tokens per card cannot be negative"}}
	}
	current.Router = cfg.Router
	if err := write(data, current, secrets); err != nil {
		return current, err
	}
	return current, nil
}

func SaveLLM(data string, families []Family, keys map[string]*string) (Config, error) {
	current, secrets, err := Load(data)
	if err != nil {
		return current, err
	}
	seen := map[string]bool{}
	for _, family := range families {
		for i, model := range family.Models {
			if model.ID == "" || seen[model.ID] {
				return current, ErrValidation
			}
			seen[model.ID] = true
			if family.Models[i].ModelOptions, err = checkOptions(family.Protocol, model.ModelOptions); err != nil {
				return current, err
			}
		}
	}
	for id, key := range keys {
		if key == nil {
			continue
		}
		if *key == "" {
			secrets.LLMKeys[id] = ""
			continue
		}
		if len(*key) < 8 {
			return current, ErrValidation
		}
		if secrets.LLMKeys == nil {
			secrets.LLMKeys = map[string]string{}
		}
		secrets.LLMKeys[id] = *key
	}
	current.LLM.Families = families
	if err := write(data, current, secrets); err != nil {
		return current, err
	}
	return current, nil
}

// write rewrites the service files. The service secrets file also holds
// databaseUrl and tokenKey, so it is decoded and encoded whole.
func write(data string, cfg Config, secrets Secrets) error {
	body, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	if err := WriteAtomic(filepath.Join(data, "config.yaml"), body); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(secrets, "", "  ")
	if err != nil {
		return err
	}
	return WriteAtomic(filepath.Join(data, "secrets.json"), append(raw, '\n'))
}

// WriteAtomic writes through a temp file in the same directory, syncs, and renames, mode 0600.
func WriteAtomic(path string, body []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".write-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if err := f.Chmod(0o600); err != nil {
		f.Close()
		return err
	}
	if _, err := f.Write(body); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
