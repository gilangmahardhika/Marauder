package sandbox

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

const (
	MaxArgs      = 64
	MaxArgvBytes = 4 << 10
)

var (
	ErrArgv    = fmt.Errorf("argv must be 1 to %d strings, at most %d bytes in total, with no NUL", MaxArgs, MaxArgvBytes)
	ErrBare    = errors.New("the command is not a bare name")
	ErrWrapper = errors.New("shells and programs that run another program are refused")
	ErrCommand = errors.New("the command is not on the allowed commands list")
	ErrExport  = errors.New("export directory is not under the exports directory or is the worktree")
)

// wrappers are shells and programs whose job is to run another program. An
// admin cannot list them as commands, because they would turn the allowlist
// into any program in the image.
var wrappers = map[string]bool{
	"sh": true, "bash": true, "zsh": true, "dash": true, "ksh": true, "fish": true, "csh": true, "tcsh": true,
	"pwsh": true, "powershell": true, "env": true, "xargs": true, "busybox": true, "timeout": true, "nice": true,
	"sudo": true, "doas": true, "su": true, "nohup": true, "exec": true, "eval": true, "command": true,
}

// Wrapper reports whether name is a shell or a program that runs another one.
func Wrapper(name string) bool {
	return wrappers[strings.ToLower(name)]
}

// ParseArgv checks a run_command argv (RFC-084): 1 to 64 strings of at most
// 4 KiB in total, whose first is a bare name on allow and not a wrapper.
func ParseArgv(argv []string, allow []string) error {
	if len(argv) == 0 || len(argv) > MaxArgs {
		return ErrArgv
	}
	total := 0
	for _, arg := range argv {
		total += len(arg)
		if strings.ContainsRune(arg, 0) {
			return ErrArgv
		}
	}
	if total > MaxArgvBytes {
		return ErrArgv
	}
	name := argv[0]
	if name == "" || strings.ContainsAny(name, `/\`) || strings.TrimSpace(name) != name {
		return ErrBare
	}
	if Wrapper(name) {
		return ErrWrapper
	}
	if !allowed(name, allow) {
		return ErrCommand
	}
	return nil
}

// CheckExport asserts that dir is strictly under exports and is neither the
// worktree nor inside or around it, before a container mounts it.
func CheckExport(exports, dir, worktree string) error {
	root, err := resolve(exports)
	if err != nil {
		return err
	}
	target, err := resolve(dir)
	if err != nil {
		return err
	}
	if !under(root, target) {
		return ErrExport
	}
	if worktree != "" {
		wt, err := resolve(worktree)
		if err != nil {
			return err
		}
		if target == wt || under(target, wt) || under(wt, target) {
			return ErrExport
		}
	}
	return nil
}

// resolve follows symlinks in the deepest part of path that exists, so a path
// not made yet compares equal to its parent's real location.
func resolve(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	for dir := abs; ; dir = filepath.Dir(dir) {
		if real, err := filepath.EvalSymlinks(dir); err == nil {
			rest, err := filepath.Rel(dir, abs)
			if err != nil {
				return "", err
			}
			return filepath.Join(real, rest), nil
		}
		if filepath.Dir(dir) == dir {
			return abs, nil
		}
	}
}

// under reports whether path is strictly inside root.
func under(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}
