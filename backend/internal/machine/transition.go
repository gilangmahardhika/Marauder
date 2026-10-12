package machine

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"orchestrator/backend/internal/board"
	"orchestrator/backend/internal/trace"
)

var (
	ErrIllegal  = errors.New("illegal transition")
	ErrEvidence = errors.New("evidence required")
)

type Evidence struct {
	ExitCode      *int
	PRURL         string
	SHA           string
	Approver      string
	LogRef        string
	Reason        string
	ResetAttempts *bool
	AttemptID     string
	ActorID       string
}

type transitionPayload struct {
	From     string `json:"from"`
	To       string `json:"to"`
	Evidence string `json:"evidence"`
	ActorID  string `json:"actorId,omitempty"`
}

// Transition is the only card status write.
// Commit tx when the error is nil, ErrIllegal, or ErrEvidence.
// Roll tx back on ErrConflict and on any other error. ErrConflict writes no event and no gate.
func Transition(ctx context.Context, tx *sql.Tx, card board.Card, to string, ev Evidence, secrets []string) (board.Card, error) {
	if _, ok := edges[[2]string{card.Status, to}]; !ok {
		if err := reject(ctx, tx, card, to, "illegal", secrets); err != nil {
			return card, err
		}
		return card, ErrIllegal
	}
	if missing := missingEvidence(card.Status, to, ev); missing != "" {
		if err := reject(ctx, tx, card, to, missing, secrets); err != nil {
			return card, err
		}
		return card, ErrEvidence
	}

	now := time.Now().UTC().Format(time.RFC3339)
	write := board.StatusWrite{
		ID:        card.ID,
		From:      card.Status,
		To:        to,
		UpdatedAt: now,
	}
	if to == "approved" {
		write.SetApprovedSHA = true
		write.ApprovedSHA = sql.NullString{String: ev.SHA, Valid: true}
	}
	if to == "blocked" {
		write.SetBlockedReason = true
		write.BlockedReason = sql.NullString{String: ev.Reason, Valid: true}
	} else if card.Status == "blocked" {
		write.SetBlockedReason = true
		write.BlockedReason = sql.NullString{}
	}
	if to == "pr_open" && ev.PRURL != "" {
		write.SetPRURL = true
		write.PRURL = sql.NullString{String: ev.PRURL, Valid: true}
	}
	if clearsApproval(card.Status, to) {
		write.SetApprovedSHA = true
		write.ApprovedSHA = sql.NullString{}
	}

	rows, err := board.CompareAndSwap(ctx, tx, write)
	if err != nil {
		return card, err
	}
	if rows != 1 {
		return card, board.ErrConflict
	}

	if name := gateName(card.Status, to); name != "" {
		if err := board.InsertGate(ctx, tx, board.Gate{
			CardID:      card.ID,
			AttemptID:   ev.AttemptID,
			Name:        name,
			Passed:      true,
			EvidenceRef: evidenceRef(ev),
			CreatedAt:   now,
		}); err != nil {
			return card, err
		}
	}
	if clearsApproval(card.Status, to) {
		if err := board.ClearApprovals(ctx, tx, card.ID, now); err != nil {
			return card, err
		}
	}
	if _, err := trace.Append(ctx, tx, trace.Event{
		CardID:  card.ID,
		ActorID: ev.ActorID,
		Kind:    "transition",
		Payload: transitionPayload{
			From:     card.Status,
			To:       to,
			Evidence: evidenceText(ev),
			ActorID:  ev.ActorID,
		},
	}, secrets); err != nil {
		return card, err
	}
	return board.GetCard(ctx, tx, card.ID)
}

func reject(ctx context.Context, tx *sql.Tx, card board.Card, to, evidence string, secrets []string) error {
	_, err := trace.Append(ctx, tx, trace.Event{
		CardID: card.ID,
		Kind:   "transition_rejected",
		Payload: transitionPayload{
			From:     card.Status,
			To:       to,
			Evidence: evidence,
		},
	}, secrets)
	return err
}

func missingEvidence(from, to string, ev Evidence) string {
	if to == "blocked" && ev.Reason == "" {
		return "reason"
	}
	switch from + "->" + to {
	case "human_review->approved":
		if ev.Approver == "" {
			return "approver"
		}
		if shaProblem(ev.SHA) {
			return "sha"
		}
	case "approved->deploying_dev", "deploying_dev->verifying", "verifying->done":
		if shaProblem(ev.SHA) {
			return "sha"
		}
	case "self_check->pr_open":
		if ev.ExitCode == nil || *ev.ExitCode != 0 {
			return "exitCode"
		}
		if ev.PRURL == "" {
			return "prUrl"
		}
	case "agent_review->pr_open":
		if ev.AttemptID == "" {
			return "attemptId"
		}
		if ev.ActorID == "" {
			return "actorId"
		}
	case "self_check->in_progress":
		if ev.ExitCode == nil || *ev.ExitCode == 0 {
			return "exitCode"
		}
	case "verifying->changes_requested":
		if ev.ExitCode == nil || *ev.ExitCode == 0 {
			return "exitCode"
		}
		if ev.LogRef == "" {
			return "logRef"
		}
	}
	return ""
}

func shaProblem(sha string) bool {
	if len(sha) != 40 && len(sha) != 64 {
		return true
	}
	for _, c := range sha {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return true
		}
	}
	return false
}

func gateName(from, to string) string {
	switch to {
	case "pr_open":
		if from == "self_check" {
			return "local_checks"
		}
	case "human_review":
		if from == "agent_review" {
			return "agent_review"
		}
	case "approved":
		return "human_approval"
	case "verifying":
		return "dev_health"
	case "done":
		return "dev_tests"
	}
	return ""
}

func clearsApproval(from, to string) bool {
	if to == "in_progress" || to == "changes_requested" {
		return true
	}
	if from == "approved" && (to == "human_review" || to == "blocked") {
		return true
	}
	return from == "blocked" && to == "human_review"
}

func evidenceRef(ev Evidence) string {
	for _, value := range []string{ev.SHA, ev.PRURL, ev.LogRef, ev.Reason} {
		if value != "" {
			return value
		}
	}
	return "recorded"
}

func evidenceText(ev Evidence) string {
	parts := make([]string, 0, 4)
	if ev.ExitCode != nil {
		parts = append(parts, fmt.Sprintf("exit %d", *ev.ExitCode))
	}
	if ev.SHA != "" {
		parts = append(parts, "sha "+ev.SHA)
	}
	if ev.Approver != "" {
		parts = append(parts, "approver "+ev.Approver)
	}
	if ev.PRURL != "" {
		parts = append(parts, "pr "+ev.PRURL)
	}
	if ev.LogRef != "" {
		parts = append(parts, "log "+ev.LogRef)
	}
	if ev.Reason != "" {
		parts = append(parts, ev.Reason)
	}
	if len(parts) == 0 {
		return "recorded"
	}
	return strings.Join(parts, ", ")
}
