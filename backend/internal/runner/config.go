package runner

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Config is what register writes and run reads. It holds the runner secret,
// so the file is mode 0600 and run refuses a file others can read.
type Config struct {
	URL      string   `json:"url"`
	RunnerID string   `json:"runnerId"`
	Secret   string   `json:"secret"`
	Name     string   `json:"name"`
	Slots    int      `json:"slots"`
	Labels   []string `json:"labels"`
	Data     string   `json:"data"`
}

// ErrConfigMode is a config file that a group or other user can read.
var ErrConfigMode = errors.New("runner config must be mode 0600")

// DefaultConfigPath is runner.json in the user's config directory.
func DefaultConfigPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "orchestrator", "runner.json"), nil
}

// CheckURL accepts an https origin, or http on a loopback host for a local
// control plane.
func CheckURL(raw string) error {
	if strings.HasPrefix(raw, "https://") {
		return nil
	}
	for _, host := range []string{"http://127.0.0.1", "http://localhost", "http://[::1]"} {
		if raw == host || strings.HasPrefix(raw, host+":") || strings.HasPrefix(raw, host+"/") {
			return nil
		}
	}
	return fmt.Errorf("control plane URL must be https")
}

// SaveConfig writes the config with mode 0600 through a temporary file.
func SaveConfig(path string, cfg Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	body, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".runner-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(append(body, '\n')); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// LoadConfig reads the config and refuses one a group or other user can read.
func LoadConfig(path string) (Config, error) {
	var cfg Config
	info, err := os.Stat(path)
	if err != nil {
		return cfg, err
	}
	if info.Mode().Perm()&0o077 != 0 {
		return cfg, fmt.Errorf("%w: %s is %#o", ErrConfigMode, path, info.Mode().Perm())
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return cfg, err
	}
	if err := json.Unmarshal(body, &cfg); err != nil {
		return cfg, err
	}
	if cfg.URL == "" || cfg.Secret == "" || cfg.Data == "" {
		return cfg, fmt.Errorf("%s lacks url, secret, or data; register the runner again", path)
	}
	return cfg, CheckURL(cfg.URL)
}
