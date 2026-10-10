package worker

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"syscall"

	"orchestrator/backend/internal/board"
	"orchestrator/backend/internal/trace"
)

var ErrBusy = errors.New("another worker is running")

func Lock(path string) (*os.File, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		return nil, ErrBusy
	}
	return f, nil
}

type Proposal struct {
	Title   string
	Reqs    []string
	Depends []string
	Risk    string
}

func Plan(proposals []Proposal, accepted map[string]bool, local, dev string) ([]board.Card, error) {
	index := map[string]int{}
	for i, proposal := range proposals {
		index[proposal.Title] = i
		for _, id := range proposal.Reqs {
			if !accepted[id] {
				return nil, errors.New("unaccepted requirement")
			}
		}
	}
	seen := map[string]bool{}
	var visit func(string, map[string]bool) error
	visit = func(title string, stack map[string]bool) error {
		if stack[title] {
			return errors.New("dependency cycle")
		}
		if seen[title] {
			return nil
		}
		stack[title] = true
		for _, dep := range proposals[index[title]].Depends {
			if err := visit(dep, stack); err != nil {
				return err
			}
		}
		delete(stack, title)
		seen[title] = true
		return nil
	}
	for _, proposal := range proposals {
		if err := visit(proposal.Title, map[string]bool{}); err != nil {
			return nil, err
		}
	}
	var cards []board.Card
	for _, proposal := range proposals {
		cards = append(cards, board.Card{
			Title: proposal.Title, Status: "backlog", Risk: proposal.Risk,
			RequirementIDs: proposal.Reqs, DependsOn: proposal.Depends,
			LocalCheck: local, DevCheck: dev,
		})
	}
	return cards, nil
}

func CanStart(done map[string]bool, deps []string) bool {
	for _, dep := range deps {
		if !done[dep] {
			return false
		}
	}
	return true
}

func Release(ctx context.Context, db *sql.DB, cardID, holder string) error {
	res, err := db.ExecContext(ctx, `DELETE FROM locks WHERE card_id = $1 AND holder = $2`, cardID, holder)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		_, err = trace.Append(ctx, db, trace.Event{CardID: cardID, Kind: "assertion_failed", Payload: map[string]any{
			"name": "lock_holder", "detail": "wrong holder",
		}}, nil)
	}
	return err
}

func BaseMoved(before, after string) bool {
	return before != "" && after != "" && before != after
}
