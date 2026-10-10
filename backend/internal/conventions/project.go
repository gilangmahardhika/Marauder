package conventions

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"orchestrator/backend/internal/identity"
)

type ProjectStack struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Source string `json:"source"`
}

type ProjectRule struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Scope     string `json:"scope"`
	StackName string `json:"stackName"`
	Category  string `json:"category"`
	Version   int    `json:"version"`
}

type Project struct {
	Stacks   []ProjectStack `json:"stacks"`
	SkillIDs []string       `json:"skillIds"`
	Rules    []ProjectRule  `json:"rules"`
}

type SkillRef struct {
	ID    string `json:"id"`
	Slug  string `json:"slug"`
	Title string `json:"title"`
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

func LoadProject(ctx context.Context, q Querier, projectID string) (Project, error) {
	p := Project{Stacks: []ProjectStack{}, SkillIDs: []string{}, Rules: []ProjectRule{}}
	rows, err := q.QueryContext(ctx, `
		SELECT s.id, s.name, ps.source FROM project_stacks ps JOIN stacks s ON s.id = ps.stack_id
		WHERE ps.project_id = $1 ORDER BY lower(s.name)`, projectID)
	if err != nil {
		return Project{}, err
	}
	for rows.Next() {
		var s ProjectStack
		if err := rows.Scan(&s.ID, &s.Name, &s.Source); err != nil {
			rows.Close()
			return Project{}, err
		}
		p.Stacks = append(p.Stacks, s)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return Project{}, err
	}
	ids, err := strings1(ctx, q, `SELECT skill_id FROM project_skills WHERE project_id = $1 ORDER BY skill_id`, projectID)
	if err != nil {
		return Project{}, err
	}
	p.SkillIDs = ids
	rows, err = q.QueryContext(ctx, rulesForProject+` ORDER BY (r.scope = 'stack'), lower(COALESCE(s.name, '')), r.category, lower(r.title), r.id`, projectID)
	if err != nil {
		return Project{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var r ProjectRule
		var hash, body string
		if err := rows.Scan(&r.ID, &r.Version, &hash, &r.Title, &r.StackName, &r.Category, &body, &r.Scope); err != nil {
			return Project{}, err
		}
		if r.Scope == "global" {
			r.StackName = ""
		}
		p.Rules = append(p.Rules, r)
	}
	return p, rows.Err()
}

const rulesForProject = `
	SELECT r.id, r.version, r.content_hash, r.title, COALESCE(s.name, 'global'), r.category, r.body, r.scope
	FROM convention_rules r LEFT JOIN stacks s ON s.id = r.stack_id
	WHERE NOT r.disabled AND (r.scope = 'global' OR (NOT s.disabled AND r.stack_id IN (
		SELECT stack_id FROM project_stacks WHERE project_id = $1)))`

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

// SaveProject sets the stacks a Lead Developer added by hand and the enabled
// skills. A stack that is already detected stays detected.
func SaveProject(ctx context.Context, db *sql.DB, actor identity.User, projectID string, manualStackIDs, skillIDs []string) (Project, error) {
	if actor.Role != "lead_developer" {
		return Project{}, identity.ErrForbidden
	}
	manualStackIDs = trimAll(manualStackIDs)
	skillIDs = trimAll(skillIDs)
	fields := map[string]string{}
	if err := checkEnabled(ctx, db, fields, "manualStackIds", "stacks", manualStackIDs); err != nil {
		return Project{}, err
	}
	if err := checkEnabled(ctx, db, fields, "skillIds", "skills", skillIDs); err != nil {
		return Project{}, err
	}
	if len(fields) > 0 {
		return Project{}, &identity.Validation{Fields: fields}
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return Project{}, err
	}
	defer tx.Rollback()
	at := now()
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM project_stacks WHERE project_id = $1 AND source = 'manual' AND NOT (stack_id = ANY(string_to_array($2, ',')))`,
		projectID, strings.Join(manualStackIDs, ",")); err != nil {
		return Project{}, err
	}
	for _, id := range manualStackIDs {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO project_stacks (project_id, stack_id, source, added_by, created_at)
			VALUES ($1, $2, 'manual', $3, $4) ON CONFLICT (project_id, stack_id) DO NOTHING`,
			projectID, id, actor.ID, at); err != nil {
			return Project{}, err
		}
	}
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM project_skills WHERE project_id = $1 AND NOT (skill_id = ANY(string_to_array($2, ',')))`,
		projectID, strings.Join(skillIDs, ",")); err != nil {
		return Project{}, err
	}
	for _, id := range skillIDs {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO project_skills (project_id, skill_id, added_by, created_at)
			VALUES ($1, $2, $3, $4) ON CONFLICT (project_id, skill_id) DO NOTHING`,
			projectID, id, actor.ID, at); err != nil {
			return Project{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return Project{}, err
	}
	return LoadProject(ctx, db, projectID)
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

// ReplaceDetected swaps the project's detected stacks and keeps the ones added
// by hand.
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

// EnabledSkills is the project's enabled skills that are not disabled in the
// catalog. The planner may attach only these.
func EnabledSkills(ctx context.Context, q Querier, projectID string) ([]Skill, error) {
	rows, err := q.QueryContext(ctx, `
		SELECT k.id, k.slug, k.title, k.use_when, k.body, k.version, k.content_hash
		FROM project_skills ps JOIN skills k ON k.id = ps.skill_id
		WHERE ps.project_id = $1 AND NOT k.disabled ORDER BY k.slug`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Skill{}
	for rows.Next() {
		var k Skill
		if err := rows.Scan(&k.ID, &k.Slug, &k.Title, &k.UseWhen, &k.Body, &k.Version, &k.ContentHash); err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, rows.Err()
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
		SELECT k.id, k.slug, k.title FROM card_skills cs JOIN skills k ON k.id = cs.skill_id
		WHERE cs.project_id = $1 AND cs.card_id = $2 ORDER BY k.slug`, projectID, cardID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SkillRef{}
	for rows.Next() {
		var k SkillRef
		if err := rows.Scan(&k.ID, &k.Slug, &k.Title); err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

// ForCard reads what a card's packs carry: enabled global rules, enabled rules
// of the project's enabled stacks, then the card's enabled skills.
func ForCard(ctx context.Context, q Querier, projectID, cardID string) ([]Item, error) {
	rows, err := q.QueryContext(ctx, rulesForProject+` ORDER BY (r.scope = 'stack'), lower(COALESCE(s.name, '')), r.category, lower(r.title), r.id`, projectID)
	if err != nil {
		return nil, err
	}
	out := []Item{}
	for rows.Next() {
		it := Item{Kind: "rule"}
		var scope string
		if err := rows.Scan(&it.ID, &it.Version, &it.Hash, &it.Title, &it.Label, &it.Category, &it.Body, &scope); err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, it)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows, err = q.QueryContext(ctx, `
		SELECT k.id, k.version, k.content_hash, k.title, k.slug, k.use_when, k.body
		FROM card_skills cs JOIN skills k ON k.id = cs.skill_id
		WHERE cs.project_id = $1 AND cs.card_id = $2 AND NOT k.disabled ORDER BY k.slug`, projectID, cardID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		it := Item{Kind: "skill"}
		if err := rows.Scan(&it.ID, &it.Version, &it.Hash, &it.Title, &it.Slug, &it.UseWhen, &it.Body); err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}
