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
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5"

	"orchestrator/backend/internal/app"
	"orchestrator/backend/internal/board"
	"orchestrator/backend/internal/dbtools"
	"orchestrator/backend/internal/execx"
	"orchestrator/backend/internal/httpapi"
	"orchestrator/backend/internal/mcpcatalog"
	"orchestrator/backend/internal/netguard"
	"orchestrator/backend/internal/notify"
	"orchestrator/backend/internal/runtime"
	"orchestrator/backend/internal/sandbox"
	"orchestrator/backend/internal/tenancy"
	"orchestrator/backend/internal/webpush"
	"orchestrator/backend/internal/worker"
)

type resolvedPaths struct {
	Repo string
	Data string
}

type secretFile struct {
	DatabaseURL     string            `json:"databaseUrl"`
	TokenKey        string            `json:"tokenKey,omitempty"`
	VAPIDPrivateKey string            `json:"vapidPrivateKey,omitempty"`
	LLMKeys         map[string]string `json:"llmKeys"`
}

// jobSecrets are the service secrets a job's events and packs redact.
func (f secretFile) jobSecrets() []string {
	out := []string{f.DatabaseURL, f.TokenKey}
	if f.VAPIDPrivateKey != "" {
		out = append(out, f.VAPIDPrivateKey)
	}
	return out
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
	secrets, err := loadSecrets(paths.Data, opts.getenv)
	if err != nil {
		return err
	}
	if opts.Open != nil {
		return opts.Open(secrets.DatabaseURL)
	}
	db, err := board.Open(secrets.DatabaseURL)
	if err != nil {
		return scrub(err, secrets.DatabaseURL)
	}
	defer db.Close()
	if err := board.Migrate(db); err != nil {
		return scrub(err, secrets.DatabaseURL)
	}
	if err := bootstrapAdmin(opts, db); err != nil {
		fmt.Fprintf(opts.Stderr, "admin bootstrap: %s\n", err)
	}
	key, err := hex.DecodeString(secrets.TokenKey)
	if err != nil || len(key) != 32 {
		return errors.New("tokenKey must be 32 bytes")
	}
	if err := tenancy.AdoptProjects(context.Background(), db, paths.Data); err != nil {
		return err
	}
	srv := httpapi.New()
	srv.DB = db
	srv.Data = paths.Data
	srv.TokenKey = key
	srv.Tools = jobTools()
	srv.Secrets = secrets.jobSecrets()
	guard, err := dbtools.BoardGuard(secrets.DatabaseURL)
	if err != nil {
		return scrub(err, secrets.DatabaseURL)
	}
	srv.MCP.Board = guard
	if engine := discoverEngine(srv.Tools); engine != "" {
		go func() { _ = sandbox.SweepMCP(context.Background(), engine, "discover", execx.Command) }()
		srv.MCP.Stdio = app.StdioOpener(engine, false, "discover", srv.Tools)
		srv.Images = app.ImagePuller{Engine: engine, Fixture: runtime.FixtureMode(paths.Repo), Run: srv.Tools.Exec}
	}
	srv.Catalog = mcpcatalog.New(mcpcatalog.DefaultRegistry, srv.MCP.Policy)
	srv.Remote = app.NewRemoteJobs()
	srv.RunnerReleases = opts.getenv("ORCHESTRATOR_RUNNER_RELEASES")
	if os.Getenv("ORCHESTRATOR_CLOUD") == "1" {
		srv.Cloud = true
		srv.AllowHosts = hostAllowlist(opts.getenv)
		if origin := opts.getenv("ORCHESTRATOR_UI_ORIGIN"); origin != "" {
			srv.AllowOrigin = origin
		}
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	done := make(chan struct{})
	go func() {
		defer close(done)
		jobs := app.ImplementJobs(db, app.JobDeps{Data: paths.Data, Key: key, Tools: srv.Tools, Secrets: secrets.jobSecrets(), Live: srv.Live, MCP: app.MCPDialer{Board: guard}})
		jobs.Reap = func(ctx context.Context) (int, error) { return app.ReapRemote(ctx, db, srv.Remote) }
		worker.Run(ctx, db, paths.Data, 2*time.Second, jobs, func(format string, args ...any) {
			fmt.Fprintf(opts.Stderr, format+"\n", args...)
		})
	}()
	logf := func(format string, args ...any) {
		fmt.Fprintln(opts.Stderr, scrub(fmt.Errorf(format, args...), secrets.DatabaseURL))
	}
	notifier := &notify.Notifier{DB: db, Data: paths.Data, Logf: logf}
	if vapid, err := webpush.ParseKey(secrets.VAPIDPrivateKey); err == nil {
		pusher := &webpush.Pusher{DB: db, Logf: logf, Sender: &webpush.Sender{
			Client:  netguard.Client(netguard.Policy{}, nil),
			Key:     vapid,
			Subject: vapidSubject(db),
		}}
		if srv.PushKey, err = webpush.PublicKey(vapid); err == nil {
			srv.Push = pusher
			notifier.Push = pusher.Push
		}
	}
	listener := &notify.Listener{
		DB: db, Hub: srv.Notify, Events: notifier.Fanout, Logf: logf,
		Connect: func(ctx context.Context) (*pgx.Conn, error) { return pgx.Connect(ctx, secrets.DatabaseURL) },
	}
	listening := make(chan struct{})
	go func() {
		defer close(listening)
		listener.Run(ctx)
	}()
	server := &http.Server{Addr: addr, Handler: srv.Handler()}
	server.RegisterOnShutdown(srv.Live.Close)
	server.RegisterOnShutdown(srv.Notify.Close)
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdown)
	}()
	err = server.ListenAndServe()
	stop()
	<-done
	<-listening
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

