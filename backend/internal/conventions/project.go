package conventions

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

type SkillRef struct {
	ID      string `json:"id"`
	Slug    string `json:"slug"`
	Title   string `json:"title"`
	UseWhen string `json:"useWhen"`
}

// Item is one rule or skill as a pack carries it. Label is "global" or the
// stack name for a rule.
type Item struct {
	Kind     string
	ID       string
	Version  int
	Hash     string
	Title    string
	Label    string
	Category string
	Slug     string
	UseWhen  string
	Body     string
}

type Execer interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

func strings1(ctx context.Context, q Querier, query string, args ...any) ([]string, error) {
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func checkEnabled(ctx context.Context, q Querier, fields map[string]string, name, table string, ids []string) error {
	if len(ids) > maxProjectN {
		fields[name] = fmt.Sprintf("at most %d entries", maxProjectN)
		return nil
	}
	if len(ids) == 0 {
		return nil
	}
	found, err := strings1(ctx, q, `SELECT id FROM `+table+` WHERE NOT disabled AND id = ANY(string_to_array($1, ','))`, strings.Join(ids, ","))
	if err != nil {
		return err
	}
	ok := map[string]bool{}
	for _, id := range found {
		ok[id] = true
	}
	for _, id := range ids {
		if !ok[id] {
			fields[name] = "unknown or disabled: " + id
			return nil
		}
	}
	return nil
}

// Stacks lists the enabled stacks with their markers, for detection.
func Stacks(ctx context.Context, q Querier) ([]Stack, error) {
	all, err := listStacks(ctx, q, "")
	if err != nil {
		return nil, err
	}
	out := []Stack{}
	for _, s := range all {
		if !s.Disabled {
			out = append(out, s)
		}
	}
	return out, nil
}

// ReplaceDetected swaps the project's detected stacks. Stacks a project adds
// by hand live in project_conventions.
func ReplaceDetected(ctx context.Context, db *sql.DB, projectID string, stackIDs []string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM project_stacks WHERE project_id = $1 AND source = 'detected'`, projectID); err != nil {
		return err
	}
	at := now()
	for _, id := range stackIDs {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO project_stacks (project_id, stack_id, source, added_by, created_at)
			VALUES ($1, $2, 'detected', NULL, $3) ON CONFLICT (project_id, stack_id) DO NOTHING`,
			projectID, id, at); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// EnabledSkills is the skills that apply to the project after the tribe and
// project layers (RFC-104). The planner may attach only these.
func EnabledSkills(ctx context.Context, q Querier, projectID string) ([]Skill, error) {
	r, err := Effective(ctx, q, projectID)
	if err != nil {
		return nil, err
	}
	out := make([]Skill, 0, len(r.Skills))
	for _, k := range r.Skills {
		out = append(out, Skill{ID: k.ID, Slug: k.Slug, Title: k.Title, UseWhen: k.UseWhen, Body: k.Body, Version: k.Version, ContentHash: k.Hash})
	}
	return out, nil
}

// SetCardSkills stores a card's skills. Call it in the card's transaction.
func SetCardSkills(ctx context.Context, q Execer, projectID, cardID string, skillIDs []string) error {
	for _, id := range skillIDs {
		if _, err := q.ExecContext(ctx, `
			INSERT INTO card_skills (project_id, card_id, skill_id) VALUES ($1, $2, $3)
			ON CONFLICT DO NOTHING`, projectID, cardID, id); err != nil {
			return err
		}
	}
	return nil
}

// CardSkills is the skills stored on one card.
func CardSkills(ctx context.Context, q Querier, projectID, cardID string) ([]SkillRef, error) {
	rows, err := q.QueryContext(ctx, `
		SELECT k.id, k.slug, k.title, k.use_when FROM card_skills cs JOIN skills k ON k.id = cs.skill_id
		WHERE cs.project_id = $1 AND cs.card_id = $2 ORDER BY k.slug`, projectID, cardID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SkillRef{}
	for rows.Next() {
		var k SkillRef
		if err := rows.Scan(&k.ID, &k.Slug, &k.Title, &k.UseWhen); err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

// ForCard reads what a card's packs carry: the project's rules after its
// layers and the card's context, then the planner's skills and the card's
// attached skills, less the ones it detached (RFC-104, RFC-105).
func ForCard(ctx context.Context, q Querier, projectID, cardID string) ([]Item, error) {
	cs, err := CardContext(ctx, q, projectID, cardID)
	if err != nil {
		return nil, err
	}
	out := []Item{}
	for _, r := range cs.Rules {
		if r.State != "on" {
			continue
		}
		label := r.StackName
		if r.Scope == "global" {
			label = "global"
		}
		out = append(out, Item{Kind: "rule", ID: r.ID, Version: r.Version, Hash: r.Hash, Title: r.Title, Label: label, Category: r.Category, Body: r.Body})
	}
	for _, k := range cs.Skills {
		if k.State != "on" {
			continue
		}
		out = append(out, Item{Kind: "skill", ID: k.ID, Version: k.Version, Hash: k.Hash, Title: k.Title, Slug: k.Slug, UseWhen: k.UseWhen, Body: k.Body})
	}
	return out, nil
}
