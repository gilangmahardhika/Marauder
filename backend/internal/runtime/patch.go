package runtime

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	// MaxPatchFile is the largest file a handoff patch carries (RFC-118).
	MaxPatchFile = 1 << 20
	// MaxPatchBytes is the largest handoff patch; a larger one is dropped.
	MaxPatchBytes = 2 << 20
)

// WorkPatch is a worktree's uncommitted change against HEAD. Files are the
// paths the patch holds, Skipped those left out for size. Dropped means the
// patch was over MaxPatchBytes, so Patch is empty.
type WorkPatch struct {
	Patch   string
	Base    string
	Files   []string
	Skipped []string
	Dropped bool
}

// WorkPatch builds the worktree's uncommitted change as a binary patch
// through a temporary index (read-tree HEAD, add -A, diff --cached HEAD), so
// the worktree's own index does not change.
func (g Git) WorkPatch(ctx context.Context, wt Worktree) (WorkPatch, error) {
	var out WorkPatch
	base, err := g.WorktreeHead(ctx, wt)
	if err != nil {
		return out, err
	}
	out.Base = base
	dir := filepath.Join(g.Data, "exports")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return out, err
	}
	file, err := os.CreateTemp(dir, "index-*")
	if err != nil {
		return out, err
	}
	index := file.Name()
	file.Close()
	os.Remove(index)
	defer os.Remove(index)
	env := append(gitEnv(g.Bin, g.Data), "GIT_INDEX_FILE="+index)
	if _, err := g.runEnv(ctx, env, gitTimeout, g.worktreeArgv(wt, "read-tree", "HEAD")...); err != nil {
		return out, err
	}
	if _, err := g.runEnv(ctx, env, gitTimeout, g.worktreeArgv(wt, "add", "-A")...); err != nil {
		return out, err
	}
	names, err := g.cachedNames(ctx, env, wt)
	if err != nil {
		return out, err
	}
	for _, name := range names {
		info, err := os.Lstat(filepath.Join(wt.Path, name))
		if err == nil && info.Mode().IsRegular() && info.Size() > MaxPatchFile {
			out.Skipped = append(out.Skipped, name)
		}
	}
	if len(out.Skipped) > 0 {
		argv := append(g.worktreeArgv(wt, "reset", "--quiet", "HEAD", "--"), out.Skipped...)
		if _, err := g.runEnv(ctx, env, gitTimeout, argv...); err != nil {
			return out, err
		}
		if names, err = g.cachedNames(ctx, env, wt); err != nil {
			return out, err
		}
	}
	if len(names) == 0 {
		return out, nil
	}
	patchFile, err := os.CreateTemp(dir, "patch-*.diff")
	if err != nil {
		return out, err
	}
	path := patchFile.Name()
	patchFile.Close()
	defer os.Remove(path)
	if _, err := g.runEnv(ctx, env, gitTimeout, g.worktreeArgv(wt, "diff", "--cached", "--binary", "--no-color", "--no-ext-diff",
		"--no-renames", "--output="+path, "HEAD")...); err != nil {
		return out, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return out, err
	}
	out.Files = names
	if info.Size() > MaxPatchBytes {
		out.Dropped = true
		return out, nil
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return out, err
	}
	out.Patch = string(body)
	return out, nil
}

func (g Git) cachedNames(ctx context.Context, env []string, wt Worktree) ([]string, error) {
	res, err := g.runEnv(ctx, env, gitTimeout, g.worktreeArgv(wt, "diff", "--cached", "--name-only", "--no-renames", "-z", "HEAD")...)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, name := range strings.Split(string(res.Out), "\x00") {
		if name != "" {
			names = append(names, name)
		}
	}
	return names, nil
}

// ErrPatch is a handoff patch that did not apply to the worktree.
var ErrPatch = errors.New("the patch does not apply")

// ApplyPatch applies a handoff patch to the worktree: as is when HEAD is the
// patch's base, otherwise with a three-way merge. A patch that does not apply
// leaves the worktree as it was. threeWay reports the merge.
func (g Git) ApplyPatch(ctx context.Context, wt Worktree, patch, base string) (threeWay bool, err error) {
	head, err := g.WorktreeHead(ctx, wt)
	if err != nil {
		return false, err
	}
	dir := filepath.Join(g.Data, "exports")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return false, err
	}
	file, err := os.CreateTemp(dir, "apply-*.diff")
	if err != nil {
		return false, err
	}
	path := file.Name()
	defer os.Remove(path)
	if _, err := file.WriteString(patch); err != nil {
		file.Close()
		return false, err
	}
	if err := file.Close(); err != nil {
		return false, err
	}
	threeWay = head != base
	args := []string{"apply", "--binary", "--whitespace=nowarn"}
	if threeWay {
		args = append(args, "--3way")
	}
	if _, err := g.runIn(ctx, wt.Path, g.worktreeArgv(wt, append(args, path)...)...); err != nil {
		if derr := g.DiscardChanges(ctx, wt); derr != nil {
			return threeWay, derr
		}
		return threeWay, errors.Join(ErrPatch, err)
	}
	if threeWay {
		if _, err := g.run(ctx, g.worktreeArgv(wt, "reset", "--quiet")...); err != nil {
			return threeWay, err
		}
	}
	return threeWay, nil
}

// Commits lists the one-line subjects of the commits on HEAD that base does
// not have, newest first, at most max.
func (g Git) Commits(ctx context.Context, wt Worktree, base string, max int) ([]string, error) {
	res, err := g.run(ctx, g.worktreeArgv(wt, "log", "--no-color", "--format=%h %s", "-n", strconv.Itoa(max), "--end-of-options", base+"..HEAD")...)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, line := range strings.Split(strings.TrimRight(string(res.Out), "\n"), "\n") {
		if line != "" {
			out = append(out, line)
		}
	}
	return out, nil
}
