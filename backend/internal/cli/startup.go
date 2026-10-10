package cli

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"orchestrator/backend/internal/board"
	"orchestrator/backend/internal/httpapi"
	"orchestrator/backend/internal/worker"
)

type resolvedPaths struct {
	Repo string
	Data string
}

type secretFile struct {
	DatabaseURL string            `json:"databaseUrl"`
	GitHubToken string            `json:"githubToken"`
	DeployToken string            `json:"deployToken"`
	LLMKeys     map[string]string `json:"llmKeys"`
}

func withBoard(opts Options, repoFlag, dataFlag string, fn func(context.Context, *sql.DB, resolvedPaths) error) error {
	paths, err := resolvePaths(opts, repoFlag, dataFlag)
	if err != nil {
		return err
	}
	if err := prepareDataDir(paths.Data); err != nil {
		return err
	}
	url, err := loadDatabaseURL(paths.Data, opts.getenv)
	if err != nil {
		return err
	}
	db, err := board.Open(url)
	if err != nil {
		return scrub(err, url)
	}
	defer db.Close()
	if err := board.Migrate(db); err != nil {
		return scrub(err, url)
	}
	return fn(context.Background(), db, paths)
}

func runServe(opts Options, repoFlag, dataFlag, addr string) error {
	paths, err := resolvePaths(opts, repoFlag, dataFlag)
	if err != nil {
		return err
	}
	if err := checkLoopback(addr); err != nil {
		return err
	}
	if err := prepareDataDir(paths.Data); err != nil {
		return err
	}
	lock, err := worker.Lock(filepath.Join(paths.Data, "worker.lock"))
	if err != nil {
		return err
	}
	defer lock.Close()
	url, err := loadDatabaseURL(paths.Data, opts.getenv)
	if err != nil {
		return err
	}
	if opts.Open != nil {
		return opts.Open(url)
	}
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return err
	}
	token := hex.EncodeToString(buf[:])
	fmt.Fprintf(opts.Stdout, "UI token: %s\n", token)
	db, err := board.Open(url)
	if err != nil {
		return scrub(err, url)
	}
	defer db.Close()
	if err := board.Migrate(db); err != nil {
		return scrub(err, url)
	}
	srv := httpapi.New(token)
	srv.DB = db
	srv.Data = paths.Data
	return http.ListenAndServe(addr, srv.Handler())
}

func resolvePaths(opts Options, repoFlag, dataFlag string) (resolvedPaths, error) {
	repo := repoFlag
	if repo == "" {
		var err error
		repo, err = opts.getwd()
		if err != nil {
			return resolvedPaths{}, err
		}
	}
	resolvedRepo, err := resolvePath(repo)
	if err != nil {
		return resolvedPaths{}, err
	}
	data := dataFlag
	if data == "" {
		data, err = defaultData(resolvedRepo)
		if err != nil {
			return resolvedPaths{}, err
		}
	}
	resolvedData, err := resolvePath(data)
	if err != nil {
		return resolvedPaths{}, err
	}
	if pathContains(resolvedRepo, resolvedData) || pathContains(resolvedData, resolvedRepo) {
		return resolvedPaths{}, fmt.Errorf("data directory %s must be outside repo %s", resolvedData, resolvedRepo)
	}
	return resolvedPaths{Repo: resolvedRepo, Data: resolvedData}, nil
}

func resolvePath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	abs = filepath.Clean(abs)
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		return resolved, nil
	}
	var missing []string
	cur := abs
	for {
		parent := filepath.Dir(cur)
		if parent == cur {
			return "", fmt.Errorf("resolve %s: no existing ancestor", path)
		}
		missing = append(missing, filepath.Base(cur))
		_, statErr := os.Lstat(parent)
		if statErr == nil {
			resolved, err := filepath.EvalSymlinks(parent)
			if err != nil {
				return "", err
			}
			for i := len(missing) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, missing[i])
			}
			return resolved, nil
		}
		if !os.IsNotExist(statErr) {
			return "", statErr
		}
		cur = parent
	}
}

func pathContains(parent, child string) bool {
	rel, err := filepath.Rel(parent, child)
	if err != nil {
		return false
	}
	if rel == "." {
		return true
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func defaultData(repo string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(repo))
	hash8 := hex.EncodeToString(sum[:])[:8]
	return filepath.Join(home, ".orchestrator", filepath.Base(repo)+"-"+hash8), nil
}

func checkLoopback(addr string) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("refusing non-loopback addr %s", addr)
	}
	if host != "127.0.0.1" && host != "::1" {
		return fmt.Errorf("refusing non-loopback addr %s", addr)
	}
	return nil
}

func prepareDataDir(data string) error {
	if err := os.MkdirAll(data, 0o700); err != nil {
		return err
	}
	info, err := os.Stat(data)
	if err != nil {
		return err
	}
	if info.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("data directory %s is group or world accessible; chmod 700 %s", data, data)
	}
	for _, name := range []string{"empty-hooks", "empty-gh", "askpass", "worktrees", "exports", "logs"} {
		if err := os.MkdirAll(filepath.Join(data, name), 0o700); err != nil {
			return err
		}
	}
	return nil
}

func loadDatabaseURL(data string, getenv func(string) string) (string, error) {
	path := filepath.Join(data, "secrets.json")
	body, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		file := secretFile{
			DatabaseURL: getenv("ORCHESTRATOR_DATABASE_URL"),
			LLMKeys: map[string]string{
				"anthropic": "",
				"openai":    "",
			},
		}
		raw, err := json.MarshalIndent(file, "", "  ")
		if err != nil {
			return "", err
		}
		raw = append(raw, '\n')
		if err := writeAtomic(path, 0o600, raw); err != nil {
			return "", err
		}
		body = raw
	} else if err != nil {
		return "", err
	}
	var file secretFile
	if err := json.Unmarshal(body, &file); err != nil {
		return "", err
	}
	if strings.TrimSpace(file.DatabaseURL) == "" {
		return "", errors.New("databaseUrl is empty")
	}
	return file.DatabaseURL, nil
}

func writeAtomic(path string, mode os.FileMode, body []byte) error {
	dir := filepath.Dir(path)
	f, err := os.CreateTemp(dir, ".secrets-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if err := f.Chmod(mode); err != nil {
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

func openBoard(url string) error {
	db, err := board.Open(url)
	if err != nil {
		return scrub(err, url)
	}
	defer db.Close()
	if err := board.Migrate(db); err != nil {
		return scrub(err, url)
	}
	return nil
}

func scrub(err error, secret string) error {
	msg := err.Error()
	if secret != "" {
		msg = strings.ReplaceAll(msg, secret, "[redacted]")
	}
	return errors.New(msg)
}
