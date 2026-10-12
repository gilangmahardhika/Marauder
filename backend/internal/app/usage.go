package app

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"time"

	"orchestrator/backend/internal/board"
	"orchestrator/backend/internal/settings"
)

const (
	// UsageDefaultDays is the range of a usage read with no dates.
	UsageDefaultDays = 30
	// UsageMaxDays is the longest range a usage read covers.
	UsageMaxDays = 366
)

// UsageScope narrows a usage read to a tribe's projects or one user's calls.
type UsageScope = board.UsageScope

// UsageRangeError is a from or to the usage read refuses; Field names it.
type UsageRangeError struct {
	Field   string
	Message string
}

func (e UsageRangeError) Error() string { return e.Field + ": " + e.Message }

// UsageRow is a board.UsageRow with the model's LLM family from config.yaml.
// FamilyID is empty for a model no family lists any more.
type UsageRow struct {
	Day               string `json:"day"`
	ProjectID         string `json:"projectId"`
	ProjectName       string `json:"projectName"`
	FamilyID          string `json:"familyId"`
	Protocol          string `json:"protocol"`
	ModelID           string `json:"modelId"`
	Task              string `json:"task"`
	UserID            string `json:"userId"`
	UserName          string `json:"userName"`
	UserEmail         string `json:"userEmail"`
	Calls             int64  `json:"calls"`
	InputTokens       int64  `json:"inputTokens"`
	OutputTokens      int64  `json:"outputTokens"`
	CachedInputTokens int64  `json:"cachedInputTokens"`
}

// UsageReport is the token usage from From to To, both inclusive days.
type UsageReport struct {
	From string     `json:"from"`
	To   string     `json:"to"`
	Rows []UsageRow `json:"rows"`
}

// UsageWindow checks from and to, YYYY-MM-DD days in UTC, and fills a blank
// one: to defaults to today and from to 30 days ending at to.
func UsageWindow(from, to string, now time.Time) (time.Time, time.Time, error) {
	day := func(field, raw string) (time.Time, error) {
		at, err := time.Parse(time.DateOnly, raw)
		if err != nil {
			return time.Time{}, UsageRangeError{field, "must be a date as YYYY-MM-DD"}
		}
		return at, nil
	}
	end := time.Date(now.UTC().Year(), now.UTC().Month(), now.UTC().Day(), 0, 0, 0, 0, time.UTC)
	if to != "" {
		var err error
		if end, err = day("to", to); err != nil {
			return time.Time{}, time.Time{}, err
		}
	}
	start := end.AddDate(0, 0, 1-UsageDefaultDays)
	if from != "" {
		var err error
		if start, err = day("from", from); err != nil {
			return time.Time{}, time.Time{}, err
		}
	}
	if start.After(end) {
		return time.Time{}, time.Time{}, UsageRangeError{"from", "must not be after to"}
	}
	if days := int(end.Sub(start).Hours()/24) + 1; days > UsageMaxDays {
		return time.Time{}, time.Time{}, UsageRangeError{"from", fmt.Sprintf("the range may cover at most %d days", UsageMaxDays)}
	}
	return start, end, nil
}

// Usage is ReadUsage of every project for an admin; anyone else gets
// ErrForbidden before the range is checked (RFC-089).
func Usage(ctx context.Context, db *sql.DB, data string, actor User, from, to string, now time.Time) (UsageReport, error) {
	if !actor.Admin {
		return UsageReport{}, ErrForbidden
	}
	return ReadUsage(ctx, db, data, board.UsageScope{}, from, to, now)
}

// TribeUsage is ReadUsage of one tribe's projects for an admin or a tribe
// admin of it. The tribe is checked before the range (RFC-090).
func TribeUsage(ctx context.Context, db *sql.DB, data string, actor User, tribeID, from, to string, now time.Time) (UsageReport, error) {
	if err := ManageTribe(ctx, db, actor, tribeID); err != nil {
		return UsageReport{}, err
	}
	return ReadUsage(ctx, db, data, board.UsageScope{TribeID: tribeID}, from, to, now)
}

