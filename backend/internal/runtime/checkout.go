package runtime

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const netTimeout = 10 * time.Minute

var ErrTokenInArgv = errors.New("token in argv")

// askpassScript answers git's username prompt with a fixed name and its
// password prompt with GH_TOKEN from the child environment.
const askpassScript = "#!/bin/sh\ncase \"$1\" in\n  Username*) printf '%s\\n' x-access-token ;;\n  *) printf '%s\\n' \"$GH_TOKEN\" ;;\nesac\n"

// Remote is an explicit https URL built from settings. Remotes stored in the repo are not used.
type Remote struct {
	URL   string
	Token string
}

var projectSubdirs = []string{"worktrees", "exports", "empty-hooks", "empty-gh", "askpass", "logs"}

// PrepareProjectDir creates a project directory, mode 0700, with the subdirectories git calls expect.
func PrepareProjectDir(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return err
	}
	for _, name := range projectSubdirs {
		if err := os.MkdirAll(filepath.Join(dir, name), 0o700); err != nil {
			return err
		}
	}
	return writeAskpass(dir)
}

func AskpassPath(data string) string {
	return filepath.Join(data, "askpass", "askpass.sh")
}

func writeAskpass(data string) error {
	path := AskpassPath(data)
	if body, err := os.ReadFile(path); err == nil && string(body) == askpassScript {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(askpassScript), 0o700)
}

// EnsureCheckout clones remote into g.Repo when it is missing. Otherwise it
// fetches base from remote and checks it out. It returns the head SHA.
func (g Git) EnsureCheckout(ctx context.Context, base string, remote Remote) (string, error) {
	if base == "" {
		base = "main"
	}
	if !ValidBranch(base) {
		return "", fmt.Errorf("base branch %q is invalid", base)
	}
	if !strings.HasPrefix(remote.URL, "https://") {
		return "", fmt.Errorf("remote must be https")
	}
	if err := writeAskpass(g.Data); err != nil {
		return "", err
	}
	env := append(gitEnv(g.Bin, g.Data),
		"GIT_ASKPASS="+AskpassPath(g.Data),
		"ORCH_ASKPASS=1",
		"GH_TOKEN="+remote.Token,
	)
	if _, err := os.Stat(filepath.Join(g.Repo, ".git")); os.IsNotExist(err) {
		if err := os.MkdirAll(filepath.Dir(g.Repo), 0o700); err != nil {
			return "", err
		}
		argv := append(g.prefix(), "clone", "--no-tags", "--branch", base, "--", remote.URL, g.Repo)
		if err := g.network(ctx, env, remote.Token, argv); err != nil {
			return "", err
		}
	} else {
		argv := append(g.prefix(), "-C", g.Repo, "fetch", "--no-tags", "--", remote.URL, "refs/heads/"+base)
		if err := g.network(ctx, env, remote.Token, argv); err != nil {
			return "", err
		}
		if _, err := g.run(ctx, append(g.prefix(), "-C", g.Repo, "checkout", "--force", "-B", base, "FETCH_HEAD")...); err != nil {
			return "", err
		}
	}
	return g.Head(ctx)
}

// Push sends orch/<cardID> to the same branch on remote. The refspec names
// only the card branch, and the token reaches git through askpass.
func (g Git) Push(ctx context.Context, cardID string, remote Remote) error {
	if err := validCardID(cardID); err != nil {
		return err
	}
	if !strings.HasPrefix(remote.URL, "https://") {
		return fmt.Errorf("remote must be https")
	}
	if err := writeAskpass(g.Data); err != nil {
		return err
	}
	env := append(gitEnv(g.Bin, g.Data),
		"GIT_ASKPASS="+AskpassPath(g.Data),
		"ORCH_ASKPASS=1",
		"GH_TOKEN="+remote.Token,
	)
	ref := "refs/heads/orch/" + cardID
	argv := append(g.prefix(), "-C", g.Repo, "push", "--", remote.URL, ref+":"+ref)
	return g.network(ctx, env, remote.Token, argv)
}

func (g Git) network(ctx context.Context, env []string, token string, argv []string) error {
	if token != "" {
		for _, arg := range argv {
			if strings.Contains(arg, token) {
				return ErrTokenInArgv
			}
		}
	}
	_, err := g.runEnv(ctx, env, netTimeout, argv...)
	if err != nil && token != "" {
		return errors.New(strings.ReplaceAll(err.Error(), token, "[redacted]"))
	}
	return err
}

// Cloned reports whether the project clone exists.
func (g Git) Cloned() bool {
	_, err := os.Stat(filepath.Join(g.Repo, ".git"))
	return err == nil
}

// ValidBranch follows the rules of git check-ref-format --branch, and refuses orch/*.
func ValidBranch(name string) bool {
	if name == "" || len(name) > 200 || strings.HasPrefix(name, "-") || strings.HasPrefix(name, "orch/") {
		return false
	}
	if strings.HasPrefix(name, "/") || strings.HasSuffix(name, "/") || strings.HasSuffix(name, ".") ||
		strings.HasSuffix(name, ".lock") || strings.Contains(name, "..") || strings.Contains(name, "//") ||
		strings.Contains(name, "@{") || name == "@" {
		return false
	}
	for _, c := range name {
		if c < 0x20 || c == 0x7f || strings.ContainsRune(" ~^:?*[\\", c) {
			return false
		}
	}
	for _, part := range strings.Split(name, "/") {
		if strings.HasPrefix(part, ".") {
			return false
		}
	}
	return true
}
