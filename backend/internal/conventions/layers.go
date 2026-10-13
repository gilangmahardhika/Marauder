package conventions

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"orchestrator/backend/internal/identity"
)

// Origins name the layer that put an item in a set (RFC-104, RFC-105).
const (
	OriginCatalog  = "catalog"
	OriginDetected = "detected"
	OriginTribe    = "tribe"
	OriginProject  = "project"
	OriginPlanner  = "planner"
	OriginCard     = "card"
)

// TribeLayer is what a tribe sets for all its projects: stacks and skills on,
// rules off.
type TribeLayer struct {
	StackIDs []string `json:"stackIds"`
	SkillIDs []string `json:"skillIds"`
	RulesOff []string `json:"rulesOff"`
}

type OnOff struct {
	On  []string `json:"on"`
	Off []string `json:"off"`
}

// Choices are a project's own on and off rows.
type Choices struct {
	Stacks OnOff `json:"stacks"`
	Skills OnOff `json:"skills"`
	Rules  OnOff `json:"rules"`
}

type EffectiveStack struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Origin string `json:"origin"`
}

type EffectiveRule struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Scope     string `json:"scope"`
	StackName string `json:"stackName"`
	Category  string `json:"category"`
	Version   int    `json:"version"`
	Origin    string `json:"origin"`
	Hash      string `json:"-"`
	Body      string `json:"-"`
}

type EffectiveSkill struct {
	ID      string `json:"id"`
	Slug    string `json:"slug"`
	Title   string `json:"title"`
	UseWhen string `json:"useWhen"`
	Version int    `json:"version"`
	Origin  string `json:"origin"`
	Hash    string `json:"-"`
	Body    string `json:"-"`
}

// Resolved is what applies to a project after the tribe and project layers.
type Resolved struct {
	Stacks []EffectiveStack
	Rules  []EffectiveRule
	Skills []EffectiveSkill
}

// Project is the project conventions view: the tribe's layer, the project's
// choices, and what applies after both.
type Project struct {
	Tribe            TribeLayer       `json:"tribe"`
	Choices          Choices          `json:"choices"`
	DetectedStackIDs []string         `json:"detectedStackIds"`
	Stacks           []EffectiveStack `json:"stacks"`
	Skills           []EffectiveSkill `json:"skills"`
	Rules            []EffectiveRule  `json:"rules"`
	CanEdit          bool             `json:"canEdit"`
}

func emptyTribe() TribeLayer {
	return TribeLayer{StackIDs: []string{}, SkillIDs: []string{}, RulesOff: []string{}}
}

func emptyChoices() Choices {
	empty := func() OnOff { return OnOff{On: []string{}, Off: []string{}} }
	return Choices{Stacks: empty(), Skills: empty(), Rules: empty()}
}

func set(ids []string) map[string]bool {
	out := make(map[string]bool, len(ids))
	for _, id := range ids {
		out[id] = true
	}
	return out
}

// resolve applies the layers in order: detected stacks, the tribe, then the
// project. The last layer that names an item wins. A disabled item, or a rule
// of a disabled stack, never applies.
func resolve(cat Catalog, detected []string, tribe TribeLayer, c Choices) Resolved {
	out := Resolved{Stacks: []EffectiveStack{}, Rules: []EffectiveRule{}, Skills: []EffectiveSkill{}}
	isDetected, tribeStacks := set(detected), set(tribe.StackIDs)
	stackOn, stackOff := set(c.Stacks.On), set(c.Stacks.Off)
	names, enabled, applies := map[string]string{}, map[string]bool{}, map[string]bool{}
	for _, s := range cat.Stacks {
		names[s.ID] = s.Name
		if s.Disabled {
			continue
		}
		enabled[s.ID] = true
		if stackOff[s.ID] {
			continue
		}
		origin := ""
		switch {
		case stackOn[s.ID]:
			origin = OriginProject
		case tribeStacks[s.ID]:
			origin = OriginTribe
		case isDetected[s.ID]:
			origin = OriginDetected
		}
		if origin == "" {
			continue
		}
		applies[s.ID] = true
		out.Stacks = append(out.Stacks, EffectiveStack{ID: s.ID, Name: s.Name, Origin: origin})
	}
	tribeOff, ruleOn, ruleOff := set(tribe.RulesOff), set(c.Rules.On), set(c.Rules.Off)
	for _, r := range cat.Rules {
		if r.Disabled || (r.Scope == "stack" && !enabled[r.StackID]) {
			continue
		}
		on, origin := r.Scope == "global" || applies[r.StackID], OriginCatalog
		if tribeOff[r.ID] {
			on = false
		}
		if ruleOn[r.ID] {
			on, origin = true, OriginProject
		} else if ruleOff[r.ID] {
			on = false
		}
		if !on {
			continue
		}
		out.Rules = append(out.Rules, EffectiveRule{
			ID: r.ID, Title: r.Title, Scope: r.Scope, StackName: names[r.StackID], Category: r.Category,
			Version: r.Version, Origin: origin, Hash: r.ContentHash, Body: r.Body,
		})
	}
	tribeSkills, skillOn, skillOff := set(tribe.SkillIDs), set(c.Skills.On), set(c.Skills.Off)
	for _, k := range cat.Skills {
		if k.Disabled || skillOff[k.ID] {
			continue
		}
		origin := ""
		switch {
		case skillOn[k.ID]:
			origin = OriginProject
		case tribeSkills[k.ID]:
			origin = OriginTribe
		}
		if origin == "" {
			continue
		}
		out.Skills = append(out.Skills, EffectiveSkill{
			ID: k.ID, Slug: k.Slug, Title: k.Title, UseWhen: k.UseWhen, Version: k.Version,
			Origin: origin, Hash: k.ContentHash, Body: k.Body,
		})
	}
	return out
}

