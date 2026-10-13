package sandbox

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"time"

	"orchestrator/backend/internal/execx"
)

const pullTimeout = 5 * time.Minute

var (
	ErrImageRef = errors.New("image reference is not valid")
	ErrPull     = errors.New("the engine could not pull the image")

	imageRefPattern = regexp.MustCompile(`^[a-z0-9]+(?:[._-][a-z0-9]+)*(?::[0-9]{1,5})?(?:/[a-z0-9]+(?:(?:[._]|__|-+)[a-z0-9]+)*)*(?::[A-Za-z0-9_][A-Za-z0-9_.-]{0,127})?(?:@sha256:[a-f0-9]{64})?$`)
)

// ImageRef reports whether ref is an image reference: a lowercase name with
// an optional registry port, tag, and digest. It never starts with '-'.
func ImageRef(ref string) bool {
	return len(ref) <= 255 && imageRefPattern.MatchString(ref)
}

// repository is ref without its tag or digest.
func repository(ref string) string {
	if i := strings.Index(ref, "@"); i >= 0 {
		ref = ref[:i]
	}
	slash := strings.LastIndex(ref, "/")
	if i := strings.LastIndex(ref, ":"); i > slash {
		ref = ref[:i]
	}
	return ref
}

// PullImage pulls ref with engine and returns it pinned by its repository
// digest (RFC-098).
func PullImage(ctx context.Context, engine, ref string, run func(context.Context, execx.Child) (execx.Result, error)) (string, error) {
	if err := CheckEngine(engine); err != nil {
		return "", err
	}
	if !ImageRef(ref) {
		return "", ErrImageRef
	}
	env := EngineEnv(engine)
	res, err := run(ctx, execx.Child{Argv: []string{engine, "pull", ref}, Env: env, Timeout: pullTimeout, MaxOut: 64 << 10})
	if err != nil || res.Exit != 0 {
		return "", ErrPull
	}
	res, err = run(ctx, execx.Child{
		Argv: []string{engine, "image", "inspect", "--format", "{{json .RepoDigests}}", ref},
		Env:  env, Timeout: 30 * time.Second, MaxOut: 64 << 10,
	})
	if err != nil || res.Exit != 0 {
		return "", ErrPull
	}
	var digests []string
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(res.Out))), &digests); err != nil {
		return "", ErrPull
	}
	repo := repository(ref)
	pinned := ""
	for _, d := range digests {
		name := d[:max(strings.Index(d, "@"), 0)]
		if name == repo || strings.HasSuffix(name, "/"+repo) {
			pinned = d
			break
		}
	}
	if pinned == "" && len(digests) > 0 {
		pinned = digests[0]
	}
	if !PinnedImage(pinned) {
		return "", ErrPull
	}
	return pinned, nil
}
