package runtime

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"orchestrator/backend/internal/execx"
	"orchestrator/backend/internal/sandbox"
)

const (
	gitTimeout = 2 * time.Minute
	gitMaxOut  = 1 << 20
	errorCap   = 2 << 10
)

type Git struct {
	Bin    string
	Repo   string
	Data   string
	Author Author
	Exec   func(context.Context, execx.Child) (execx.Result, error)
}

// Author is the name and email on every commit, as author and committer.
type Author struct {
	Name  string
	Email string
}

// LoginAuthor names a GitHub login with its noreply address. A positive id
// gives the <id>+<login> form GitHub links to the account (RFC-054).
func LoginAuthor(login string, id int64) Author {
	if id > 0 {
		return Author{Name: login, Email: fmt.Sprintf("%d+%s@users.noreply.github.com", id, login)}
	}
	return Author{Name: login, Email: login + "@users.noreply.github.com"}
}

type Worktree struct {
	Branch string
	Path   string
	GitDir string
}

func (g Git) EnsureWorktree(ctx context.Context, cardID, baseBranch string) (Worktree, error) {
	if err := validCardID(cardID); err != nil {
		return Worktree{}, err
	}
	branch := "orch/" + cardID
	wtPath := filepath.Join(g.Data, "worktrees", cardID)
	if info, err := os.Stat(wtPath); err == nil && info.IsDir() {
		gitDir, err := readGitDir(filepath.Join(wtPath, ".git"))
		if err != nil {
			return Worktree{}, err
		}
		return Worktree{Branch: branch, Path: wtPath, GitDir: gitDir}, nil
	}
	if err := os.MkdirAll(filepath.Dir(wtPath), 0o700); err != nil {
		return Worktree{}, err
	}
	if _, err := g.run(ctx, append(g.prefix(), "-C", g.Repo, "worktree", "add", "-b", branch, wtPath, baseBranch)...); err != nil {
		return Worktree{}, err
	}
	gitDir, err := readGitDir(filepath.Join(wtPath, ".git"))
	if err != nil {
		return Worktree{}, err
	}
	return Worktree{Branch: branch, Path: wtPath, GitDir: gitDir}, nil
}

func (g Git) Commit(ctx context.Context, wt Worktree, message string) error {
	if _, err := g.run(ctx, g.worktreeArgv(wt, "add", "-A")...); err != nil {
		return err
	}
	_, err := g.run(ctx, g.commitArgv(wt, "commit", "-m", message)...)
	return err
}

// DiscardChanges drops uncommitted edits and untracked files from the
// worktree, so an attempt starts from the worktree's last commit.
func (g Git) DiscardChanges(ctx context.Context, wt Worktree) error {
	if _, err := g.run(ctx, g.worktreeArgv(wt, "reset", "--hard", "--quiet", "HEAD")...); err != nil {
		return err
	}
	_, err := g.run(ctx, g.worktreeArgv(wt, "clean", "-fd", "--quiet")...)
	return err
}

func (g Git) Status(ctx context.Context, repo string) error {
	_, err := g.run(ctx, append(g.prefix(), "-C", repo, "status", "--porcelain")...)
	return err
}

func (g Git) WorktreeHead(ctx context.Context, wt Worktree) (string, error) {
	res, err := g.run(ctx, g.worktreeArgv(wt, "rev-parse", "HEAD")...)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(res.Out)), nil
}

func (g Git) Head(ctx context.Context) (string, error) {
	res, err := g.run(ctx, append(g.prefix(), "-C", g.Repo, "rev-parse", "HEAD")...)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(res.Out)), nil
}

func (g Git) RevParse(ctx context.Context, ref string) (string, error) {
	res, err := g.run(ctx, append(g.prefix(), "-C", g.Repo, "rev-parse", "--verify", "--end-of-options", ref+"^{commit}")...)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(res.Out)), nil
}

// Export writes the commit's tree to dest. git writes the archive to a file
// beside dest, so the size of the repository is not bounded by gitMaxOut.
func (g Git) Export(ctx context.Context, sha, dest string) error {
	if err := os.MkdirAll(dest, 0o700); err != nil {
		return err
	}
	tarPath, err := filepath.Abs(filepath.Clean(dest) + ".tar")
	if err != nil {
		return err
	}
	defer os.Remove(tarPath)
	if _, err := g.run(ctx, append(g.prefix(), "-C", g.Repo, "archive", "--format=tar", "--output="+tarPath, sha)...); err != nil {
		return err
	}
	f, err := os.Open(tarPath)
	if err != nil {
		return err
	}
	defer f.Close()
	return sandbox.ExtractArchive(f, dest)
}