// Effective resolves the project's stacks, rules, and skills over its tribe
// (RFC-104).
func Effective(ctx context.Context, q Querier, projectID string) (Resolved, error) {
	p, err := LoadProject(ctx, q, projectID)
	if err != nil {
		return Resolved{}, err
	}
	return Resolved{Stacks: p.Stacks, Rules: p.Rules, Skills: p.Skills}, nil
}

// LoadProject reads the project's layers and resolves them. CanEdit is left
// for the caller.
func LoadProject(ctx context.Context, q Querier, projectID string) (Project, error) {
	p, _, err := loadProject(ctx, q, projectID)
	return p, err
}

func loadProject(ctx context.Context, q Querier, projectID string) (Project, Catalog, error) {
	tribeID, err := projectTribe(ctx, q, projectID)
	if err != nil {
		return Project{}, Catalog{}, err
	}
	tribe, err := loadTribeLayer(ctx, q, tribeID)
	if err != nil {
		return Project{}, Catalog{}, err
	}
	choices, err := loadChoices(ctx, q, projectID)
	if err != nil {
		return Project{}, Catalog{}, err
	}
	detected, err := strings1(ctx, q, `
		SELECT stack_id FROM project_stacks WHERE project_id = $1 AND source = 'detected' ORDER BY stack_id`, projectID)
	if err != nil {
		return Project{}, Catalog{}, err
	}
	cat, err := Load(ctx, q)
	if err != nil {
		return Project{}, Catalog{}, err
	}
	r := resolve(cat, detected, tribe, choices)
	return Project{
		Tribe: tribe, Choices: choices, DetectedStackIDs: detected,
		Stacks: r.Stacks, Skills: r.Skills, Rules: r.Rules,
	}, cat, nil
}

func projectTribe(ctx context.Context, q Querier, projectID string) (string, error) {
	ids, err := strings1(ctx, q, `SELECT tribe_id FROM projects WHERE id = $1`, projectID)
	if err != nil {
		return "", err
	}
	if len(ids) == 0 {
		return "", identity.ErrNotFound
	}
	return ids[0], nil
}

// LoadTribe reads a tribe's layer. The caller checks who may read it.
func LoadTribe(ctx context.Context, q Querier, tribeID string) (TribeLayer, error) {
	return loadTribeLayer(ctx, q, tribeID)
}

