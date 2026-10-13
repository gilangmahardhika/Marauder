package conventions_test

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pressly/goose/v3"

	"orchestrator/backend/internal/board"
	"orchestrator/backend/internal/board/boardtest"
	"orchestrator/backend/internal/conventions"
	"orchestrator/backend/internal/identity"
	"orchestrator/backend/migrations"
)

func user(t *testing.T, db *sql.DB, email, role string, admin bool) identity.User {
	t.Helper()
	u, err := identity.Insert(context.Background(), db, email, "password-long-1", role, admin)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func TestSeedHashesMatchGo(t *testing.T) {
	db := boardtest.Open(t)
	cat, err := conventions.Load(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	if len(cat.Stacks) != 6 || len(cat.Rules) != 2 || len(cat.Skills) != 1 {
		t.Fatalf("seeds: %d stacks, %d rules, %d skills", len(cat.Stacks), len(cat.Rules), len(cat.Skills))
	}
	admin := user(t, db, "admin@example.com", "lead_developer", true)
	for _, r := range cat.Rules {
		saved, changed, err := conventions.SaveRule(context.Background(), db, admin, r.ID, conventions.RuleInput{
			Scope: r.Scope, StackID: r.StackID, Category: r.Category, Title: r.Title, Body: r.Body,
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(changed) != 0 || saved.Version != 1 || saved.ContentHash != r.ContentHash {
			t.Fatalf("rule %s: changed %v, version %d, hash %s want %s", r.ID, changed, saved.Version, saved.ContentHash, r.ContentHash)
		}
	}
	for _, s := range cat.Stacks {
		saved, changed, err := conventions.SaveStack(context.Background(), db, admin, s.ID, conventions.StackInput{Name: s.Name, Detect: s.Detect})
		if err != nil {
			t.Fatal(err)
		}
		if len(changed) != 0 || saved.ContentHash != s.ContentHash {
			t.Fatalf("stack %s: changed %v, hash %s want %s", s.ID, changed, saved.ContentHash, s.ContentHash)
		}
	}
	k := cat.Skills[0]
	saved, changed, err := conventions.SaveSkill(context.Background(), db, admin, k.ID, conventions.SkillInput{
		Slug: k.Slug, Title: k.Title, UseWhen: k.UseWhen, Body: k.Body,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(changed) != 0 || saved.ContentHash != k.ContentHash {
		t.Fatalf("skill: changed %v, hash %s want %s", changed, saved.ContentHash, k.ContentHash)
	}
}

func TestConventionRevisions(t *testing.T) {
	db := boardtest.Open(t)
	ctx := context.Background()
	admin := user(t, db, "admin@example.com", "lead_developer", true)
	lead := user(t, db, "lead@example.com", "lead_developer", false)

	in := conventions.RuleInput{Scope: "stack", StackID: "stk_go", Category: "naming", Title: "Go names", Body: "Exported names are MixedCaps."}
	if _, _, err := conventions.SaveRule(ctx, db, lead, "", in); !errors.Is(err, identity.ErrForbidden) {
		t.Fatalf("lead save: %v", err)
	}
	rule, _, err := conventions.SaveRule(ctx, db, admin, "", in)
	if err != nil {
		t.Fatal(err)
	}
	if rule.Version != 1 || rule.UpdatedBy == nil || *rule.UpdatedBy != "admin@example.com" {
		t.Fatalf("created: %+v", rule)
	}
	first := rule.ContentHash

	in.Body = "Exported names are MixedCaps. Acronyms keep one case: ID, URL."
	rule, changed, err := conventions.SaveRule(ctx, db, admin, rule.ID, in)
	if err != nil {
		t.Fatal(err)
	}
	if rule.Version != 2 || rule.ContentHash == first || strings.Join(changed, ",") != "body" {
		t.Fatalf("edited: version %d, changed %v", rule.Version, changed)
	}

	same, changed, err := conventions.SaveRule(ctx, db, admin, rule.ID, in)
	if err != nil {
		t.Fatal(err)
	}
	if same.Version != 2 || len(changed) != 0 {
		t.Fatalf("same text: version %d, changed %v", same.Version, changed)
	}

	in.Disabled = true
	off, changed, err := conventions.SaveRule(ctx, db, admin, rule.ID, in)
	if err != nil {
		t.Fatal(err)
	}
	if off.Version != 2 || !off.Disabled || strings.Join(changed, ",") != "disabled" {
		t.Fatalf("disable: version %d, changed %v", off.Version, changed)
	}

	var oldBody, oldHash string
	if err := db.QueryRow(`
		SELECT body, content_hash FROM convention_revisions WHERE kind = 'rule' AND item_id = $1 AND version = 1`, rule.ID).
		Scan(&oldBody, &oldHash); err != nil {
		t.Fatal(err)
	}
	if oldBody != "Exported names are MixedCaps." || oldHash != first {
		t.Fatalf("revision 1: %q %s", oldBody, oldHash)
	}
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM convention_revisions WHERE kind = 'rule' AND item_id = $1`, rule.ID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("revisions: %d", n)
	}

	long := in
	long.Body = strings.Repeat("x", 8001)
	var ve *identity.Validation
	if _, _, err := conventions.SaveRule(ctx, db, admin, rule.ID, long); !errors.As(err, &ve) || ve.Fields["body"] == "" {
		t.Fatalf("long body: %v", err)
	}
	bad := conventions.RuleInput{Scope: "global", StackID: "stk_go", Category: "naming", Title: "t", Body: "b"}
	if _, _, err := conventions.SaveRule(ctx, db, admin, "", bad); !errors.As(err, &ve) || ve.Fields["stackId"] == "" {
		t.Fatalf("global with stack: %v", err)
	}
	if _, _, err := conventions.SaveStack(ctx, db, admin, "", conventions.StackInput{Name: "go"}); !errors.As(err, &ve) || ve.Fields["name"] == "" {
		t.Fatalf("duplicate stack name: %v", err)
	}
	if _, _, err := conventions.SaveStack(ctx, db, admin, "", conventions.StackInput{Name: "Rust", Detect: conventions.Markers{Files: []string{"../Cargo.toml"}}}); !errors.As(err, &ve) || ve.Fields["detect.files"] == "" {
		t.Fatalf("file with slash: %v", err)
	}
	if _, _, err := conventions.SaveSkill(ctx, db, admin, "", conventions.SkillInput{Slug: "oauth2-integration", Title: "t", UseWhen: "u", Body: "b"}); !errors.As(err, &ve) || ve.Fields["slug"] == "" {
		t.Fatalf("duplicate slug: %v", err)
	}

	deleted, err := conventions.Delete(ctx, db, admin, "stack", "stk_go")
	if err != nil || deleted {
		t.Fatalf("delete named stack: %v %v", deleted, err)
	}
	deleted, err = conventions.Delete(ctx, db, admin, "rule", rule.ID)
	if err != nil || !deleted {
		t.Fatalf("delete rule: %v %v", deleted, err)
	}
	if err := db.QueryRow(`SELECT count(*) FROM convention_revisions WHERE item_id = $1`, rule.ID).Scan(&n); err != nil || n != 2 {
		t.Fatalf("revisions after delete: %d %v", n, err)
	}
	if _, err := conventions.Delete(ctx, db, admin, "rule", rule.ID); !errors.Is(err, identity.ErrNotFound) {
		t.Fatalf("delete twice: %v", err)
	}
}

func write(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDetectStacks(t *testing.T) {
	db := boardtest.Open(t)
	stacks, err := conventions.Stacks(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	write(t, filepath.Join(root, "go.mod"), "module example\n")
	write(t, filepath.Join(root, "web", "package.json"),
		`{"dependencies":{"@sveltejs/kit":"^2"},"devDependencies":{"typescript":"^5"}}`)
	write(t, filepath.Join(root, "web", "node_modules", "react", "package.json"), `{"dependencies":{"react":"18"}}`)
	write(t, filepath.Join(root, "web", "node_modules", "pkg", "manage.py"), "")

	outside := t.TempDir()
	write(t, filepath.Join(outside, "pyproject.toml"), "[project]\ndependencies = [\"django>=5\"]\n")
	if err := os.Symlink(outside, filepath.Join(root, "linked")); err != nil {
		t.Fatal(err)
	}

	got, err := conventions.Detect(root, stacks)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(got, ",") != "stk_go,stk_sveltekit,stk_typescript" {
		t.Fatalf("detected %v", got)
	}

	py := t.TempDir()
	write(t, filepath.Join(py, "pyproject.toml"), "[project]\nname = \"app\"\ndependencies = [\n  \"Django>=5\",\n  \"requests\",\n]\n")
	got, err = conventions.Detect(py, stacks)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(got, ",") != "stk_django,stk_python" {
		t.Fatalf("python detected %v", got)
	}
}

func effective(t *testing.T, db *sql.DB) (stacks, rules, skills []string) {
	t.Helper()
	r, err := conventions.Effective(context.Background(), db, "proj_default")
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range r.Stacks {
		stacks = append(stacks, s.ID+":"+s.Origin)
	}
	for _, x := range r.Rules {
		rules = append(rules, x.ID+":"+x.Origin)
	}
	for _, k := range r.Skills {
		skills = append(skills, k.ID+":"+k.Origin)
	}
	return stacks, rules, skills
}

func TestEffectiveConventions(t *testing.T) {
	db := boardtest.Open(t)
	ctx := context.Background()
	admin := user(t, db, "admin@example.com", "product_owner", true)
	goRule, _, err := conventions.SaveRule(ctx, db, admin, "", conventions.RuleInput{
		Scope: "stack", StackID: "stk_go", Category: "style", Title: "Go errors", Body: "Wrap errors.",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := conventions.ReplaceDetected(ctx, db, "proj_default", []string{"stk_react"}); err != nil {
		t.Fatal(err)
	}
	if _, err := conventions.SaveTribe(ctx, db, admin, "tribe_default", conventions.TribeLayer{
		StackIDs: []string{"stk_go"}, SkillIDs: []string{"skl_oauth2"}, RulesOff: []string{"rule_naming"},
	}); err != nil {
		t.Fatal(err)
	}
	stacks, rules, skills := effective(t, db)
	if strings.Join(stacks, ",") != "stk_go:tribe,stk_react:detected" {
		t.Fatalf("tribe stacks = %v", stacks)
	}
	if strings.Join(rules, ",") != "rule_database:catalog,"+goRule.ID+":catalog" {
		t.Fatalf("tribe rules = %v", rules)
	}
	if strings.Join(skills, ",") != "skl_oauth2:tribe" {
		t.Fatalf("tribe skills = %v", skills)
	}

	if _, err := conventions.SaveProject(ctx, db, admin, "proj_default", conventions.Choices{
		Stacks: conventions.OnOff{On: []string{"stk_python"}, Off: []string{"stk_react"}},
		Skills: conventions.OnOff{Off: []string{"skl_oauth2"}},
		Rules:  conventions.OnOff{On: []string{"rule_naming"}, Off: []string{"rule_database"}},
	}); err != nil {
		t.Fatal(err)
	}
	stacks, rules, skills = effective(t, db)
	if strings.Join(stacks, ",") != "stk_go:tribe,stk_python:project" {
		t.Fatalf("project stacks = %v", stacks)
	}
	if strings.Join(rules, ",") != "rule_naming:project,"+goRule.ID+":catalog" {
		t.Fatalf("project rules = %v", rules)
	}
	if len(skills) != 0 {
		t.Fatalf("project skills = %v", skills)
	}

	if _, err := conventions.SaveProject(ctx, db, admin, "proj_default", conventions.Choices{
		Skills: conventions.OnOff{On: []string{"skl_oauth2"}},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE stacks SET disabled = true WHERE id = 'stk_go'`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE skills SET disabled = true WHERE id = 'skl_oauth2'`); err != nil {
		t.Fatal(err)
	}
	stacks, rules, skills = effective(t, db)
	if strings.Join(stacks, ",") != "stk_react:detected" || strings.Join(rules, ",") != "rule_database:catalog" || len(skills) != 0 {
		t.Fatalf("after disabling: stacks %v rules %v skills %v", stacks, rules, skills)
	}
}

func TestProjectConventionsMigration(t *testing.T) {
	url := boardtest.IsolatedURL(t)
	db, err := board.Open(url)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	provider, err := goose.NewProvider(goose.DialectPostgres, db, migrations.FS)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.UpTo(ctx, 26); err != nil {
		t.Fatal(err)
	}
	for _, stmt := range []string{
		`INSERT INTO project_skills (project_id, skill_id, created_at) VALUES ('proj_default', 'skl_oauth2', 'then')`,
		`INSERT INTO project_stacks (project_id, stack_id, source, created_at) VALUES ('proj_default', 'stk_django', 'manual', 'then')`,
		`INSERT INTO project_stacks (project_id, stack_id, source, created_at) VALUES ('proj_default', 'stk_go', 'detected', 'then')`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := provider.Up(ctx); err != nil {
		t.Fatal(err)
	}
	stacks, _, skills := effective(t, db)
	if strings.Join(stacks, ",") != "stk_django:project,stk_go:detected" || strings.Join(skills, ",") != "skl_oauth2:project" {
		t.Fatalf("after 00027: stacks %v skills %v", stacks, skills)
	}
	var manual int
	if err := db.QueryRow(`SELECT count(*) FROM project_stacks WHERE source = 'manual'`).Scan(&manual); err != nil || manual != 0 {
		t.Fatalf("manual project_stacks = %d %v", manual, err)
	}
	var table sql.NullString
	if err := db.QueryRow(`SELECT to_regclass('project_skills')::text`).Scan(&table); err != nil || table.Valid {
		t.Fatalf("project_skills still exists: %v %v", table, err)
	}
}
