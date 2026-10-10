// Package conventions is the company catalog of coding rules, stacks, and
// integration skills, a project's stacks and enabled skills, and the rules and
// skills a card's packs carry (RFC-055, RFC-056). Rule and skill text is data
// for the model. Nothing here adds a tool, a command, or a status.
package conventions

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"orchestrator/backend/internal/board"
	"orchestrator/backend/internal/identity"
)

const (
	maxTitle    = 120
	maxBody     = 8000
	maxName     = 60
	maxUseWhen  = 500
	maxDetect   = 20
	maxProjectN = 30
)

var (
	Categories = []string{"naming", "database", "style", "framework", "security", "testing"}

	filePattern = regexp.MustCompile(`^[A-Za-z0-9._*+-]+$`)
	npmPattern  = regexp.MustCompile(`^(@[A-Za-z0-9._~-]+/)?[A-Za-z0-9._~-]+$`)
	pyPattern   = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)
	slugPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,59}$`)
)

type Querier interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

type Markers struct {
	Files   []string `json:"files"`
	NpmDeps []string `json:"npmDeps"`
	PyDeps  []string `json:"pyDeps"`
}

type Rule struct {
	ID          string  `json:"id"`
	Scope       string  `json:"scope"`
	StackID     string  `json:"stackId"`
	Category    string  `json:"category"`
	Title       string  `json:"title"`
	Body        string  `json:"body"`
	Version     int     `json:"version"`
	ContentHash string  `json:"contentHash"`
	Disabled    bool    `json:"disabled"`
	UpdatedBy   *string `json:"updatedBy"`
	UpdatedAt   string  `json:"updatedAt"`
}

type Stack struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Detect      Markers `json:"detect"`
	Version     int     `json:"version"`
	ContentHash string  `json:"contentHash"`
	Disabled    bool    `json:"disabled"`
	UpdatedBy   *string `json:"updatedBy"`
	UpdatedAt   string  `json:"updatedAt"`
}

type Skill struct {
	ID          string  `json:"id"`
	Slug        string  `json:"slug"`
	Title       string  `json:"title"`
	UseWhen     string  `json:"useWhen"`
	Body        string  `json:"body"`
	Version     int     `json:"version"`
	ContentHash string  `json:"contentHash"`
	Disabled    bool    `json:"disabled"`
	UpdatedBy   *string `json:"updatedBy"`
	UpdatedAt   string  `json:"updatedAt"`
}

type Catalog struct {
	Rules  []Rule  `json:"rules"`
	Stacks []Stack `json:"stacks"`
	Skills []Skill `json:"skills"`
}

type RuleInput struct {
	Scope    string `json:"scope"`
	StackID  string `json:"stackId"`
	Category string `json:"category"`
	Title    string `json:"title"`
	Body     string `json:"body"`
	Disabled bool   `json:"disabled"`
}

type StackInput struct {
	Name     string  `json:"name"`
	Detect   Markers `json:"detect"`
	Disabled bool    `json:"disabled"`
}

type SkillInput struct {
	Slug     string `json:"slug"`
	Title    string `json:"title"`
	UseWhen  string `json:"useWhen"`
	Body     string `json:"body"`
	Disabled bool   `json:"disabled"`
}

// Hash is the SHA-256 hex of the parts joined by the byte 0x1e. The migration
// seeds compute the same value in SQL with chr(30).
func Hash(parts ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x1e")))
	return hex.EncodeToString(sum[:])
}

func ruleHash(in RuleInput) string {
	return Hash(in.Scope, in.StackID, in.Category, in.Title, in.Body)
}

func stackHash(name, detectJSON string) string { return Hash(name, detectJSON) }

func skillHash(in SkillInput) string { return Hash(in.Slug, in.Title, in.UseWhen, in.Body) }

func skillRevisionBody(useWhen, body string) string { return "use when: " + useWhen + "\n\n" + body }

func detectJSON(d Markers) string {
	d = Markers{Files: nonNil(d.Files), NpmDeps: nonNil(d.NpmDeps), PyDeps: nonNil(d.PyDeps)}
	raw, _ := json.Marshal(d)
	return string(raw)
}

func parseDetect(raw string) Markers {
	var d Markers
	_ = json.Unmarshal([]byte(raw), &d)
	return Markers{Files: nonNil(d.Files), NpmDeps: nonNil(d.NpmDeps), PyDeps: nonNil(d.PyDeps)}
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func now() string { return time.Now().UTC().Format(time.RFC3339) }

func emailPtr(s sql.NullString) *string {
	if !s.Valid {
		return nil
	}
	return &s.String
}

func Load(ctx context.Context, q Querier) (Catalog, error) {
	cat := Catalog{Rules: []Rule{}, Stacks: []Stack{}, Skills: []Skill{}}
	stacks, err := listStacks(ctx, q, "")
	if err != nil {
		return Catalog{}, err
	}
	cat.Stacks = stacks
	rules, err := listRules(ctx, q, "")
	if err != nil {
		return Catalog{}, err
	}
	cat.Rules = rules
	skills, err := listSkills(ctx, q, "")
	if err != nil {
		return Catalog{}, err
	}
	cat.Skills = skills
	return cat, nil
}

func listStacks(ctx context.Context, q Querier, id string) ([]Stack, error) {
	rows, err := q.QueryContext(ctx, `
		SELECT s.id, s.name, s.detect_json, s.version, s.content_hash, s.disabled, u.email, s.updated_at
		FROM stacks s LEFT JOIN users u ON u.id = s.updated_by
		WHERE $1 = '' OR s.id = $1
		ORDER BY lower(s.name)`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Stack{}
	for rows.Next() {
		var s Stack
		var raw string
		var by sql.NullString
		if err := rows.Scan(&s.ID, &s.Name, &raw, &s.Version, &s.ContentHash, &s.Disabled, &by, &s.UpdatedAt); err != nil {
			return nil, err
		}
		s.Detect = parseDetect(raw)
		s.UpdatedBy = emailPtr(by)
		out = append(out, s)
	}
	return out, rows.Err()
}

func listRules(ctx context.Context, q Querier, id string) ([]Rule, error) {
	rows, err := q.QueryContext(ctx, `
		SELECT r.id, r.scope, COALESCE(r.stack_id, ''), r.category, r.title, r.body, r.version,
			r.content_hash, r.disabled, u.email, r.updated_at
		FROM convention_rules r
		LEFT JOIN users u ON u.id = r.updated_by
		LEFT JOIN stacks s ON s.id = r.stack_id
		WHERE $1 = '' OR r.id = $1
		ORDER BY r.scope, lower(COALESCE(s.name, '')), r.category, lower(r.title), r.id`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Rule{}
	for rows.Next() {
		var r Rule
		var by sql.NullString
		if err := rows.Scan(&r.ID, &r.Scope, &r.StackID, &r.Category, &r.Title, &r.Body, &r.Version,
			&r.ContentHash, &r.Disabled, &by, &r.UpdatedAt); err != nil {
			return nil, err
		}
		r.UpdatedBy = emailPtr(by)
		out = append(out, r)
	}
	return out, rows.Err()
}

func listSkills(ctx context.Context, q Querier, id string) ([]Skill, error) {
	rows, err := q.QueryContext(ctx, `
		SELECT k.id, k.slug, k.title, k.use_when, k.body, k.version, k.content_hash, k.disabled, u.email, k.updated_at
		FROM skills k LEFT JOIN users u ON u.id = k.updated_by
		WHERE $1 = '' OR k.id = $1
		ORDER BY k.slug`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Skill{}
	for rows.Next() {
		var k Skill
		var by sql.NullString
		if err := rows.Scan(&k.ID, &k.Slug, &k.Title, &k.UseWhen, &k.Body, &k.Version, &k.ContentHash,
			&k.Disabled, &by, &k.UpdatedAt); err != nil {
			return nil, err
		}
		k.UpdatedBy = emailPtr(by)
		out = append(out, k)
	}
	return out, rows.Err()
}

func checkLen(fields map[string]string, name, value string, max int) {
	n := utf8.RuneCountInString(value)
	if n == 0 {
		fields[name] = name + " is required"
	} else if n > max {
		fields[name] = fmt.Sprintf("%s must be at most %d characters", name, max)
	}
}

func validCategory(c string) bool {
	for _, v := range Categories {
		if v == c {
			return true
		}
	}
	return false
}

func revise(ctx context.Context, tx *sql.Tx, kind, id string, version int, title, body, hash, actorID, at string) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO convention_revisions (kind, item_id, version, title, body, content_hash, created_by, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		kind, id, version, title, body, hash, actorID, at)
	return err
}

// SaveRule creates a rule when id is empty, else replaces it. A change to the
// hashed fields increments the version and appends a revision. The returned
// field names are the ones that changed.
func SaveRule(ctx context.Context, db *sql.DB, actor identity.User, id string, in RuleInput) (Rule, []string, error) {
	if !actor.Admin {
		return Rule{}, nil, identity.ErrForbidden
	}
	in.Scope = strings.TrimSpace(in.Scope)
	in.StackID = strings.TrimSpace(in.StackID)
	in.Category = strings.TrimSpace(in.Category)
	in.Title = strings.TrimSpace(in.Title)
	in.Body = strings.TrimSpace(in.Body)
	fields := map[string]string{}
	switch in.Scope {
	case "global":
		if in.StackID != "" {
			fields["stackId"] = "a global rule has no stack"
		}
	case "stack":
		if in.StackID == "" {
			fields["stackId"] = "choose a stack"
		} else {
			var n int
			if err := db.QueryRowContext(ctx, `SELECT count(*) FROM stacks WHERE id = $1`, in.StackID).Scan(&n); err != nil {
				return Rule{}, nil, err
			}
			if n == 0 {
				fields["stackId"] = "unknown stack"
			}
		}
	default:
		fields["scope"] = "scope must be global or stack"
	}
	if !validCategory(in.Category) {
		fields["category"] = "category must be one of " + strings.Join(Categories, ", ")
	}
	checkLen(fields, "title", in.Title, maxTitle)
	checkLen(fields, "body", in.Body, maxBody)
	if len(fields) > 0 {
		return Rule{}, nil, &identity.Validation{Fields: fields}
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return Rule{}, nil, err
	}
	defer tx.Rollback()
	at := now()
	hash := ruleHash(in)
	var stackID any
	if in.StackID != "" {
		stackID = in.StackID
	}
	var changed []string
	if id == "" {
		if id, err = board.NewID("rule_"); err != nil {
			return Rule{}, nil, err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO convention_rules (id, scope, stack_id, category, title, body, version, content_hash, disabled, updated_by, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, 1, $7, $8, $9, $10, $10)`,
			id, in.Scope, stackID, in.Category, in.Title, in.Body, hash, in.Disabled, actor.ID, at); err != nil {
			return Rule{}, nil, err
		}
		if err := revise(ctx, tx, "rule", id, 1, in.Title, in.Body, hash, actor.ID, at); err != nil {
			return Rule{}, nil, err
		}
		changed = []string{"scope", "stackId", "category", "title", "body", "disabled"}
	} else {
		var cur RuleInput
		var version int
		var curHash string
		err := tx.QueryRowContext(ctx, `
			SELECT scope, COALESCE(stack_id, ''), category, title, body, disabled, version, content_hash
			FROM convention_rules WHERE id = $1 FOR UPDATE`, id).
			Scan(&cur.Scope, &cur.StackID, &cur.Category, &cur.Title, &cur.Body, &cur.Disabled, &version, &curHash)
		if errors.Is(err, sql.ErrNoRows) {
			return Rule{}, nil, identity.ErrNotFound
		}
		if err != nil {
			return Rule{}, nil, err
		}
		changed = diff(map[string][2]string{
			"scope": {cur.Scope, in.Scope}, "stackId": {cur.StackID, in.StackID}, "category": {cur.Category, in.Category},
			"title": {cur.Title, in.Title}, "body": {cur.Body, in.Body},
			"disabled": {fmt.Sprint(cur.Disabled), fmt.Sprint(in.Disabled)},
		})
		if len(changed) > 0 {
			if hash != curHash {
				version++
				if err := revise(ctx, tx, "rule", id, version, in.Title, in.Body, hash, actor.ID, at); err != nil {
					return Rule{}, nil, err
				}
			}
			if _, err := tx.ExecContext(ctx, `
				UPDATE convention_rules SET scope = $2, stack_id = $3, category = $4, title = $5, body = $6,
					version = $7, content_hash = $8, disabled = $9, updated_by = $10, updated_at = $11
				WHERE id = $1`,
				id, in.Scope, stackID, in.Category, in.Title, in.Body, version, hash, in.Disabled, actor.ID, at); err != nil {
				return Rule{}, nil, err
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return Rule{}, nil, err
	}
	rules, err := listRules(ctx, db, id)
	if err != nil {
		return Rule{}, nil, err
	}
	if len(rules) == 0 {
		return Rule{}, nil, identity.ErrNotFound
	}
	return rules[0], changed, nil
}

func SaveStack(ctx context.Context, db *sql.DB, actor identity.User, id string, in StackInput) (Stack, []string, error) {
	if !actor.Admin {
		return Stack{}, nil, identity.ErrForbidden
	}
	in.Name = strings.TrimSpace(in.Name)
	in.Detect = Markers{Files: trimAll(in.Detect.Files), NpmDeps: trimAll(in.Detect.NpmDeps), PyDeps: trimAll(in.Detect.PyDeps)}
	fields := map[string]string{}
	checkLen(fields, "name", in.Name, maxName)
	if _, ok := fields["name"]; !ok {
		var n int
		if err := db.QueryRowContext(ctx, `SELECT count(*) FROM stacks WHERE lower(name) = lower($1) AND id <> $2`, in.Name, id).Scan(&n); err != nil {
			return Stack{}, nil, err
		}
		if n > 0 {
			fields["name"] = "a stack with this name already exists"
		}
	}
	checkList(fields, "detect.files", in.Detect.Files, filePattern, "file names may use letters, digits, '.', '_', '*', '+', and '-', with no slash")
	checkList(fields, "detect.npmDeps", in.Detect.NpmDeps, npmPattern, "npm package names only, such as react or @sveltejs/kit")
	checkList(fields, "detect.pyDeps", in.Detect.PyDeps, pyPattern, "Python package names may use letters, digits, '.', '_', and '-'")
	if len(fields) > 0 {
		return Stack{}, nil, &identity.Validation{Fields: fields}
	}

	raw := detectJSON(in.Detect)
	hash := stackHash(in.Name, raw)
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return Stack{}, nil, err
	}
	defer tx.Rollback()
	at := now()
	var changed []string
	if id == "" {
		if id, err = board.NewID("stk_"); err != nil {
			return Stack{}, nil, err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO stacks (id, name, detect_json, version, content_hash, disabled, updated_by, created_at, updated_at)
			VALUES ($1, $2, $3, 1, $4, $5, $6, $7, $7)`,
			id, in.Name, raw, hash, in.Disabled, actor.ID, at); err != nil {
			return Stack{}, nil, err
		}
		if err := revise(ctx, tx, "stack", id, 1, in.Name, raw, hash, actor.ID, at); err != nil {
			return Stack{}, nil, err
		}
		changed = []string{"name", "detect", "disabled"}
	} else {
		var name, curRaw, curHash string
		var disabled bool
		var version int
		err := tx.QueryRowContext(ctx, `
			SELECT name, detect_json, disabled, version, content_hash FROM stacks WHERE id = $1 FOR UPDATE`, id).
			Scan(&name, &curRaw, &disabled, &version, &curHash)
		if errors.Is(err, sql.ErrNoRows) {
			return Stack{}, nil, identity.ErrNotFound
		}
		if err != nil {
			return Stack{}, nil, err
		}
		changed = diff(map[string][2]string{
			"name": {name, in.Name}, "detect": {detectJSON(parseDetect(curRaw)), raw},
			"disabled": {fmt.Sprint(disabled), fmt.Sprint(in.Disabled)},
		})
		if len(changed) > 0 {
			if hash != curHash {
				version++
				if err := revise(ctx, tx, "stack", id, version, in.Name, raw, hash, actor.ID, at); err != nil {
					return Stack{}, nil, err
				}
			}
			if _, err := tx.ExecContext(ctx, `
				UPDATE stacks SET name = $2, detect_json = $3, version = $4, content_hash = $5, disabled = $6,
					updated_by = $7, updated_at = $8
				WHERE id = $1`,
				id, in.Name, raw, version, hash, in.Disabled, actor.ID, at); err != nil {
				return Stack{}, nil, err
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return Stack{}, nil, err
	}
	stacks, err := listStacks(ctx, db, id)
	if err != nil {
		return Stack{}, nil, err
	}
	if len(stacks) == 0 {
		return Stack{}, nil, identity.ErrNotFound
	}
	return stacks[0], changed, nil
}

func SaveSkill(ctx context.Context, db *sql.DB, actor identity.User, id string, in SkillInput) (Skill, []string, error) {
	if !actor.Admin {
		return Skill{}, nil, identity.ErrForbidden
	}
	in.Slug = strings.TrimSpace(in.Slug)
	in.Title = strings.TrimSpace(in.Title)
	in.UseWhen = strings.TrimSpace(in.UseWhen)
	in.Body = strings.TrimSpace(in.Body)
	fields := map[string]string{}
	if !slugPattern.MatchString(in.Slug) {
		fields["slug"] = "slug is 2 to 60 lowercase letters, digits, and '-', starting with a letter or digit"
	} else {
		var n int
		if err := db.QueryRowContext(ctx, `SELECT count(*) FROM skills WHERE slug = $1 AND id <> $2`, in.Slug, id).Scan(&n); err != nil {
			return Skill{}, nil, err
		}
		if n > 0 {
			fields["slug"] = "a skill with this slug already exists"
		}
	}
	checkLen(fields, "title", in.Title, maxTitle)
	checkLen(fields, "useWhen", in.UseWhen, maxUseWhen)
	checkLen(fields, "body", in.Body, maxBody)
	if len(fields) > 0 {
		return Skill{}, nil, &identity.Validation{Fields: fields}
	}

	hash := skillHash(in)
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return Skill{}, nil, err
	}
	defer tx.Rollback()
	at := now()
	var changed []string
	if id == "" {
		if id, err = board.NewID("skl_"); err != nil {
			return Skill{}, nil, err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO skills (id, slug, title, use_when, body, version, content_hash, disabled, updated_by, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, 1, $6, $7, $8, $9, $9)`,
			id, in.Slug, in.Title, in.UseWhen, in.Body, hash, in.Disabled, actor.ID, at); err != nil {
			return Skill{}, nil, err
		}
		if err := revise(ctx, tx, "skill", id, 1, in.Slug, skillRevisionBody(in.UseWhen, in.Body), hash, actor.ID, at); err != nil {
			return Skill{}, nil, err
		}
		changed = []string{"slug", "title", "useWhen", "body", "disabled"}
	} else {
		var cur SkillInput
		var version int
		var curHash string
		err := tx.QueryRowContext(ctx, `
			SELECT slug, title, use_when, body, disabled, version, content_hash FROM skills WHERE id = $1 FOR UPDATE`, id).
			Scan(&cur.Slug, &cur.Title, &cur.UseWhen, &cur.Body, &cur.Disabled, &version, &curHash)
		if errors.Is(err, sql.ErrNoRows) {
			return Skill{}, nil, identity.ErrNotFound
		}
		if err != nil {
			return Skill{}, nil, err
		}
		changed = diff(map[string][2]string{
			"slug": {cur.Slug, in.Slug}, "title": {cur.Title, in.Title}, "useWhen": {cur.UseWhen, in.UseWhen},
			"body": {cur.Body, in.Body}, "disabled": {fmt.Sprint(cur.Disabled), fmt.Sprint(in.Disabled)},
		})
		if len(changed) > 0 {
			if hash != curHash {
				version++
				if err := revise(ctx, tx, "skill", id, version, in.Slug, skillRevisionBody(in.UseWhen, in.Body), hash, actor.ID, at); err != nil {
					return Skill{}, nil, err
				}
			}
			if _, err := tx.ExecContext(ctx, `
				UPDATE skills SET slug = $2, title = $3, use_when = $4, body = $5, version = $6, content_hash = $7,
					disabled = $8, updated_by = $9, updated_at = $10
				WHERE id = $1`,
				id, in.Slug, in.Title, in.UseWhen, in.Body, version, hash, in.Disabled, actor.ID, at); err != nil {
				return Skill{}, nil, err
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return Skill{}, nil, err
	}
	skills, err := listSkills(ctx, db, id)
	if err != nil {
		return Skill{}, nil, err
	}
	if len(skills) == 0 {
		return Skill{}, nil, identity.ErrNotFound
	}
	return skills[0], changed, nil
}

// Delete removes an item nothing names and disables one that is still named:
// a stack named by a rule or a project, a skill named by a project or a card.
// Revisions stay. It reports whether the row was removed.
func Delete(ctx context.Context, db *sql.DB, actor identity.User, kind, id string) (bool, error) {
	if !actor.Admin {
		return false, identity.ErrForbidden
	}
	var table, refs string
	switch kind {
	case "rule":
		table, refs = "convention_rules", `SELECT 0`
	case "stack":
		table, refs = "stacks", `SELECT (SELECT count(*) FROM convention_rules WHERE stack_id = $1) + (SELECT count(*) FROM project_stacks WHERE stack_id = $1)`
	case "skill":
		table, refs = "skills", `SELECT (SELECT count(*) FROM project_skills WHERE skill_id = $1) + (SELECT count(*) FROM card_skills WHERE skill_id = $1)`
	default:
		return false, identity.ErrNotFound
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	var found string
	err = tx.QueryRowContext(ctx, `SELECT id FROM `+table+` WHERE id = $1 FOR UPDATE`, id).Scan(&found)
	if errors.Is(err, sql.ErrNoRows) {
		return false, identity.ErrNotFound
	}
	if err != nil {
		return false, err
	}
	var n int
	if kind == "rule" {
		err = tx.QueryRowContext(ctx, refs).Scan(&n)
	} else {
		err = tx.QueryRowContext(ctx, refs, id).Scan(&n)
	}
	if err != nil {
		return false, err
	}
	deleted := n == 0
	if deleted {
		_, err = tx.ExecContext(ctx, `DELETE FROM `+table+` WHERE id = $1`, id)
	} else {
		_, err = tx.ExecContext(ctx, `UPDATE `+table+` SET disabled = true, updated_by = $2, updated_at = $3 WHERE id = $1`, id, actor.ID, now())
	}
	if err != nil {
		return false, err
	}
	return deleted, tx.Commit()
}

func trimAll(in []string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

func checkList(fields map[string]string, name string, values []string, pattern *regexp.Regexp, msg string) {
	if len(values) > maxDetect {
		fields[name] = fmt.Sprintf("at most %d entries", maxDetect)
		return
	}
	for _, v := range values {
		if len(v) > 214 || !pattern.MatchString(v) {
			fields[name] = msg
			return
		}
	}
}

func diff(pairs map[string][2]string) []string {
	out := []string{}
	for name, p := range pairs {
		if p[0] != p[1] {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}
