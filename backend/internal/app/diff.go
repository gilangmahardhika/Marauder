package app

import (
	"context"
	"database/sql"
	"strings"

	"orchestrator/backend/internal/board"
	"orchestrator/backend/internal/company"
	"orchestrator/backend/internal/runtime"
	"orchestrator/backend/internal/settings"
	"orchestrator/backend/internal/tenancy"
	"orchestrator/backend/internal/trace"
)

// CardDiffCap is the most unified diff text CardDiff reads, and
// FileLineCap the most lines it returns for one file (RFC-068).
const (
	CardDiffCap = 256 << 10
	FileLineCap = 1500
)

type DiffLine struct {
	Kind string `json:"kind"`
	Text string `json:"text"`
}

type DiffHunk struct {
	Header string     `json:"header"`
	Lines  []DiffLine `json:"lines"`
}

type DiffFile struct {
	Path      string     `json:"path"`
	Added     int        `json:"added"`
	Deleted   int        `json:"deleted"`
	Binary    bool       `json:"binary"`
	Truncated bool       `json:"truncated"`
	Hunks     []DiffHunk `json:"hunks"`
}

// CardDiff is the change a person approves: the card's head commit against
// the merge base with the project's base branch.
type CardDiff struct {
	SHA        string     `json:"sha"`
	Base       string     `json:"base"`
	CompareURL string     `json:"compareUrl"`
	Missing    bool       `json:"missing"`
	Truncated  bool       `json:"truncated"`
	Files      []DiffFile `json:"files"`
}

// ReadCardDiff reads the diff of the card's head SHA from the project clone.
// It fetches the card branch with the company token when the head is not in
// the clone, and falls back to the latest checked commit. Paths and text are
// redacted with the model keys and the company token.
func ReadCardDiff(ctx context.Context, db *sql.DB, deps JobDeps, projectID, cardID string) (CardDiff, error) {
	out := CardDiff{Files: []DiffFile{}}
	card, err := board.GetCard(ctx, db, cardID)
	if err != nil {
		return out, err
	}
	if card.ProjectID != projectID {
		return out, board.ErrNotFound
	}
	project, err := tenancy.GetProject(ctx, db, projectID)
	if err != nil {
		return out, err
	}
	attempt, err := board.LatestReview(ctx, db, card.ID)
	if err != nil {
		return out, err
	}
	sha := card.HeadSHA
	if sha == "" {
		sha = attempt.CommitSHA
	}
	out.SHA = sha
	out.Base = project.BaseBranch
	if out.Base == "" {
		out.Base = "main"
	}
	if sha == "" {
		return out, nil
	}
	acct, err := company.Load(ctx, db)
	if err != nil {
		return out, err
	}
	if acct.Host != "" && acct.Org != "" && project.Repo != "" {
		out.CompareURL = "https://" + acct.Host + "/" + acct.Org + "/" + project.Repo + "/compare/" + out.Base + "..." + sha
	}
	git := ProjectGit(deps.Data, project, runtime.Author{}, deps.Tools)
	if deps.Data == "" || deps.Tools.Git == "" || project.RepoPath == "" || !git.Cloned() {
		out.Missing = true
		return out, nil
	}
	var redact []string
	if _, secrets, err := settings.Load(deps.Data); err == nil {
		redact = modelKeys(secrets)
	}
	token, _ := company.Token(ctx, db, deps.Key)
	if token != "" {
		redact = append(redact, token)
	}

	if _, err := git.RevParse(ctx, sha); err != nil && token != "" && project.Repo != "" {
		_ = git.FetchCard(ctx, card.ID, runtime.Remote{URL: acct.CloneURL(project.Repo), Token: token})
	}
	if _, err := git.RevParse(ctx, sha); err != nil {
		if attempt.CommitSHA == "" || attempt.CommitSHA == sha {
			out.Missing = true
			return out, nil
		}
		if _, err := git.RevParse(ctx, attempt.CommitSHA); err != nil {
			out.Missing = true
			return out, nil
		}
		sha = attempt.CommitSHA
		out.SHA = sha
	}

	base := "refs/heads/" + out.Base
	stats, err := git.NumStat(ctx, base, sha)
	if err != nil {
		return out, redactedErr(err, redact)
	}
	text, truncated, err := git.Diff(ctx, base, sha, CardDiffCap)
	if err != nil {
		return out, redactedErr(err, redact)
	}
	out.Truncated = truncated
	out.Files = mergeStats(ParseDiff(trace.Redact(text, redact)), stats, redact, truncated)
	return out, nil
}

