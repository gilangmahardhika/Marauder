package vcs

import (
	"fmt"
	"regexp"
	"strings"
)

var cardIDPattern = regexp.MustCompile(`^card_[0-9a-f]{12}$`)

type Command struct {
	Argv []string
	Env  []string
}

// Push authenticates with token. Callers pass the card owner's verified token (RFC-054).
func Push(git, askpass, url, token, cardID string) (Command, error) {
	if !cardIDPattern.MatchString(cardID) {
		return Command{}, fmt.Errorf("card id %s", cardID)
	}
	ref := "refs/heads/orch/" + cardID + ":refs/heads/orch/" + cardID
	argv := []string{git, "push", url, ref}
	joined := strings.Join(argv, " ")
	if token != "" && strings.Contains(joined, token) {
		return Command{}, fmt.Errorf("token in argv")
	}
	return Command{
		Argv: argv,
		Env: []string{
			"GIT_ASKPASS=" + askpass,
			"ORCH_ASKPASS=1",
			"GH_TOKEN=" + token,
		},
	}, nil
}

func RewriteBody(previous, summary, metadata string) string {
	const start = "<!-- orchestrator:metadata:start -->"
	const end = "<!-- orchestrator:metadata:end -->"
	block := start + "\n" + metadata + "\n" + end
	if previous == "" {
		if summary == "" {
			summary = "<summary written by the implementer>"
		}
		return block + "\n\n" + summary + "\n"
	}
	i := strings.Index(previous, start)
	j := strings.Index(previous, end)
	if i >= 0 && j > i {
		return block + previous[j+len(end):]
	}
	return block + "\n\n" + previous
}

const (
	BodyStart = "<!-- orchestrator:body:start -->"
	BodyEnd   = "<!-- orchestrator:body:end -->"
)

// MergeBody puts the harness text between BodyStart and BodyEnd and keeps the
// text a person wrote outside them (RFC-066). A body from before RFC-066,
// which has only the metadata markers, is replaced whole. A body without any
// marker follows the harness text.
func MergeBody(previous, harness string) string {
	region := BodyStart + "\n" + strings.TrimSpace(harness) + "\n" + BodyEnd
	i := strings.Index(previous, BodyStart)
	j := strings.Index(previous, BodyEnd)
	switch {
	case i >= 0 && j > i:
		return previous[:i] + region + previous[j+len(BodyEnd):]
	case strings.TrimSpace(previous) == "" || strings.Contains(previous, "<!-- orchestrator:metadata:start -->"):
		return region + "\n"
	default:
		return region + "\n\n" + previous
	}
}

type PullRequest struct {
	URL  string
	Body string
}

func EnsurePR(existingURL, existingBody, summary, metadata string, creates *int) PullRequest {
	body := RewriteBody(existingBody, summary, metadata)
	if existingURL != "" {
		return PullRequest{URL: existingURL, Body: body}
	}
	*creates++
	return PullRequest{URL: "https://github.com/acme/widget/pull/1", Body: body}
}
