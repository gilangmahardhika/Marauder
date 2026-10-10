package settings

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

var ErrValidation = errors.New("validation")

type Secrets struct {
	DatabaseURL string            `json:"databaseUrl"`
	GitHubToken string            `json:"githubToken"`
	DeployToken string            `json:"deployToken"`
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
	GitHub struct {
		Host       string   `yaml:"host" json:"host"`
		Owner      string   `yaml:"owner" json:"owner"`
		Repo       string   `yaml:"repo" json:"repo"`
		BaseBranch string   `yaml:"baseBranch" json:"baseBranch"`
		BotLogin   string   `yaml:"botLogin" json:"botLogin"`
		Approvers  []string `yaml:"approvers" json:"approvers"`
	} `yaml:"github" json:"github"`
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
	Deploy struct {
		Environment       string   `yaml:"environment" json:"environment"`
		DevURL            string   `yaml:"devUrl" json:"devUrl"`
		DevHosts          []string `yaml:"devHosts" json:"devHosts"`
		DevCommand        string   `yaml:"devCommand" json:"devCommand"`
		RollbackCommand   string   `yaml:"rollbackCommand" json:"rollbackCommand"`
		HealthURL         string   `yaml:"healthUrl" json:"healthUrl"`
		LocalCheck        string   `yaml:"localCheck" json:"localCheck"`
		DevCheck          string   `yaml:"devCheck" json:"devCheck"`
		AllowExecutables  []string `yaml:"allowExecutables" json:"allowExecutables"`
		SandboxEngine     string   `yaml:"sandboxEngine" json:"sandboxEngine"`
		SandboxImage      string   `yaml:"sandboxImage" json:"sandboxImage"`
		ImplementationCap int      `yaml:"implementationCap" json:"implementationCap"`
		PRDPath           string   `yaml:"prdPath" json:"prdPath"`
		RFCPath           string   `yaml:"rfcPath" json:"rfcPath"`
		OperatorLogin     string   `yaml:"operatorLogin" json:"operatorLogin"`
	} `yaml:"deploy" json:"deploy"`
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

func SaveGitHub(data string, cfg Config, token *string) (Config, error) {
	current, secrets, err := Load(data)
	if err != nil {
		return cfg, err
	}
	if contains(cfg.GitHub.Approvers, cfg.GitHub.BotLogin) && cfg.GitHub.BotLogin != "" {
		return current, ErrValidation
	}
	if token != nil && *token == "" {
		secrets.GitHubToken = ""
	} else if token != nil {
		if len(*token) < 8 {
			return current, ErrValidation
		}
		secrets.GitHubToken = *token
	}
	current.GitHub = cfg.GitHub
	if err := write(data, current, secrets); err != nil {
		return current, err
	}
	return current, nil
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

func SaveDeploy(data string, cfg Config, token *string, clear bool) (Config, error) {
	current, secrets, err := Load(data)
	if err != nil {
		return current, err
	}
	if clear {
		secrets.DeployToken = ""
	} else if token != nil {
		if len(*token) < 8 {
			return current, ErrValidation
		}
		secrets.DeployToken = *token
	}
	if cfg.Deploy.Environment == "" {
		cfg.Deploy.Environment = "dev"
	}
	if cfg.Deploy.Environment != "dev" {
		return current, ErrValidation
	}
	current.Deploy = cfg.Deploy
	if err := write(data, current, secrets); err != nil {
		return current, err
	}
	return current, nil
}

func contains(list []string, value string) bool {
	for _, item := range list {
		if item == value {
			return true
		}
	}
	return false
}

func write(data string, cfg Config, secrets Secrets) error {
	body, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(data, "config.yaml"), body, 0o600); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(secrets, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(data, "secrets.json"), append(raw, '\n'), 0o600)
}
