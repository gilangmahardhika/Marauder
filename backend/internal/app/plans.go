package app

import (
	"context"
	"database/sql"
	"errors"
	"slices"
	"strings"

	"orchestrator/backend/internal/board"
	"orchestrator/backend/internal/identity"
	"orchestrator/backend/internal/loop"
	"orchestrator/backend/internal/machine"
	"orchestrator/backend/internal/settings"
	"orchestrator/backend/internal/trace"
)

var planModes = []string{"required", "off"}

// SetPlanMode stores whether the card must plan before coding (RFC-128).
func SetPlanMode(ctx context.Context, db *sql.DB, actor User, projectID, cardID, mode string) (board.Card, error) {
	if !slices.Contains(planModes, mode) {
		return board.Card{}, &settings.Invalid{Fields: map[string]string{"planMode": "Choose required or off."}}
	}
	ok, err := CanAnswer(ctx, db, actor, projectID)
	if err != nil {
		return board.Card{}, err
	}
	if !ok {
		return board.Card{}, identity.ErrForbidden
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return board.Card{}, err
	}
	defer tx.Rollback()
	card, err := lockCard(ctx, tx, projectID, cardID)
	if err != nil {
		return card, err
	}
	if card.Status == "done" || card.Status == "wont_fix" {
		return card, board.ErrConflict
	}
	old, err := board.SetPlanMode(ctx, tx, projectID, cardID, mode)
	if err != nil {
		return card, err
	}
	if old != mode {
		if _, err := trace.Append(ctx, tx, trace.Event{
			CardID: card.ID, ProjectID: card.ProjectID, ActorID: actor.ID, Kind: "plan_mode_changed",
			Payload: map[string]any{"from": old, "to": mode},
		}, nil); err != nil {
			return card, err
		}
	}
	if card, err = board.GetCard(ctx, tx, cardID); err != nil {
		return card, err
	}
	return card, tx.Commit()
}

// SavePlan updates the open plan without changing status (RFC-129).
func SavePlan(ctx context.Context, db *sql.DB, actor User, projectID, cardID, title string, steps []board.PlanStep) (board.Card, error) {
	ok, err := CanAnswer(ctx, db, actor, projectID)
	if err != nil {
		return board.Card{}, err
	}
	if !ok {
		return board.Card{}, identity.ErrForbidden
	}
	title, steps, err = loop.NormalizePlan(title, steps)
	if err != nil {
		return board.Card{}, planFieldErr(err)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return board.Card{}, err
	}
	defer tx.Rollback()
	card, err := lockCard(ctx, tx, projectID, cardID)
	if err != nil {
		return card, err
	}
	if card.Status != "waiting_answer" {
		return card, board.ErrConflict
	}
	if _, err := board.UpdateOpenPlan(ctx, tx, projectID, cardID, title, steps); err != nil {
		return card, err
	}
	if card, err = board.GetCard(ctx, tx, cardID); err != nil {
		return card, err
	}
	return card, tx.Commit()
}

// AcceptPlan accepts the open plan (optionally with edited steps), moves to
// ready, and queues a run (RFC-129).
func AcceptPlan(ctx context.Context, db *sql.DB, actor User, projectID, cardID, title string, steps []board.PlanStep, useStored bool) (board.Card, error) {
	ok, err := CanAnswer(ctx, db, actor, projectID)
	if err != nil {
		return board.Card{}, err
	}
	if !ok {
		return board.Card{}, identity.ErrForbidden
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return board.Card{}, err
	}
	defer tx.Rollback()
	card, err := lockCard(ctx, tx, projectID, cardID)
	if err != nil {
		return card, err
	}
	if card.Status != "waiting_answer" {
		return card, board.ErrConflict
	}
	open, err := board.OpenPlan(ctx, tx, projectID, cardID)
	if err != nil {
		if errors.Is(err, board.ErrNotFound) {
			return card, board.ErrConflict
		}
		return card, err
	}
	if useStored || (title == "" && len(steps) == 0) {
		title, steps = open.Title, open.Steps
	}
	title, steps, err = loop.NormalizePlan(title, steps)
	if err != nil {
		return card, planFieldErr(err)
	}
	accepted, err := board.AcceptOpenPlan(ctx, tx, projectID, cardID, title, steps, actor.ID)
	if err != nil {
		return card, err
	}
	next, err := machine.Transition(ctx, tx, card, "ready", machine.Evidence{ActorID: actor.ID}, nil)
	if err != nil {
		return card, err
	}
	if _, err := insertCardRun(ctx, tx, projectID, cardID, "api"); err != nil {
		return card, err
	}
	if _, err := trace.Append(ctx, tx, trace.Event{
		CardID: card.ID, ProjectID: card.ProjectID, ActorID: actor.ID, Kind: "plan_accepted",
		Payload: map[string]any{"planId": accepted.ID, "steps": len(accepted.Steps)},
	}, nil); err != nil {
		return card, err
	}
	return next, tx.Commit()
}

// SkipPlan discards an open plan when plan mode is off and no questions are
// open, then queues a run (RFC-128).
func SkipPlan(ctx context.Context, db *sql.DB, actor User, projectID, cardID string) (board.Card, error) {
	ok, err := CanAnswer(ctx, db, actor, projectID)
	if err != nil {
		return board.Card{}, err
	}
	if !ok {
		return board.Card{}, identity.ErrForbidden
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return board.Card{}, err
	}
	defer tx.Rollback()
	card, err := lockCard(ctx, tx, projectID, cardID)
	if err != nil {
		return card, err
	}
	if card.Status != "waiting_answer" || card.PlanMode != "off" {
		return card, board.ErrConflict
	}
	openQ, err := board.OpenQuestions(ctx, tx, projectID, cardID)
	if err != nil {
		return card, err
	}
	if len(openQ) > 0 {
		return card, board.ErrConflict
	}
	if err := board.DiscardOpenPlan(ctx, tx, projectID, cardID); err != nil {
		return card, err
	}
	next, err := machine.Transition(ctx, tx, card, "ready", machine.Evidence{ActorID: actor.ID}, nil)
	if err != nil {
		return card, err
	}
	if _, err := insertCardRun(ctx, tx, projectID, cardID, "api"); err != nil {
		return card, err
	}
	if _, err := trace.Append(ctx, tx, trace.Event{
		CardID: card.ID, ProjectID: card.ProjectID, ActorID: actor.ID, Kind: "plan_skipped",
		Payload: map[string]any{},
	}, nil); err != nil {
		return card, err
	}
	return next, tx.Commit()
}

func planFieldErr(err error) error {
	msg := err.Error()
	fields := map[string]string{}
	switch {
	case strings.Contains(msg, "title"):
		fields["title"] = sentence(msg)
	case strings.Contains(msg, "step"):
		fields["steps"] = sentence(msg)
	default:
		fields["steps"] = sentence(msg)
	}
	return &settings.Invalid{Fields: fields}
}

// PlanningPhase reports whether the next implement attempt must only plan.
func PlanningPhase(ctx context.Context, q board.DBTX, card board.Card) (bool, error) {
	if card.PlanMode != "required" {
		return false, nil
	}
	ok, err := board.HasAcceptedPlan(ctx, q, card.ID)
	if err != nil {
		return false, err
	}
	return !ok, nil
}