// lookTool returns an absolute path, or "" when the tool is not installed.
// The fallbacks are absolute install paths checked when PATH has no match.
func lookTool(name string, fallbacks ...string) string {
	path, err := exec.LookPath(name)
	if err != nil {
		for _, candidate := range fallbacks {
			if info, statErr := os.Stat(candidate); statErr == nil && !info.IsDir() && info.Mode()&0o111 != 0 {
				return candidate
			}
		}
		return ""
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return ""
	}
	return abs
}

func jobTools() app.Tools {
	return app.Tools{
		Git:    lookTool("git"),
		GH:     lookTool("gh"),
		Podman: lookTool("podman", "/opt/podman/bin/podman", "/opt/homebrew/bin/podman", "/usr/local/bin/podman"),
		Docker: lookTool("docker", "/usr/local/bin/docker", "/opt/homebrew/bin/docker"),
	}
}

// discoverEngine runs stdio servers for tool discovery, which has no project:
// podman when installed, else docker.
func discoverEngine(tools app.Tools) string {
	if tools.Podman != "" {
		return tools.Podman
	}
	return tools.Docker
}

func bootstrapAdmin(opts Options, db *sql.DB) error {
	email := strings.TrimSpace(opts.getenv("ORCHESTRATOR_ADMIN_EMAIL"))
	password := opts.getenv("ORCHESTRATOR_ADMIN_PASSWORD")
	if email == "" && password == "" {
		return nil
	}
	if email == "" || password == "" {
		return errors.New("set both ORCHESTRATOR_ADMIN_EMAIL and ORCHESTRATOR_ADMIN_PASSWORD")
	}
	_, err := app.Bootstrap(context.Background(), db, email, password)
	if errors.Is(err, app.ErrAdminExists) {
		return nil
	}
	if err == nil {
		fmt.Fprintln(opts.Stdout, "admin created")
	}
	return err
}

// vapidSubject is the contact push services see: the first admin's email.
func vapidSubject(db *sql.DB) string {
	var email string
	if err := db.QueryRow(`SELECT email FROM users WHERE admin ORDER BY created_at LIMIT 1`).Scan(&email); err != nil || email == "" {
		return "mailto:orchestrator@localhost"
	}
	return "mailto:" + email
}

func hostAllowlist(getenv func(string) string) map[string]bool {
	raw := getenv("ORCHESTRATOR_HOSTS")
	if raw == "" {
		raw = "127.0.0.1,localhost"
	}
	out := map[string]bool{}
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			out[part] = true
		}
	}
	return out
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
	if os.Getenv("ORCHESTRATOR_CLOUD") == "1" {
		return nil
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
	file, err := loadSecrets(data, getenv)
	if err != nil {
		return "", err
	}
	return file.DatabaseURL, nil
}

func loadSecrets(data string, getenv func(string) string) (secretFile, error) {
	path := filepath.Join(data, "secrets.json")
	body, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		key, err := newTokenKey()
		if err != nil {
			return secretFile{}, err
		}
		file := secretFile{
			DatabaseURL: getenv("ORCHESTRATOR_DATABASE_URL"),
			TokenKey:    key,
			LLMKeys: map[string]string{
				"anthropic": "",
				"openai":    "",
			},
		}
		if err := writeSecretFile(path, file); err != nil {
			return secretFile{}, err
		}
		body, err = os.ReadFile(path)
		if err != nil {
			return secretFile{}, err
		}
	} else if err != nil {
		return secretFile{}, err
	}
	var file secretFile
	if err := json.Unmarshal(body, &file); err != nil {
		return secretFile{}, err
	}
	changed := false
	if strings.TrimSpace(file.TokenKey) == "" {
		key, err := newTokenKey()
		if err != nil {
			return secretFile{}, err
		}
		file.TokenKey = key
		changed = true
	}
	if _, err := webpush.ParseKey(file.VAPIDPrivateKey); err != nil {
		key, err := webpush.GenerateKey()
		if err != nil {
			return secretFile{}, err
		}
		if file.VAPIDPrivateKey, err = webpush.EncodeKey(key); err != nil {
			return secretFile{}, err
		}
		changed = true
	}
	if changed {
		if err := writeSecretFile(path, file); err != nil {
			return secretFile{}, err
		}
	}
	if strings.TrimSpace(file.DatabaseURL) == "" {
		return secretFile{}, errors.New("databaseUrl is empty")
	}
	if strings.TrimSpace(file.TokenKey) == "" {
		return secretFile{}, errors.New("tokenKey is empty")
	}
	return file, nil
}

func newTokenKey() (string, error) {
	var buf [32]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf[:]), nil
}

func writeSecretFile(path string, file secretFile) error {
	raw, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(path, 0o600, append(raw, '\n'))
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
