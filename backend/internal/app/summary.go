package app

import (
	"context"
	"database/sql"
	"time"

	"orchestrator/backend/internal/board"
	"orchestrator/backend/internal/settings"
)

// ProjectSummaryReport is GET /api/projects/{id}/summary (RFC-127).
type ProjectSummaryReport struct {
	From      string                `json:"from"`
	To        string                `json:"to"`
	Groups    board.GroupCounts     `json:"groups"`
	Attention board.Attention       `json:"attention"`
	Aging     []board.AgingCard     `json:"aging"`
	Range     board.RangeInsights   `json:"range"`
}

// TribeProjectSummary is one project inside a tribe summary.
type TribeProjectSummary struct {
	ProjectID   string            `json:"projectId"`
	ProjectName string            `json:"projectName"`
	Groups      board.GroupCounts `json:"groups"`
	Attention   board.Attention   `json:"attention"`
}

// TribeSummaryReport is GET /api/tribes/{id}/summary (RFC-127).
type TribeSummaryReport struct {
	From      string                 `json:"from"`
	To        string                 `json:"to"`
	Groups    board.GroupCounts      `json:"groups"`
	Attention board.Attention        `json:"attention"`
	Range     board.RangeInsights    `json:"range"`
	Projects  []TribeProjectSummary  `json:"projects"`
	MCP       board.MCPHealth        `json:"mcp"`
}

// ProjectSummary builds the project dashboard for a member (RFC-127).
func ProjectSummary(ctx context.Context, db *sql.DB, data string, actor User, projectID, from, to string, now time.Time) (ProjectSummaryReport, error) {
	if err := SeeProject(ctx, db, projectID, actor.ID); err != nil {
		return ProjectSummaryReport{}, err
	}
	start, end, err := UsageWindow(from, to, now)
	if err != nil {
		return ProjectSummaryReport{}, err
	}
	snap, rng, err := buildProjectSummary(ctx, db, data, projectID, start, end)
	if err != nil {
		return ProjectSummaryReport{}, err
	}
	return ProjectSummaryReport{
		From: start.Format(time.DateOnly), To: end.Format(time.DateOnly),
		Groups: snap.Groups, Attention: snap.Attention, Aging: snap.Aging, Range: rng,
	}, nil
}

// TribeSummary builds the tribe dashboard for a tribe admin or company admin (RFC-127).
func TribeSummary(ctx context.Context, db *sql.DB, data string, actor User, tribeID, from, to string, now time.Time) (TribeSummaryReport, error) {
	if err := ManageTribe(ctx, db, actor, tribeID); err != nil {
		return TribeSummaryReport{}, err
	}
	start, end, err := UsageWindow(from, to, now)
	if err != nil {
		return TribeSummaryReport{}, err
	}
	fromTS := start.Format(time.RFC3339)
	toTS := end.AddDate(0, 0, 1).Format(time.RFC3339)

	projects, err := board.TribeProjectIDs(ctx, db, tribeID)
	if err != nil {
		return TribeSummaryReport{}, err
	}
	out := TribeSummaryReport{
		From: start.Format(time.DateOnly), To: end.Format(time.DateOnly),
		Groups: board.EmptyGroupCounts(),
		Attention: board.Attention{Blocked: []board.ReasonCount{}},
		Range: board.RangeInsights{BudgetStops: []board.CapCount{}, AnswerSources: board.AnswerSources{}},
		Projects: []TribeProjectSummary{},
	}
	var answerHours []float64
	for _, p := range projects {
		snap, rng, err := buildProjectSummary(ctx, db, data, p.ID, start, end)
		if err != nil {
			return TribeSummaryReport{}, err
		}
		out.Projects = append(out.Projects, TribeProjectSummary{
			ProjectID: p.ID, ProjectName: p.Name, Groups: snap.Groups, Attention: snap.Attention,
		})
		out.Groups = board.AddGroups(out.Groups, snap.Groups)
		out.Attention.ReadyCount += snap.Attention.ReadyCount
		out.Attention.WaitingAnswer += snap.Attention.WaitingAnswer
		out.Attention.HumanReview += snap.Attention.HumanReview
		out.Attention.OpenRuns += snap.Attention.OpenRuns
		out.Attention.Locked += snap.Attention.Locked
		out.Attention.OpenQuestions += snap.Attention.OpenQuestions
		out.Attention.ImplementationCap += snap.Attention.ImplementationCap
		out.Attention.Blocked = board.MergeBlocked(out.Attention.Blocked, snap.Attention.Blocked)
		out.Range.Done += rng.Done
		out.Range.WontFix += rng.WontFix
		out.Range.Rework += rng.Rework
		out.Range.BudgetStops = board.MergeCaps(out.Range.BudgetStops, rng.BudgetStops)
		out.Range.AnswerSources.Person += rng.AnswerSources.Person
		out.Range.AnswerSources.Suggestion += rng.AnswerSources.Suggestion
		out.Range.AnswerSources.Auto += rng.AnswerSources.Auto
		if rng.MedianAnswerHours != nil {
			answerHours = append(answerHours, *rng.MedianAnswerHours)
		}
	}
	if len(answerHours) > 0 {
		sum := 0.0
		for _, h := range answerHours {
			sum += h
		}
		avg := sum / float64(len(answerHours))
		out.Range.MedianAnswerHours = &avg
	}
	mcp, err := board.TribeMCPHealth(ctx, db, tribeID, fromTS, toTS)
	if err != nil {
		return TribeSummaryReport{}, err
	}
	out.MCP = mcp
	return out, nil
}

func buildProjectSummary(ctx context.Context, db *sql.DB, data, projectID string, start, end time.Time) (board.ProjectSnapshot, board.RangeInsights, error) {
	fromTS := start.Format(time.RFC3339)
	toTS := end.AddDate(0, 0, 1).Format(time.RFC3339)

	groups, att, err := board.CountGroups(ctx, db, projectID)
	if err != nil {
		return board.ProjectSnapshot{}, board.RangeInsights{}, err
	}
	ready, err := ReadyCount(ctx, db, projectID)
	if err != nil {
		return board.ProjectSnapshot{}, board.RangeInsights{}, err
	}
	att.ReadyCount = ready
	if att.OpenRuns, err = board.CountOpenRuns(ctx, db, projectID); err != nil {
		return board.ProjectSnapshot{}, board.RangeInsights{}, err
	}
	if att.Locked, err = board.CountLocks(ctx, db, projectID); err != nil {
		return board.ProjectSnapshot{}, board.RangeInsights{}, err
	}
	if att.OpenQuestions, err = board.CountOpenQuestions(ctx, db, projectID); err != nil {
		return board.ProjectSnapshot{}, board.RangeInsights{}, err
	}
	cfg, _, err := settings.LoadProject(data, projectID)
	if err != nil {
		return board.ProjectSnapshot{}, board.RangeInsights{}, err
	}
	att.ImplementationCap = settings.WorkflowView(cfg).ImplementationCap
	aging, err := board.AgingCards(ctx, db, projectID, 10)
	if err != nil {
		return board.ProjectSnapshot{}, board.RangeInsights{}, err
	}
	rng, err := board.ProjectRange(ctx, db, projectID, fromTS, toTS)
	if err != nil {
		return board.ProjectSnapshot{}, board.RangeInsights{}, err
	}
	return board.ProjectSnapshot{Groups: groups, Attention: att, Aging: aging}, rng, nil
}
