package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"orchestrator/backend/internal/board"
	"orchestrator/backend/internal/intake"
	"orchestrator/backend/internal/runtime"
	"orchestrator/backend/internal/trace"
)

const (
	prTitleCap = 100
	prFileCap  = 50
)

var commitTrailer = regexp.MustCompile(`^(Orchestrator-[A-Za-z]+|Model):`)

// PRText is a pull request title and the harness part of its body (RFC-066).
type PRText struct {
	Title string
	Body  string
}

// PRRequirement is one requirement a pull request implements.
type PRRequirement struct {
	ID         string
	Statement  string
	Acceptance []string
}

// PRInput is everything the pull request text is built from.
type PRInput struct {
	CardID        string
	CardTitle     string
	Requirements  []PRRequirement
	CommitMessage string
	CommitSHA     string
	Files         []runtime.FileChange
	LocalCheck    string
	AttemptID     string
	Model         string
}

// BuildPRText reads the card's requirements, the commit message, and the
// changed files, and builds the pull request text. A git read that fails
// leaves its section out rather than stopping the publish.
func BuildPRText(ctx context.Context, db *sql.DB, git runtime.Git, card board.Card, baseBranch, sha, attemptID, model string, secrets []string) (PRText, error) {
	reqs, err := prRequirements(ctx, db, card)
	if err != nil {
		return PRText{}, err
	}
	if baseBranch == "" {
		baseBranch = "main"
	}
	message, _ := git.CommitMessage(ctx, sha)
	files, _ := git.NumStat(ctx, "refs/heads/"+baseBranch, sha)
	in := PRInput{
		CardID: card.ID, CardTitle: card.Title, Requirements: reqs,
		CommitMessage: message, CommitSHA: sha, Files: files,
		LocalCheck: card.LocalCheck, AttemptID: attemptID, Model: model,
	}
	return PRText{
		Title: trace.Redact(PRTitle(card.RequirementIDs, card.Title, message), secrets),
		Body:  trace.Redact(PRBody(in), secrets),
	}, nil
}

func prRequirements(ctx context.Context, db *sql.DB, card board.Card) ([]PRRequirement, error) {
	var out []PRRequirement
	for _, id := range card.RequirementIDs {
		var statement, acceptance string
		err := db.QueryRowContext(ctx, `
			SELECT statement, acceptance_json FROM requirements WHERE project_id = $1 AND id = $2`,
			card.ProjectID, id).Scan(&statement, &acceptance)
		if errors.Is(err, sql.ErrNoRows) {
			out = append(out, PRRequirement{ID: id})
			continue
		}
		if err != nil {
			return nil, err
		}
		req := PRRequirement{ID: id, Statement: statement}
		_ = json.Unmarshal([]byte(acceptance), &req.Acceptance)
		out = append(out, req)
	}
	return out, nil
}

// PRTitle is the requirement ids, then the card title, or the commit subject
// when the card title names only the requirement ids.
func PRTitle(ids []string, cardTitle, commitMessage string) string {
	prefix := strings.Join(ids, ", ")
	title := strings.TrimSpace(cardTitle)
	if onlyIDs(title, ids) {
		if subject, _ := splitCommit(commitMessage); subject != "" {
			title = subject
		}
	}
	if prefix != "" && !onlyIDs(title, ids) && !strings.HasPrefix(title, prefix) {
		title = prefix + ": " + title
	}
	if title == "" {
		title = prefix
	}
	return capRunes(oneLine(title), prTitleCap)
}

// PRBody is the harness part of the pull request body. The caller places it
// between the body markers with vcs.MergeBody.
func PRBody(in PRInput) string {
	var b strings.Builder
	subject, body := splitCommit(in.CommitMessage)
	if subject == "" {
		subject = in.CardTitle
	}
	b.WriteString("## Summary\n\n")
	b.WriteString(untrustedText(subject))
	if body != "" {
		b.WriteString("\n\n")
		b.WriteString(untrustedText(body))
	}
	b.WriteString("\n\n")

	if len(in.Requirements) > 0 {
		heading := "## Requirement"
		if len(in.Requirements) > 1 {
			heading += "s"
		}
		b.WriteString(heading)
		b.WriteString("\n\n")
		for _, req := range in.Requirements {
			statement := strings.TrimSpace(req.Statement)
			if statement == "" {
				statement = "(not found in this project)"
			}
			fmt.Fprintf(&b, "**%s**\n\n%s\n\n", req.ID, untrustedText(escapeMentions(closeFences(statement))))
			if len(req.Acceptance) > 0 {
				b.WriteString("Acceptance criteria:\n")
				for _, line := range req.Acceptance {
					fmt.Fprintf(&b, "- [ ] %s\n", untrustedText(escapeMentions(oneLine(line))))
				}
				b.WriteString("\n")
			}
		}
	}

	if len(in.Files) > 0 {
		b.WriteString("## Changes\n\n")
		for i, f := range in.Files {
			if i == prFileCap {
				fmt.Fprintf(&b, "- and %d more files\n", len(in.Files)-prFileCap)
				break
			}
			if f.Added < 0 {
				fmt.Fprintf(&b, "- `%s` (binary)\n", codeText(f.Path))
				continue
			}
			fmt.Fprintf(&b, "- `%s` (+%d -%d)\n", codeText(f.Path), f.Added, f.Deleted)
		}
		b.WriteString("\n")
	}

	b.WriteString("## Verification\n\n")
	check := in.LocalCheck
	if check == "" {
		check = "(none)"
	}
	fmt.Fprintf(&b, "Local check `%s` exited 0 on `%s` in the sandbox.\n\n", codeText(check), shortSHA(in.CommitSHA))

	b.WriteString("## Review\n\n")
	b.WriteString("The AI review is posted as a comment. A person approves the card in the orchestrator before merge.\n\n")

	b.WriteString(prMetadataStart + "\n\n")
	b.WriteString("<!-- orchestrator:metadata:start -->\n```text\n")
	fmt.Fprintf(&b, "requirements: %s\nlocalCheck: %s\ncard: %s\nattempt: %s\nmodel: %s\ncommit: %s\n",
		strings.Join(requirementIDs(in.Requirements), ", "), oneLine(in.LocalCheck), in.CardID, in.AttemptID, in.Model, in.CommitSHA)
	b.WriteString("```\n<!-- orchestrator:metadata:end -->\n\n</details>")
	return b.String()
}