func redactedErr(err error, redact []string) error {
	return &redactedError{msg: trace.Redact(err.Error(), redact), err: err}
}

type redactedError struct {
	msg string
	err error
}

func (e *redactedError) Error() string { return e.msg }
func (e *redactedError) Unwrap() error { return e.err }

// mergeStats sets the line counts from NumStat on the parsed files, and adds
// the files the cut diff text did not reach as truncated.
func mergeStats(files []DiffFile, stats []runtime.FileChange, redact []string, truncated bool) []DiffFile {
	index := map[string]int{}
	for i, f := range files {
		index[f.Path] = i
	}
	if truncated && len(files) > 0 {
		files[len(files)-1].Truncated = true
	}
	for _, s := range stats {
		path := trace.Redact(s.Path, redact)
		i, ok := index[path]
		if !ok {
			files = append(files, DiffFile{Path: path, Truncated: truncated, Hunks: []DiffHunk{}})
			i = len(files) - 1
			index[path] = i
		}
		if s.Added < 0 {
			files[i].Binary = true
			continue
		}
		files[i].Added, files[i].Deleted = s.Added, s.Deleted
	}
	return files
}

// ParseDiff splits unified diff text into files, hunks, and lines. A file
// keeps at most FileLineCap lines.
func ParseDiff(text string) []DiffFile {
	files := []DiffFile{}
	var file *DiffFile
	var hunk *DiffHunk
	lines := 0
	flush := func() {
		if file == nil {
			return
		}
		if hunk != nil {
			file.Hunks = append(file.Hunks, *hunk)
			hunk = nil
		}
		files = append(files, *file)
		file = nil
	}
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, "diff --git ") {
			flush()
			file = &DiffFile{Path: diffPath(line), Hunks: []DiffHunk{}}
			lines = 0
			continue
		}
		if file == nil {
			continue
		}
		switch {
		case strings.HasPrefix(line, "@@"):
			if hunk != nil {
				file.Hunks = append(file.Hunks, *hunk)
			}
			hunk = &DiffHunk{Header: line, Lines: []DiffLine{}}
		case hunk == nil:
			if strings.HasPrefix(line, "Binary files ") {
				file.Binary = true
			}
			if strings.HasPrefix(line, "+++ b/") {
				file.Path = strings.TrimPrefix(line, "+++ b/")
			}
		case lines >= FileLineCap:
			file.Truncated = true
		default:
			kind := "ctx"
			switch {
			case strings.HasPrefix(line, "+"):
				kind = "add"
			case strings.HasPrefix(line, "-"):
				kind = "del"
			case strings.HasPrefix(line, `\`):
				kind = "meta"
			case line == "":
				continue
			}
			text := line
			if kind != "meta" {
				text = line[1:]
			}
			hunk.Lines = append(hunk.Lines, DiffLine{Kind: kind, Text: text})
			lines++
		}
	}
	flush()
	return files
}

// diffPath reads the path from "diff --git a/P b/P", which --no-renames keeps
// equal on both sides.
func diffPath(header string) string {
	rest := strings.TrimPrefix(header, "diff --git ")
	if len(rest) >= 5 && strings.HasPrefix(rest, "a/") && (len(rest)-5)%2 == 0 {
		n := (len(rest) - 5) / 2
		if rest[2+n:2+n+3] == " b/" {
			return rest[2 : 2+n]
		}
	}
	return rest
}
