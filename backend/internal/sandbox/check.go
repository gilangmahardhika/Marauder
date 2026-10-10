package sandbox

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var (
	ErrParse = errors.New("check command refused")
	ErrAllow = errors.New("check command is not allowlisted")
	ErrEmpty = errors.New("check command is empty")
	ErrImage = errors.New("sandbox image is not pinned by digest")
)

const forbidden = ";|&$`<>(){}[]*?!~\"'\\"

func ParseCheck(command string, allow []string) ([]string, error) {
	if strings.ContainsAny(command, "\n\t") || strings.ContainsAny(command, forbidden) {
		return nil, ErrParse
	}
	var parts []string
	for _, part := range strings.Split(command, " ") {
		if part == "" {
			continue
		}
		parts = append(parts, part)
	}
	if len(parts) == 0 {
		return nil, ErrEmpty
	}
	if !allowed(parts[0], allow) {
		return nil, ErrAllow
	}
	return parts, nil
}

func allowed(name string, allow []string) bool {
	for _, item := range allow {
		if item == name {
			return true
		}
	}
	return false
}

func ResolveFixture(name string) (string, error) {
	if name == "" || strings.Contains(name, "/") {
		return "", fmt.Errorf("check name %q is not a bare name", name)
	}
	for _, dir := range []string{"/usr/local/bin", "/usr/bin", "/bin"} {
		candidate := filepath.Join(dir, name)
		info, err := os.Stat(candidate)
		if err == nil && !info.IsDir() {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("check %q is not in the fixture path", name)
}

func ContainerEnv() []string {
	return []string{
		"PATH=/usr/local/bin:/usr/bin:/bin",
		"HOME=/tmp",
		"CI=1",
	}
}

func EngineEnv(engine string) []string {
	env := []string{"PATH=" + filepath.Dir(engine)}
	if home := os.Getenv("HOME"); home != "" {
		env = append(env, "HOME="+home)
	}
	if xdg := os.Getenv("XDG_RUNTIME_DIR"); xdg != "" {
		env = append(env, "XDG_RUNTIME_DIR="+xdg)
	}
	return env
}

func LocalArgv(engine, image, export string, check []string) ([]string, error) {
	return containerArgv(engine, image, export, check, true, "", "")
}

func DevArgv(engine, image, export, devURL, sha string, check []string) ([]string, error) {
	return containerArgv(engine, image, export, check, false, devURL, sha)
}

func containerArgv(engine, image, export string, check []string, local bool, devURL, sha string) ([]string, error) {
	if !pinnedImage(image) {
		return nil, ErrImage
	}
	argv := []string{engine, "run", "--rm"}
	if local {
		argv = append(argv, "--network=none")
	}
	argv = append(argv,
		"--cap-drop=ALL",
		"--security-opt=no-new-privileges",
		"--pids-limit=256",
		"--memory=2g",
		"--cpus=2",
		"--read-only",
		"--tmpfs", "/tmp:rw,nosuid,size=256m",
		"--mount", fmt.Sprintf("type=bind,src=%s,dst=/work,relabel=private", export),
		"--workdir", "/work",
		"--user", "65532:65532",
		"--env", "CI=1",
		"--env", "PATH=/usr/local/bin:/usr/bin:/bin",
		"--env", "HOME=/tmp",
	)
	if !local {
		argv = append(argv, "--env", "DEV_URL="+devURL, "--env", "ORCH_SHA="+sha)
	}
	argv = append(argv, image)
	argv = append(argv, check...)
	return argv, nil
}

func pinnedImage(image string) bool {
	marker := "@sha256:"
	i := strings.LastIndex(image, marker)
	if i < 0 {
		return false
	}
	hexPart := image[i+len(marker):]
	if len(hexPart) != 64 {
		return false
	}
	for _, c := range hexPart {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}
