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
	DatabaseURL string            `json:"databaseUrl"`
	TokenKey    string            `json:"tokenKey,omitempty"`
	LLMKeys     map[string]string `json:"llmKeys"`
}

type Model struct {
	ID string `yaml:"id" json:"id"`
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
		for _, model := range family.Models {
			if model.ID == "" || seen[model.ID] {
				return current, ErrValidation
			}
			seen[model.ID] = true
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
