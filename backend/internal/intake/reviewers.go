package intake

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

const maxReviewers = 15

var ErrForbidden = errors.New("only the person who brought this requirement in, or a Lead Developer, can name its reviewers")

// Reviewer is a user named to review the pull requests of a requirement.
type Reviewer struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Email       string `json:"email"`
	GitHubLogin string `json:"githubLogin"`
}

// CanEditReviewers reports whether the actor may name reviewers for a row
// created by createdBy (RFC-054).
func CanEditReviewers(actorID, role, createdBy string) bool {
	return role == "lead_developer" || (createdBy != "" && createdBy == actorID)
}

// ErrSelfReviewForbidden refuses a Self review change by anyone but a Lead
// Developer or an admin (RFC-070).
var ErrSelfReviewForbidden = errors.New("only a Lead Developer or an admin can turn Self review on or off")

// CanSetSelfReview reports whether a user may set a requirement's Self review
// switch (RFC-070).
func CanSetSelfReview(role string, admin bool) bool {
	return admin || role == "lead_developer"
}

// SetSelfReview turns Self review on or off for one requirement on projectID.
// The caller checks CanSetSelfReview.
func SetSelfReview(ctx context.Context, db *sql.DB, projectID, id string, on bool) error {
	res, err := db.ExecContext(ctx, `
		UPDATE requirements SET self_review = $3 WHERE project_id = $1 AND id = $2`, projectID, id, on)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err != nil {
		return err
	} else if n == 0 {
		return ErrNotFound
	}
	return nil
}

// AI review modes of a requirement (RFC-072).
const (
	ReviewContinue = "continue"
	ReviewManual   = "manual"
)

const errReviewMode = "Choose continue or manual."

// ErrAIReviewModeForbidden refuses a manual AI review mode, or a change of
// the mode, by anyone but a Lead Developer or an admin (RFC-072).
var ErrAIReviewModeForbidden = errors.New("only a Lead Developer or an admin can change the AI review mode")

func validReviewMode(mode string) bool {
	return mode == ReviewContinue || mode == ReviewManual
}

// SetAIReviewMode sets the AI review mode of one requirement on projectID.
// The caller checks CanSetSelfReview.
func SetAIReviewMode(ctx context.Context, db *sql.DB, projectID, id, mode string) error {
	if !validReviewMode(mode) {
		return &Invalid{Fields: map[string]string{"aiReviewMode": errReviewMode}}
	}
	res, err := db.ExecContext(ctx, `
		UPDATE requirements SET ai_review_mode = $3 WHERE project_id = $1 AND id = $2`, projectID, id, mode)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err != nil {
		return err
	} else if n == 0 {
		return ErrNotFound
	}
	return nil
}

// SetReviewers replaces the reviewers of one requirement on projectID. Each
// reviewer must be an enabled member of the project's tribe with a verified
// GitHub login, and not the row's creator, who owns the pull request.
func SetReviewers(ctx context.Context, db *sql.DB, projectID, id, actorID, role string, userIDs []string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var createdBy sql.NullString
	err = tx.QueryRowContext(ctx, `
		SELECT created_by FROM requirements WHERE project_id = $1 AND id = $2 FOR UPDATE`,
		projectID, id).Scan(&createdBy)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if !CanEditReviewers(actorID, role, createdBy.String) {
		return ErrForbidden
	}
	seen := map[string]bool{}
	unique := []string{}
	for _, userID := range userIDs {
		if userID == "" || seen[userID] {
			continue
		}
		seen[userID] = true
		unique = append(unique, userID)
	}
	if len(unique) > maxReviewers {
		return &Invalid{Fields: map[string]string{"userIds": fmt.Sprintf("Name at most %d reviewers.", maxReviewers)}}
	}
	for _, userID := range unique {
		if userID == createdBy.String {
			return &Invalid{Fields: map[string]string{"userIds": "The person who brought the requirement in opens the pull request and cannot review it."}}
		}
		var ok bool
		err := tx.QueryRowContext(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM users u
				JOIN user_github g ON g.user_id = u.id AND g.verified_at IS NOT NULL AND g.login <> ''
				JOIN tribe_members m ON m.user_id = u.id
				JOIN projects p ON p.tribe_id = m.tribe_id AND p.id = $2
				WHERE u.id = $1 AND u.disabled = false)`, userID, projectID).Scan(&ok)
		if err != nil {
			return err
		}
		if !ok {
			return &Invalid{Fields: map[string]string{"userIds": "Each reviewer must be a member of this tribe with a verified GitHub login."}}
		}
	}
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM requirement_reviewers WHERE project_id = $1 AND requirement_id = $2`, projectID, id); err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	for _, userID := range unique {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO requirement_reviewers (project_id, requirement_id, user_id, added_by, created_at)
			VALUES ($1, $2, $3, $4, $5)`, projectID, id, userID, actorID, now); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// Reviewers lists the reviewers of every requirement on projectID, by requirement id.
func Reviewers(ctx context.Context, db *sql.DB, projectID string) (map[string][]Reviewer, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT rr.requirement_id, u.id, u.name, u.email,
			CASE WHEN g.verified_at IS NOT NULL THEN g.login ELSE '' END
		FROM requirement_reviewers rr
		JOIN users u ON u.id = rr.user_id
		LEFT JOIN user_github g ON g.user_id = u.id
		WHERE rr.project_id = $1
		ORDER BY rr.requirement_id, u.email`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string][]Reviewer{}
	for rows.Next() {
		var reqID string
		var r Reviewer
		if err := rows.Scan(&reqID, &r.ID, &r.Name, &r.Email, &r.GitHubLogin); err != nil {
			return nil, err
		}
		out[reqID] = append(out[reqID], r)
	}
	return out, rows.Err()
}

// ReviewerOptions are the enabled members of the project's tribe with a
// verified GitHub login.
func ReviewerOptions(ctx context.Context, db *sql.DB, projectID string) ([]Reviewer, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT u.id, u.name, u.email, g.login
		FROM projects p
		JOIN tribe_members m ON m.tribe_id = p.tribe_id
		JOIN users u ON u.id = m.user_id AND u.disabled = false
		JOIN user_github g ON g.user_id = u.id AND g.verified_at IS NOT NULL AND g.login <> ''
		WHERE p.id = $1
		ORDER BY u.email`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Reviewer{}
	for rows.Next() {
		var r Reviewer
		if err := rows.Scan(&r.ID, &r.Name, &r.Email, &r.GitHubLogin); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