const prMetadataStart = "<details><summary>Orchestrator metadata</summary>"

// PRBodyForDisplay is a stored PR body as the card page shows it: without the
// metadata block, whose HTML the page's markdown renders as text, and with the
// untrustedText escape undone (RFC-110).
func PRBodyForDisplay(body string) string {
	if i := strings.LastIndex(body, prMetadataStart); i >= 0 {
		body = body[:i]
	}
	body = strings.ReplaceAll(body, "&lt;!-- orchestrator:", "<!-- orchestrator:")
	return strings.TrimSpace(body)
}

// splitCommit returns the subject and body of a commit message without the
// harness trailers.
func splitCommit(message string) (subject, body string) {
	var kept []string
	for _, line := range strings.Split(strings.ReplaceAll(message, "\r\n", "\n"), "\n") {
		if commitTrailer.MatchString(line) {
			continue
		}
		kept = append(kept, line)
	}
	text := strings.TrimSpace(strings.Join(kept, "\n"))
	if text == "" {
		return "", ""
	}
	subject, body, _ = strings.Cut(text, "\n")
	return strings.TrimSpace(subject), strings.TrimSpace(body)
}

func onlyIDs(title string, ids []string) bool {
	for _, id := range ids {
		title = strings.ReplaceAll(title, id, "")
	}
	return strings.Trim(title, " ,;:-") == ""
}

func requirementIDs(reqs []PRRequirement) []string {
	ids := make([]string, 0, len(reqs))
	for _, req := range reqs {
		ids = append(ids, req.ID)
	}
	return ids
}

// untrustedText keeps model and requirement text from closing the harness
// region of the body.
func untrustedText(s string) string {
	return strings.ReplaceAll(s, "<!-- orchestrator:", "&lt;!-- orchestrator:")
}

var fenceLine = regexp.MustCompile("^ {0,3}(`{3,}|~{3,})(.*)$")

// closeFences ends a code fence that a markdown statement left open, so the
// statement cannot turn the rest of the pull request body into code.
func closeFences(s string) string {
	open := ""
	for _, line := range strings.Split(s, "\n") {
		m := fenceLine.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		mark, rest := m[1], m[2]
		switch {
		case open == "":
			if mark[0] == '`' && strings.Contains(rest, "`") {
				continue
			}
			open = mark
		case mark[0] == open[0] && len(mark) >= len(open) && strings.TrimSpace(rest) == "":
			open = ""
		}
	}
	if open == "" {
		return s
	}
	return s + "\n" + open
}

// escapeMentions wraps each @slug mention outside code in backticks, so the
// pull request never notifies a GitHub user or team (RFC-101).
func escapeMentions(s string) string {
	lines := strings.Split(s, "\n")
	open := ""
	for i, line := range lines {
		if m := fenceLine.FindStringSubmatch(line); m != nil {
			mark, rest := m[1], m[2]
			switch {
			case open == "":
				if mark[0] != '`' || !strings.Contains(rest, "`") {
					open = mark
				}
			case mark[0] == open[0] && len(mark) >= len(open) && strings.TrimSpace(rest) == "":
				open = ""
			}
			continue
		}
		if open == "" {
			lines[i] = escapeLineMentions(line)
		}
	}
	return strings.Join(lines, "\n")
}

// escapeLineMentions wraps mentions in the parts of one line that are not
// inline code.
func escapeLineMentions(line string) string {
	var b strings.Builder
	plain := 0
	flush := func(end int) {
		text := line[plain:end]
		last := 0
		for _, span := range intake.FindMentions(text) {
			b.WriteString(text[last:span[0]])
			b.WriteString("`")
			b.WriteString(text[span[0]:span[1]])
			b.WriteString("`")
			last = span[1]
		}
		b.WriteString(text[last:])
	}
	for i := 0; i < len(line); {
		if line[i] != '`' {
			i++
			continue
		}
		run := i
		for i < len(line) && line[i] == '`' {
			i++
		}
		ticks := line[run:i]
		end := closingTicks(line, i, ticks)
		if end < 0 {
			continue
		}
		flush(run)
		b.WriteString(line[run:end])
		plain, i = end, end
	}
	flush(len(line))
	return b.String()
}

// closingTicks is the end of the backtick run that closes a code span opened
// by ticks, or -1.
func closingTicks(line string, from int, ticks string) int {
	for i := from; i < len(line); {
		if line[i] != '`' {
			i++
			continue
		}
		run := i
		for i < len(line) && line[i] == '`' {
			i++
		}
		if i-run == len(ticks) {
			return i
		}
	}
	return -1
}

func codeText(s string) string {
	return strings.ReplaceAll(oneLine(s), "`", "'")
}

func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func shortSHA(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}

func capRunes(s string, max int) string {
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	r := []rune(s)
	return string(r[:max-3]) + "..."
}
