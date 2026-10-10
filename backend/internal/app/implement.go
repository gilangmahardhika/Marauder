package app

import (
	"context"

	"orchestrator/backend/internal/board"
	"orchestrator/backend/internal/loop"
)

func RunImplementer(ctx context.Context, in loop.Input) (loop.Result, error) {
	return loop.Run(ctx, in)
}

func CreateCard(ctx context.Context, in loop.CreateInput) (board.Card, error) {
	return loop.CreateCard(ctx, in)
}
