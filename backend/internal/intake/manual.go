package intake

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

var (
	ErrNotFound  = errors.New("requirement not found")
	ErrDuplicate = errors.New("a requirement with this id already exists")
	ErrNotManual = errors.New("this requirement comes from the repository; edit it there")
	ErrInUse     = errors.New("a card lists this requirement")
)

var (
	manualID   = regexp.MustCompile(`^REQ-[0-9]+$`)
	listMarker = regexp.MustCompile(`^(?:[-*+]|[0-9]{1,9}[.)])(?:[ \t]+\[[ xX]\])?(?:[ \t]+|$)`)
	lineBreak  = strings.NewReplacer("\r\n", " ", "\r", " ", "\n", " ")
)

const (
	maxStatement  = 10000
	maxAcceptance = 50
	maxLine       = 1000
)

// Invalid carries one message per rejected field: id, statement, acceptance, or userIds.
type Invalid struct {
	Fields map[string]string
}

func (v *Invalid) Error() string {
	for _, key := range []string{"id", "statement", "acceptance", "userIds", "aiReviewMode"} {
		if msg, ok := v.Fields[key]; ok {
			return msg
		}
	}
	return "validation"
}

// Draft is the text of a manual requirement. SelfReview and AIReviewMode are
// read by Add only; the caller checks CanSetSelfReview (RFC-070, RFC-072). A
// blank AIReviewMode is continue.
type Draft struct {
	Statement    string
	Acceptance   []string
	SelfReview   bool
	AIReviewMode string
}

func (d Draft) clean() (Draft, error) {
	fields := map[string]string{}
	statement := strings.ReplaceAll(strings.ReplaceAll(d.Statement, "\r\n", "\n"), "\r", "\n")
	out := Draft{Statement: strings.TrimSpace(statement), Acceptance: []string{}, SelfReview: d.SelfReview, AIReviewMode: d.AIReviewMode}
	if out.AIReviewMode == "" {
		out.AIReviewMode = ReviewContinue
	}
	if !validReviewMode(out.AIReviewMode) {
		fields["aiReviewMode"] = errReviewMode
	}
	switch {
	case out.Statement == "":
		fields["statement"] = "Enter a statement."
	case utf8.RuneCountInString(out.Statement) > maxStatement:
		fields["statement"] = fmt.Sprintf("Keep the statement to %d characters or fewer.", maxStatement)
	}
	if len(d.Acceptance) > maxAcceptance {
		fields["acceptance"] = fmt.Sprintf("Use at most %d acceptance lines.", maxAcceptance)
	}
	for _, line := range d.Acceptance {
		line = acceptanceLine(line)
		if line == "" {
			fields["acceptance"] = "Acceptance lines cannot be empty."
			continue
		}
		if utf8.RuneCountInString(line) > maxLine {
			fields["acceptance"] = fmt.Sprintf("Keep each acceptance line to %d characters or fewer.", maxLine)
		}
		out.Acceptance = append(out.Acceptance, line)
	}
	if len(fields) > 0 {
		return out, &Invalid{Fields: fields}
	}
	return out, nil
}

// Validate cleans d's statement and acceptance with the rules a save uses and
// returns *Invalid when they fail. It saves nothing.
func Validate(d Draft) (Draft, error) {
	return Draft{Statement: d.Statement, Acceptance: d.Acceptance}.clean()
}

// acceptanceLine keeps one line and drops one leading list or checkbox marker,
// since the pull request body writes each line as "- [ ] " (RFC-066).
func acceptanceLine(line string) string {
	line = strings.TrimSpace(lineBreak.Replace(line))
	return strings.TrimSpace(listMarker.ReplaceAllString(line, ""))
}