// MyUsage is ReadUsage of the actor's own calls in any project (RFC-090).
func MyUsage(ctx context.Context, db *sql.DB, data string, actor User, from, to string, now time.Time) (UsageReport, error) {
	return ReadUsage(ctx, db, data, board.UsageScope{UserID: actor.ID}, from, to, now)
}

// ReadUsage totals the stored model calls within scope in the range and
// names each model's family from the service config.yaml.
func ReadUsage(ctx context.Context, db *sql.DB, data string, scope board.UsageScope, from, to string, now time.Time) (UsageReport, error) {
	start, end, err := UsageWindow(from, to, now)
	if err != nil {
		return UsageReport{}, err
	}
	cfg, _, err := settings.Load(data)
	if err != nil {
		return UsageReport{}, err
	}
	families := map[string]settings.Family{}
	for _, family := range cfg.LLM.Families {
		for _, model := range family.Models {
			if _, seen := families[model.ID]; !seen {
				families[model.ID] = family
			}
		}
	}
	rows, err := board.UsageRows(ctx, db, start.Format(time.DateOnly), end.AddDate(0, 0, 1).Format(time.DateOnly), scope)
	if err != nil {
		return UsageReport{}, err
	}
	report := UsageReport{From: start.Format(time.DateOnly), To: end.Format(time.DateOnly), Rows: make([]UsageRow, 0, len(rows))}
	for _, r := range rows {
		family := families[r.ModelID]
		report.Rows = append(report.Rows, UsageRow{
			Day: r.Day, ProjectID: r.ProjectID, ProjectName: r.ProjectName, FamilyID: family.ID, Protocol: family.Protocol,
			ModelID: r.ModelID, Task: r.Task, UserID: r.UserID, UserName: r.UserName, UserEmail: r.UserEmail,
			Calls: r.Calls, InputTokens: r.InputTokens, OutputTokens: r.OutputTokens, CachedInputTokens: r.CachedInputTokens,
		})
	}
	return report, nil
}

// UsageGroup is the total of the rows that share one value of a dimension.
type UsageGroup struct {
	Key               string
	Calls             int64
	InputTokens       int64
	OutputTokens      int64
	CachedInputTokens int64
}

// Total is input plus output; cached input is counted apart.
func (g UsageGroup) Total() int64 { return g.InputTokens + g.OutputTokens }

// GroupUsage totals rows by one dimension, largest total first, then by key.
func GroupUsage(rows []UsageRow, by string) ([]UsageGroup, error) {
	key := func(r UsageRow) string {
		switch by {
		case "llm":
			return orUnknown(r.FamilyID, "Unknown family")
		case "model":
			return r.ModelID
		case "project":
			return orUnknown(r.ProjectName, r.ProjectID)
		case "user":
			if r.UserID == "" {
				return "No owner"
			}
			return orUnknown(r.UserName, r.UserEmail)
		default:
			return r.Task
		}
	}
	switch by {
	case "llm", "model", "project", "user", "task":
	default:
		return nil, fmt.Errorf("--by must be one of llm, model, project, user, or task")
	}
	byKey := map[string]*UsageGroup{}
	for _, r := range rows {
		k := key(r)
		g := byKey[k]
		if g == nil {
			g = &UsageGroup{Key: k}
			byKey[k] = g
		}
		g.Calls += r.Calls
		g.InputTokens += r.InputTokens
		g.OutputTokens += r.OutputTokens
		g.CachedInputTokens += r.CachedInputTokens
	}
	out := make([]UsageGroup, 0, len(byKey))
	for _, g := range byKey {
		out = append(out, *g)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Total() != out[j].Total() {
			return out[i].Total() > out[j].Total()
		}
		return out[i].Key < out[j].Key
	})
	return out, nil
}

func orUnknown(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
