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
	Unnumbered   []string
	Skipped      []string
}

// Ingest parses the PRD and RFC paths from configDir/config.yaml inside repo
// and stores the records on projectID. actorID is the session user, or empty
// for the CLI.
func Ingest(ctx context.Context, db *sql.DB, projectID, actorID, repo, configDir string) (IngestResult, error) {
	cfg, err := intake.LoadConfig(configDir)
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
	skipped, err := intake.Apply(ctx, db, projectID, actorID, parsed)
	if err != nil {
		return IngestResult{}, err
	}
	return IngestResult{
		Requirements: len(parsed.Requirements),
		Decisions:    len(parsed.Decisions),
		DraftPending: parsed.DraftPending,
		Unnumbered:   parsed.Unnumbered,
		Skipped:      skipped,
	}, nil
}

// Accept is the CLI twin. The operator comes from operatorLogin in the service config.yaml.
func Accept(ctx context.Context, db *sql.DB, projectID, data string, ids []string) error {
	cfg, err := intake.LoadConfig(data)
	if err != nil {
		return err
	}
	return intake.Accept(ctx, db, projectID, cfg.Operator, ids)
}

func CardCount(ctx context.Context, db *sql.DB) (int, error) {
	return board.CountCards(ctx, db)
}
