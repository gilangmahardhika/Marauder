package app

import (
	"context"
	"database/sql"

	"orchestrator/backend/internal/board"
	"orchestrator/backend/internal/intake"
)

type IngestResult struct {
	Requirements int
	Decisions    int
	DraftPending bool
}

func Ingest(ctx context.Context, db *sql.DB, repo, data string) (IngestResult, error) {
	cfg, err := intake.LoadConfig(data)
	if err != nil {
		return IngestResult{}, err
	}
	prd, err := intake.RepoPath(repo, cfg.PRD)
	if err != nil {
		return IngestResult{}, err
	}
	rfc, err := intake.RepoPath(repo, cfg.RFC)
	if err != nil {
		return IngestResult{}, err
	}
	parsed, err := intake.Parse(repo, prd, rfc)
	if err != nil {
		return IngestResult{}, err
	}
	if err := intake.Apply(ctx, db, parsed); err != nil {
		return IngestResult{}, err
	}
	return IngestResult{
		Requirements: len(parsed.Requirements),
		Decisions:    len(parsed.Decisions),
		DraftPending: parsed.DraftPending,
	}, nil
}

func Accept(ctx context.Context, db *sql.DB, data string, ids []string) error {
	cfg, err := intake.LoadConfig(data)
	if err != nil {
		return err
	}
	return intake.Accept(ctx, db, cfg.Operator, ids)
}

func CardCount(ctx context.Context, db *sql.DB) (int, error) {
	return board.CountCards(ctx, db)
}