// Add stores a manual requirement on projectID. A blank id takes the next
// free REQ- number, chosen while the project's intake_state row is locked.
func Add(ctx context.Context, db *sql.DB, projectID, userID, id string, draft Draft) (string, error) {
	id = strings.TrimSpace(id)
	clean, err := draft.clean()
	if id != "" && !manualID.MatchString(id) {
		fields := map[string]string{"id": "Use REQ- followed by digits, or leave it blank."}
		var invalid *Invalid
		if errors.As(err, &invalid) {
			for key, msg := range invalid.Fields {
				fields[key] = msg
			}
		}
		return "", &Invalid{Fields: fields}
	}
	if err != nil {
		return "", err
	}
	encoded, err := json.Marshal(clean.Acceptance)
	if err != nil {
		return "", err
	}
	mentions := MentionsOf(clean.Statement, clean.Acceptance)
	mentionsJSON, err := json.Marshal(mentions)
	if err != nil {
		return "", err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	var one int
	err = tx.QueryRowContext(ctx, `SELECT 1 FROM intake_state WHERE project_id = $1 FOR UPDATE`, projectID).Scan(&one)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	if id == "" {
		var highest int64
		err := tx.QueryRowContext(ctx, `
			SELECT coalesce(max(substring(id FROM 5)::bigint), 0)
			FROM requirements WHERE project_id = $1 AND id ~ '^REQ-[0-9]+$'`, projectID).Scan(&highest)
		if err != nil {
			return "", err
		}
		id = fmt.Sprintf("REQ-%03d", highest+1)
	}
	res, err := tx.ExecContext(ctx, `
		INSERT INTO requirements (
			id, source_kind, source_path, heading, statement,
			acceptance_json, non_goals_json, content_hash, project_id, created_by, self_review, ai_review_mode, mentions_json
		) VALUES ($1, 'manual', '', $1, $2, $3, '[]', $4, $5, $6, $7, $8, $9)
		ON CONFLICT (project_id, id) DO NOTHING`,
		id, clean.Statement, string(encoded), hashRequirement(clean.Statement, clean.Acceptance, mentions), projectID, userID, clean.SelfReview,
		clean.AIReviewMode, string(mentionsJSON),
	)
	if err != nil {
		return "", err
	}
	if n, err := res.RowsAffected(); err != nil {
		return "", err
	} else if n == 0 {
		return "", ErrDuplicate
	}
	return id, tx.Commit()
}

// Update changes the text of a manual requirement. A changed hash clears
// acceptance, the same rule ingest applies to a source edit (REQ-004).
func Update(ctx context.Context, db *sql.DB, projectID, id string, draft Draft) error {
	clean, err := draft.clean()
	if err != nil {
		return err
	}
	encoded, err := json.Marshal(clean.Acceptance)
	if err != nil {
		return err
	}
	mentions := MentionsOf(clean.Statement, clean.Acceptance)
	mentionsJSON, err := json.Marshal(mentions)
	if err != nil {
		return err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := lockManual(ctx, tx, projectID, id); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `
		UPDATE requirements SET
			statement = $1,
			acceptance_json = $2,
			accepted_at = CASE WHEN content_hash = $3 THEN accepted_at ELSE NULL END,
			accepted_by = CASE WHEN content_hash = $3 THEN accepted_by ELSE NULL END,
			content_hash = $3,
			mentions_json = $6
		WHERE project_id = $4 AND id = $5`,
		clean.Statement, string(encoded), hashRequirement(clean.Statement, clean.Acceptance, mentions), projectID, id, string(mentionsJSON),
	)
	if err != nil {
		return err
	}
	return tx.Commit()
}

// Delete removes a manual requirement that no card in the project lists.
func Delete(ctx context.Context, db *sql.DB, projectID, id string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := lockManual(ctx, tx, projectID, id); err != nil {
		return err
	}
	var used bool
	err = tx.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM cards WHERE project_id = $1 AND requirement_ids_json::jsonb ? $2
		)`, projectID, id).Scan(&used)
	if err != nil {
		return err
	}
	if used {
		return ErrInUse
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM requirements WHERE project_id = $1 AND id = $2`, projectID, id); err != nil {
		return err
	}
	return tx.Commit()
}

func lockManual(ctx context.Context, tx *sql.Tx, projectID, id string) error {
	var kind string
	err := tx.QueryRowContext(ctx, `
		SELECT source_kind FROM requirements WHERE project_id = $1 AND id = $2 FOR UPDATE`,
		projectID, id).Scan(&kind)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if kind != "manual" {
		return ErrNotManual
	}
	return nil
}
