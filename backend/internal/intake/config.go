package intake

import (
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	PRD      string
	RFC      string
	Operator string
}

type fileConfig struct {
	Paths struct {
		PRD string `yaml:"prd"`
		RFC string `yaml:"rfc"`
	} `yaml:"paths"`
	OperatorLogin string `yaml:"operatorLogin"`
}

func LoadConfig(dataDir string) (Config, error) {
	cfg := Config{PRD: "docs/prd", RFC: "docs/rfc"}
	body, err := os.ReadFile(filepath.Join(dataDir, "config.yaml"))
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return Config{}, err
	}
	var file fileConfig
	if err := yaml.Unmarshal(body, &file); err != nil {
		return Config{}, err
	}
	if file.Paths.PRD != "" {
		cfg.PRD = file.Paths.PRD
	}
	if file.Paths.RFC != "" {
		cfg.RFC = file.Paths.RFC
	}
	cfg.Operator = file.OperatorLogin
	return cfg, nil
}

func RepoPath(repo, configured string) (string, error) {
	path := configured
	if !filepath.IsAbs(path) {
		path = filepath.Join(repo, path)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	abs = filepath.Clean(abs)
	repoAbs, err := filepath.Abs(repo)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(repoAbs, abs)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", os.ErrInvalid
	}
	return abs, nil
}