func loadTribeLayer(ctx context.Context, q Querier, tribeID string) (TribeLayer, error) {
	out := emptyTribe()
	rows, err := q.QueryContext(ctx, `
		SELECT kind, item_id FROM tribe_conventions WHERE tribe_id = $1 ORDER BY kind, item_id`, tribeID)
	if err != nil {
		return TribeLayer{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var kind, id string
		if err := rows.Scan(&kind, &id); err != nil {
			return TribeLayer{}, err
		}
		switch kind {
		case "stack":
			out.StackIDs = append(out.StackIDs, id)
		case "skill":
			out.SkillIDs = append(out.SkillIDs, id)
		case "rule":
			out.RulesOff = append(out.RulesOff, id)
		}
	}
	return out, rows.Err()
}

func loadChoices(ctx context.Context, q Querier, projectID string) (Choices, error) {
	out := emptyChoices()
	rows, err := q.QueryContext(ctx, `
		SELECT kind, item_id, mode FROM project_conventions WHERE project_id = $1 ORDER BY kind, item_id`, projectID)
	if err != nil {
		return Choices{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var kind, id, mode string
		if err := rows.Scan(&kind, &id, &mode); err != nil {
			return Choices{}, err
		}
		var list *OnOff
		switch kind {
		case "stack":
			list = &out.Stacks
		case "skill":
			list = &out.Skills
		case "rule":
			list = &out.Rules
		default:
			continue
		}
		if mode == "on" {
			list.On = append(list.On, id)
		} else {
			list.Off = append(list.Off, id)
		}
	}
	return out, rows.Err()
}

// SaveTribe replaces a tribe's layer. The caller checks who may save.
func SaveTribe(ctx context.Context, db *sql.DB, actor identity.User, tribeID string, in TribeLayer) (TribeLayer, error) {
	in = TribeLayer{StackIDs: trimAll(in.StackIDs), SkillIDs: trimAll(in.SkillIDs), RulesOff: trimAll(in.RulesOff)}
	fields := map[string]string{}
	for _, c := range []struct {
		field, table string
		ids          []string
	}{
		{"stackIds", "stacks", in.StackIDs},
		{"skillIds", "skills", in.SkillIDs},
		{"rulesOff", "convention_rules", in.RulesOff},
	} {
		if err := checkEnabled(ctx, db, fields, c.field, c.table, c.ids); err != nil {
			return TribeLayer{}, err
		}
	}
	if len(fields) > 0 {
		return TribeLayer{}, &identity.Validation{Fields: fields}
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return TribeLayer{}, err
	}
	defer tx.Rollback()
	at := now()
	for _, c := range []struct {
		kind, mode string
		ids        []string
	}{
		{"stack", "on", in.StackIDs},
		{"skill", "on", in.SkillIDs},
		{"rule", "off", in.RulesOff},
	} {
		if _, err := tx.ExecContext(ctx, `
			DELETE FROM tribe_conventions WHERE tribe_id = $1 AND kind = $2 AND NOT (item_id = ANY(string_to_array($3, ',')))`,
			tribeID, c.kind, strings.Join(c.ids, ",")); err != nil {
			return TribeLayer{}, err
		}
		for _, id := range c.ids {
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO tribe_conventions (tribe_id, kind, item_id, mode, added_by, created_at)
				VALUES ($1, $2, $3, $4, $5, $6) ON CONFLICT (tribe_id, kind, item_id) DO NOTHING`,
				tribeID, c.kind, id, c.mode, actor.ID, at); err != nil {
				return TribeLayer{}, err
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return TribeLayer{}, err
	}
	return loadTribeLayer(ctx, db, tribeID)
}

// SaveProject replaces the project's on and off choices. The caller checks
// who may save.
func SaveProject(ctx context.Context, db *sql.DB, actor identity.User, projectID string, in Choices) (Project, error) {
	lists := []struct {
		kind, table string
		list        *OnOff
	}{
		{"stack", "stacks", &in.Stacks},
		{"skill", "skills", &in.Skills},
		{"rule", "convention_rules", &in.Rules},
	}
	fields := map[string]string{}
	for _, l := range lists {
		l.list.On, l.list.Off = trimAll(l.list.On), trimAll(l.list.Off)
		name := l.table
		if l.kind == "rule" {
			name = "rules"
		}
		if err := checkEnabled(ctx, db, fields, name+".on", l.table, l.list.On); err != nil {
			return Project{}, err
		}
		if err := checkEnabled(ctx, db, fields, name+".off", l.table, l.list.Off); err != nil {
			return Project{}, err
		}
		on := set(l.list.On)
		for _, id := range l.list.Off {
			if on[id] {
				fields[name+".off"] = fmt.Sprintf("in both lists: %s", id)
				break
			}
		}
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
	for _, l := range lists {
		all := append(append([]string{}, l.list.On...), l.list.Off...)
		if _, err := tx.ExecContext(ctx, `
			DELETE FROM project_conventions WHERE project_id = $1 AND kind = $2 AND NOT (item_id = ANY(string_to_array($3, ',')))`,
			projectID, l.kind, strings.Join(all, ",")); err != nil {
			return Project{}, err
		}
		for _, c := range []struct {
			mode string
			ids  []string
		}{{"on", l.list.On}, {"off", l.list.Off}} {
			for _, id := range c.ids {
				if _, err := tx.ExecContext(ctx, `
					INSERT INTO project_conventions (project_id, kind, item_id, mode, added_by, created_at)
					VALUES ($1, $2, $3, $4, $5, $6)
					ON CONFLICT (project_id, kind, item_id) DO UPDATE
					SET mode = EXCLUDED.mode, added_by = EXCLUDED.added_by, created_at = EXCLUDED.created_at
					WHERE project_conventions.mode <> EXCLUDED.mode`,
					projectID, l.kind, id, c.mode, actor.ID, at); err != nil {
					return Project{}, err
				}
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return Project{}, err
	}
	return LoadProject(ctx, db, projectID)
}
