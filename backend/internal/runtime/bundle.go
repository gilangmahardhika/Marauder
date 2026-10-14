package runtime

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// ErrBundle is a bundle that does not hold exactly the card branch at the
// expected commit, or whose commit does not descend from the attempt base.
var ErrBundle = errors.New("bundle is not the card branch at the attempt commit")

// BundleCard writes a git bundle of refs/heads/orch/<cardID> to dest, holding
// only the commits after the branch's merge base with base (RFC-108).
func (g Git) BundleCard(ctx context.Context, cardID, base, dest string) error {
	if err := validCardID(cardID); err != nil {
		return err
	}
	ref := "refs/heads/orch/" + cardID
	res, err := g.run(ctx, append(g.prefix(), "-C", g.Repo, "merge-base", ref, "refs/heads/"+base)...)
	if err != nil {
		return err
	}
	mergeBase := strings.TrimSpace(string(res.Out))
	argv := append(g.prefix(), "-C", g.Repo, "bundle", "create", "--quiet", dest, ref)
	if mergeBase != "" {
		argv = append(argv, "^"+mergeBase)
	}
	_, err = g.run(ctx, argv...)
	return err
}

// SeedCardBranch points orch/<cardID> at FETCH_HEAD, which FetchCard left at
// the published card branch, unless the local branch already contains it.
// A runner that did not run the card's last attempt starts from what the pull
// request holds (RFC-108).
func (g Git) SeedCardBranch(ctx context.Context, cardID string) error {
	if err := validCardID(cardID); err != nil {
		return err
	}
	res, err := g.run(ctx, append(g.prefix(), "-C", g.Repo, "rev-parse", "--verify", "FETCH_HEAD^{commit}")...)
	if err != nil {
		return err
	}
	fetched := strings.TrimSpace(string(res.Out))
	ref := "refs/heads/orch/" + cardID
	if _, err := g.run(ctx, append(g.prefix(), "-C", g.Repo, "merge-base", "--is-ancestor", fetched, ref)...); err == nil {
		return nil
	}
	_, err = g.run(ctx, append(g.prefix(), "-C", g.Repo, "update-ref", ref, fetched)...)
	return err
}

// ImportCardBundle checks a runner's bundle and moves orch/<cardID> to sha.
// The bundle must list one head, refs/heads/orch/<cardID> at sha, and sha must
// descend from baseSHA. No hook or filter of the bundle runs: unbundle only
// writes objects, and the ref is set here.
func (g Git) ImportCardBundle(ctx context.Context, cardID, file, sha, baseSHA string) error {
	if err := validCardID(cardID); err != nil {
		return err
	}
	if len(sha) < 40 || strings.Trim(sha, "0123456789abcdef") != "" {
		return ErrBundle
	}
	ref := "refs/heads/orch/" + cardID
	res, err := g.run(ctx, append(g.prefix(), "-C", g.Repo, "bundle", "list-heads", file)...)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrBundle, err)
	}
	heads := strings.Fields(strings.TrimSpace(string(res.Out)))
	if len(heads) != 2 || heads[0] != sha || heads[1] != ref {
		return ErrBundle
	}
	if _, err := g.run(ctx, append(g.prefix(), "-C", g.Repo, "bundle", "verify", "--quiet", file)...); err != nil {
		return fmt.Errorf("%w: %v", ErrBundle, err)
	}
	if _, err := g.run(ctx, append(g.prefix(), "-C", g.Repo, "bundle", "unbundle", file)...); err != nil {
		return fmt.Errorf("%w: %v", ErrBundle, err)
	}
	if baseSHA != "" {
		if _, err := g.run(ctx, append(g.prefix(), "-C", g.Repo, "merge-base", "--is-ancestor", baseSHA, sha)...); err != nil {
			return ErrBundle
		}
	}
	_, err = g.run(ctx, append(g.prefix(), "-C", g.Repo, "update-ref", ref, sha)...)
	return err
}