// Diff returns the diff from the merge base of base and head to head, cut at
// max bytes. git writes it to a file under the data directory, so a large diff
// is not bounded by gitMaxOut. truncated reports a cut.
func (g Git) Diff(ctx context.Context, base, head string, max int) (string, bool, error) {
	dir := filepath.Join(g.Data, "exports")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", false, err
	}
	file, err := os.CreateTemp(dir, "diff-*.txt")
	if err != nil {
		return "", false, err
	}
	path := file.Name()
	file.Close()
	defer os.Remove(path)
	if _, err := g.run(ctx, append(g.prefix(), "-C", g.Repo, "diff", "--no-color", "--no-ext-diff", "--output="+path,
		"--end-of-options", base+"..."+head)...); err != nil {
		return "", false, err
	}
	f, err := os.Open(path)
	if err != nil {
		return "", false, err
	}
	defer f.Close()
	buf := make([]byte, max+1)
	n, err := io.ReadFull(f, buf)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return "", false, err
	}
	if n > max {
		return strings.ToValidUTF8(string(buf[:max]), ""), true, nil
	}
	return string(buf[:n]), false, nil
}

func (g Git) run(ctx context.Context, argv ...string) (execx.Result, error) {
	return g.runEnv(ctx, gitEnv(g.Bin, g.Data), gitTimeout, argv...)
}

func (g Git) runEnv(ctx context.Context, env []string, timeout time.Duration, argv ...string) (execx.Result, error) {
	run := g.Exec
	if run == nil {
		run = execx.Command
	}
	res, err := run(ctx, execx.Child{
		Argv:    argv,
		Env:     env,
		Timeout: timeout,
		MaxOut:  gitMaxOut,
	})
	if err != nil {
		return res, err
	}
	if res.Exit != 0 {
		return res, fmt.Errorf("git exit %d: %s", res.Exit, errorText(res.Out))
	}
	return res, nil
}

// errorText keeps the first errorCap bytes of git output for an error message.
func errorText(out []byte) string {
	if len(out) <= errorCap {
		return string(out)
	}
	return strings.ToValidUTF8(string(out[:errorCap]), "") + " (truncated)"
}

func (g Git) prefix() []string {
	return []string{
		g.Bin,
		"-c", "core.hooksPath=" + filepath.Join(g.Data, "empty-hooks"),
		"-c", "core.fsmonitor=false",
		"-c", "protocol.file.allow=never",
		"-c", "credential.helper=",
		"-c", "commit.gpgSign=false",
	}
}

func (g Git) worktreeArgv(wt Worktree, args ...string) []string {
	argv := g.prefix()
	argv = append(argv, "--git-dir="+wt.GitDir, "--work-tree="+wt.Path)
	return append(argv, args...)
}

func (g Git) commitArgv(wt Worktree, args ...string) []string {
	argv := g.prefix()
	argv = append(argv,
		"-c", "user.name="+g.Author.Name,
		"-c", "user.email="+g.Author.Email,
		"--git-dir="+wt.GitDir,
		"--work-tree="+wt.Path,
	)
	return append(argv, args...)
}

func gitEnv(gitBin, data string) []string {
	return []string{
		"PATH=" + filepath.Dir(gitBin),
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_TERMINAL_PROMPT=0",
		"HOME=" + filepath.Join(data, "empty-gh"),
	}
}

func readGitDir(path string) (string, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	line := strings.TrimSpace(string(body))
	const prefix = "gitdir:"
	if !strings.HasPrefix(line, prefix) {
		return "", fmt.Errorf("worktree gitdir missing in %s", path)
	}
	return strings.TrimSpace(strings.TrimPrefix(line, prefix)), nil
}

func validCardID(id string) error {
	if len(id) != len("card_")+12 || !strings.HasPrefix(id, "card_") {
		return fmt.Errorf("card id %q is invalid", id)
	}
	for _, c := range id[len("card_"):] {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return fmt.Errorf("card id %q is invalid", id)
		}
	}
	return nil
}

func FixtureMode(repo string) bool {
	resolved, err := filepath.EvalSymlinks(repo)
	if err != nil {
		resolved = filepath.Clean(repo)
	}
	return strings.HasSuffix(resolved, "/testdata/fixture-repo")
}

func Passed(exit int, modelNote string) bool {
	return exit == 0
}

func RunCheck(ctx context.Context, repo, command string, allow []string, dir, modelNote string) (int, bool, error) {
	res, executed, err := sandbox.ExecuteCheck(ctx, sandbox.ExecuteOpts{
		Command: command,
		Allow:   allow,
		Fixture: FixtureMode(repo),
		Dir:     dir,
		Timeout: gitTimeout,
		MaxOut:  gitMaxOut,
	}, execx.Command)
	if err != nil {
		return 0, false, err
	}
	if !executed {
		return 0, false, fmt.Errorf("check was not executed")
	}
	return res.Exit, Passed(res.Exit, modelNote), nil
}
