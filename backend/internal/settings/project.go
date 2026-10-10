package settings

import (
	"encoding/json"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"

	"orchestrator/backend/internal/sandbox"
)

var (
	hostPattern  = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)*$`)
	execPattern  = regexp.MustCompile(`^[A-Za-z0-9._+-]+$`)
	imagePattern = regexp.MustCompile(`@sha256:[0-9a-f]{64}$`)
)

// ProjectConfig is <data>/projects/<id>/config.yaml.
type ProjectConfig struct {
	Deploy struct {
		Environment     string   `yaml:"environment"`
		DevURL          string   `yaml:"devUrl"`
		DevHosts        []string `yaml:"devHosts"`
		DevCommand      string   `yaml:"devCommand"`
		RollbackCommand string   `yaml:"rollbackCommand"`
		HealthURL       string   `yaml:"healthUrl"`
	} `yaml:"deploy"`
	Checks struct {
		Local            string   `yaml:"local"`
		Dev              string   `yaml:"dev"`
		AllowExecutables []string `yaml:"allowExecutables"`
	} `yaml:"checks"`
	Sandbox struct {
		Engine string `yaml:"engine"`
		Image  string `yaml:"image"`
	} `yaml:"sandbox"`
	Scheduler struct {
		ImplementationCap int `yaml:"implementationCap"`
	} `yaml:"scheduler"`
	Paths struct {
		PRD string `yaml:"prd"`
		RFC string `yaml:"rfc"`
	} `yaml:"paths"`
}

// ProjectSecrets is <data>/projects/<id>/secrets.json. It is never encoded to a client.
type ProjectSecrets struct {
	DeployToken string `json:"deployToken"`
}

// Deploy is the flat shape of the project deploy route.
type Deploy struct {
	Environment       string   `json:"environment"`
	DevURL            string   `json:"devUrl"`
	DevHosts          []string `json:"devHosts"`
	DevCommand        string   `json:"devCommand"`
	RollbackCommand   string   `json:"rollbackCommand"`
	HealthURL         string   `json:"healthUrl"`
	LocalCheck        string   `json:"localCheck"`
	DevCheck          string   `json:"devCheck"`
	AllowExecutables  []string `json:"allowExecutables"`
	SandboxEngine     string   `json:"sandboxEngine"`
	SandboxImage      string   `json:"sandboxImage"`
	ImplementationCap int      `json:"implementationCap"`
	PRDPath           string   `json:"prdPath"`
	RFCPath           string   `json:"rfcPath"`
}

// ProjectPaths are the directories a deploy command must stay out of.
type ProjectPaths struct {
	Data string
	Repo string
}

func ProjectDir(data, projectID string) string {
	return filepath.Join(data, "projects", projectID)
}

func LoadProject(data, projectID string) (ProjectConfig, ProjectSecrets, error) {
	var cfg ProjectConfig
	var secrets ProjectSecrets
	dir := ProjectDir(data, projectID)
	if body, err := os.ReadFile(filepath.Join(dir, "config.yaml")); err == nil {
		if err := yaml.Unmarshal(body, &cfg); err != nil {
			return cfg, secrets, err
		}
	} else if !os.IsNotExist(err) {
		return cfg, secrets, err
	}
	if body, err := os.ReadFile(filepath.Join(dir, "secrets.json")); err == nil {
		if err := json.Unmarshal(body, &secrets); err != nil {
			return cfg, secrets, err
		}
	} else if !os.IsNotExist(err) {
		return cfg, secrets, err
	}
	return cfg, secrets, nil
}

// DeployView flattens the project sections and fills the defaults.
func DeployView(cfg ProjectConfig) Deploy {
	d := Deploy{
		Environment:       cfg.Deploy.Environment,
		DevURL:            cfg.Deploy.DevURL,
		DevHosts:          cfg.Deploy.DevHosts,
		DevCommand:        cfg.Deploy.DevCommand,
		RollbackCommand:   cfg.Deploy.RollbackCommand,
		HealthURL:         cfg.Deploy.HealthURL,
		LocalCheck:        cfg.Checks.Local,
		DevCheck:          cfg.Checks.Dev,
		AllowExecutables:  cfg.Checks.AllowExecutables,
		SandboxEngine:     cfg.Sandbox.Engine,
		SandboxImage:      cfg.Sandbox.Image,
		ImplementationCap: cfg.Scheduler.ImplementationCap,
		PRDPath:           cfg.Paths.PRD,
		RFCPath:           cfg.Paths.RFC,
	}
	if d.Environment == "" {
		d.Environment = "dev"
	}
	if d.DevHosts == nil {
		d.DevHosts = []string{}
	}
	if d.AllowExecutables == nil {
		d.AllowExecutables = []string{}
	}
	if d.SandboxEngine == "" {
		d.SandboxEngine = "podman"
	}
	if d.ImplementationCap == 0 {
		d.ImplementationCap = 2
	}
	if d.PRDPath == "" {
		d.PRDPath = "docs/prd"
	}
	if d.RFCPath == "" {
		d.RFCPath = "docs/rfc"
	}
	return d
}

// SaveProjectDeploy validates d, then writes that project's config.yaml and,
// only when a token was sent or cleared, its secrets.json. It returns the saved
// field names for the config_changed event.
func SaveProjectDeploy(data, projectID string, paths ProjectPaths, d Deploy, token *string, clear bool) (ProjectConfig, []string, error) {
	current, secrets, err := LoadProject(data, projectID)
	if err != nil {
		return current, nil, err
	}
	if d.Environment == "" {
		d.Environment = "dev"
	}
	if fields := validateDeploy(d, paths); len(fields) > 0 {
		return current, nil, &Invalid{Fields: fields}
	}
	if token != nil && !clear && len(*token) < 8 {
		return current, nil, &Invalid{Fields: map[string]string{"deployToken": "token is too short"}}
	}
	next := current
	next.Deploy.Environment = d.Environment
	next.Deploy.DevURL = d.DevURL
	next.Deploy.DevHosts = d.DevHosts
	next.Deploy.DevCommand = d.DevCommand
	next.Deploy.RollbackCommand = d.RollbackCommand
	next.Deploy.HealthURL = d.HealthURL
	next.Checks.Local = d.LocalCheck
	next.Checks.Dev = d.DevCheck
	next.Checks.AllowExecutables = d.AllowExecutables
	next.Sandbox.Engine = d.SandboxEngine
	next.Sandbox.Image = d.SandboxImage
	next.Scheduler.ImplementationCap = d.ImplementationCap
	next.Paths.PRD = d.PRDPath
	next.Paths.RFC = d.RFCPath

	dir := ProjectDir(data, projectID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return current, nil, err
	}
	body, err := yaml.Marshal(next)
	if err != nil {
		return current, nil, err
	}
	if err := WriteAtomic(filepath.Join(dir, "config.yaml"), body); err != nil {
		return current, nil, err
	}
	changed := []string{"environment", "devUrl", "devHosts", "devCommand", "rollbackCommand", "healthUrl",
		"localCheck", "devCheck", "allowExecutables", "sandboxEngine", "sandboxImage",
		"implementationCap", "prdPath", "rfcPath"}
	if clear || token != nil {
		if clear {
			secrets.DeployToken = ""
		} else {
			secrets.DeployToken = *token
		}
		raw, err := json.MarshalIndent(secrets, "", "  ")
		if err != nil {
			return next, nil, err
		}
		if err := WriteAtomic(filepath.Join(dir, "secrets.json"), append(raw, '\n')); err != nil {
			return next, nil, err
		}
		changed = append(changed, "deployToken")
	}
	return next, changed, nil
}

func validateDeploy(d Deploy, paths ProjectPaths) map[string]string {
	fields := map[string]string{}
	if d.Environment != "dev" {
		fields["environment"] = "environment must be dev"
	}
	hosts := map[string]bool{}
	for _, host := range d.DevHosts {
		if !validHost(host) {
			fields["devHosts"] = "each dev host is a lowercase hostname or IP, with no scheme, port, or wildcard"
			break
		}
		hosts[host] = true
	}
	for name, raw := range map[string]string{"devUrl": d.DevURL, "healthUrl": d.HealthURL} {
		if raw == "" {
			continue
		}
		if msg := checkURL(raw, hosts); msg != "" {
			fields[name] = msg
		}
	}
	for name, path := range map[string]string{"devCommand": d.DevCommand, "rollbackCommand": d.RollbackCommand} {
		if path == "" {
			continue
		}
		if msg := checkScript(path, paths); msg != "" {
			fields[name] = msg
		}
	}
	for _, name := range d.AllowExecutables {
		if !execPattern.MatchString(name) {
			fields["allowExecutables"] = "each executable is a bare name"
			break
		}
	}
	for name, command := range map[string]string{"localCheck": d.LocalCheck, "devCheck": d.DevCheck} {
		if command == "" {
			continue
		}
		if _, err := sandbox.ParseCheck(command, d.AllowExecutables); err != nil {
			fields[name] = "the first word must be an allowed executable, with no shell syntax"
		}
	}
	if d.SandboxEngine != "" && d.SandboxEngine != "podman" && d.SandboxEngine != "docker" {
		fields["sandboxEngine"] = "engine is podman or docker"
	}
	if d.SandboxImage != "" && !imagePattern.MatchString(d.SandboxImage) {
		fields["sandboxImage"] = "pin the image by digest, name@sha256:<64 hex>"
	}
	if d.ImplementationCap != 0 && (d.ImplementationCap < 1 || d.ImplementationCap > 8) {
		fields["implementationCap"] = "implementation cap is from 1 to 8"
	}
	for name, path := range map[string]string{"prdPath": d.PRDPath, "rfcPath": d.RFCPath} {
		if path == "" {
			continue
		}
		if !RelativeInside(path) {
			fields[name] = "path is relative to the repository, with no '..'"
		}
	}
	return fields
}

// RelativeInside reports whether p is a relative path that stays inside its root
// and is not under .git or .orchestrator.
func RelativeInside(p string) bool {
	if p == "" || strings.ContainsRune(p, 0) || filepath.IsAbs(p) || strings.HasPrefix(p, "/") {
		return false
	}
	cleaned := filepath.ToSlash(filepath.Clean(p))
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return false
	}
	for _, part := range strings.Split(cleaned, "/") {
		if part == ".." {
			return false
		}
	}
	first := strings.SplitN(cleaned, "/", 2)[0]
	return first != ".git" && first != ".orchestrator"
}

func validHost(host string) bool {
	if host == "" || host != strings.ToLower(host) {
		return false
	}
	if ip := net.ParseIP(host); ip != nil {
		return true
	}
	return hostPattern.MatchString(host)
}

func checkURL(raw string, hosts map[string]bool) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "not a URL"
	}
	if u.User != nil {
		return "URL must not carry a user or password"
	}
	host := u.Hostname()
	switch u.Scheme {
	case "https":
	case "http":
		if host != "127.0.0.1" {
			return "use https, or http on 127.0.0.1"
		}
	default:
		return "use https, or http on 127.0.0.1"
	}
	if !hosts[host] {
		return "host must be listed in dev hosts"
	}
	return ""
}

func checkScript(path string, paths ProjectPaths) string {
	if !filepath.IsAbs(path) {
		return "use an absolute path"
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "file not found"
	}
	for _, root := range []string{paths.Repo, paths.Data} {
		if root == "" {
			continue
		}
		if r, err := filepath.EvalSymlinks(root); err == nil {
			root = r
		}
		if inside(root, resolved) {
			return "script must be outside the repository and the data directory"
		}
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.Mode().IsRegular() {
		return "not a regular file"
	}
	if info.Mode().Perm()&0o022 != 0 {
		return "script is group or world writable"
	}
	if info.Mode().Perm()&0o100 == 0 {
		return "script is not executable"
	}
	if !ownedByCurrentUser(info) {
		return "script is not owned by the service user"
	}
	return ""
}

func inside(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}
