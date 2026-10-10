package tenancy

import (
	"context"
	"database/sql"
	"time"

	"orchestrator/backend/internal/board"
	"orchestrator/backend/internal/identity"
)

const DefaultProject = "proj_default"

type Tribe struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Project is one repository in the company organization. RepoPath is the
// server-side clone and is not sent to clients.
type Project struct {
	ID         string `json:"id"`
	TribeID    string `json:"tribeId"`
	Name       string `json:"name"`
	Repo       string `json:"repo"`
	BaseBranch string `json:"baseBranch"`
	RepoPath   string `json:"-"`
}

type Member struct {
	ID       string `json:"id"`
	Email    string `json:"email"`
	Role     string `json:"role"`
	Admin    bool   `json:"admin"`
	Disabled bool   `json:"disabled"`
}

func CreateTribe(ctx context.Context, db *sql.DB, actor identity.User, name string) (Tribe, error) {
	if !actor.Admin {
		return Tribe{}, identity.ErrForbidden
	}
	if name == "" {
		return Tribe{}, &identity.Validation{Fields: map[string]string{"name": "name is required"}}
	}
	id, err := board.NewID("tribe_")
	if err != nil {
		return Tribe{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.ExecContext(ctx, `INSERT INTO tribes (id, name, created_at) VALUES ($1, $2, $3)`, id, name, now); err != nil {
		return Tribe{}, err
	}
	return Tribe{ID: id, Name: name}, nil
}

func ListTribes(ctx context.Context, db *sql.DB, actor identity.User) ([]Tribe, error) {
	var query string
	var args []any
	if actor.Admin {
		query = `SELECT id, name FROM tribes ORDER BY name`
	} else {
		query = `
			SELECT t.id, t.name FROM tribes t
			JOIN tribe_members m ON m.tribe_id = t.id
			WHERE m.user_id = $1 ORDER BY t.name`
		args = []any{actor.ID}
	}
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Tribe{}
	for rows.Next() {
		var tribe Tribe
		if err := rows.Scan(&tribe.ID, &tribe.Name); err != nil {
			return nil, err
		}
		out = append(out, tribe)
	}
	return out, rows.Err()
}

func ListMembers(ctx context.Context, db *sql.DB, actor identity.User, tribeID string) ([]Member, error) {
	if !actor.Admin {
		return nil, identity.ErrForbidden
	}
	var n int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM tribes WHERE id = $1`, tribeID).Scan(&n); err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, identity.ErrNotFound
	}
	rows, err := db.QueryContext(ctx, `
		SELECT u.id, u.email, u.role, u.admin, u.disabled
		FROM tribe_members m
		JOIN users u ON u.id = m.user_id
		WHERE m.tribe_id = $1
		ORDER BY u.email`, tribeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Member{}
	for rows.Next() {
		var member Member
		if err := rows.Scan(&member.ID, &member.Email, &member.Role, &member.Admin, &member.Disabled); err != nil {
			return nil, err
		}
		out = append(out, member)
	}
	return out, rows.Err()
}

func AddMember(ctx context.Context, db *sql.DB, actor identity.User, tribeID, userID string) error {
	if !actor.Admin {
		return identity.ErrForbidden
	}
	var n int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM tribes WHERE id = $1`, tribeID).Scan(&n); err != nil {
		return err
	}
	if n == 0 {
		return identity.ErrNotFound
	}
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM users WHERE id = $1`, userID).Scan(&n); err != nil {
		return err
	}
	if n == 0 {
		return identity.ErrNotFound
	}
	_, err := db.ExecContext(ctx, `
		INSERT INTO tribe_members (tribe_id, user_id) VALUES ($1, $2)
		ON CONFLICT DO NOTHING`, tribeID, userID)
	return err
}

func RemoveMember(ctx context.Context, db *sql.DB, actor identity.User, tribeID, userID string) error {
	if !actor.Admin {
		return identity.ErrForbidden
	}
	res, err := db.ExecContext(ctx, `DELETE FROM tribe_members WHERE tribe_id = $1 AND user_id = $2`, tribeID, userID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return identity.ErrNotFound
	}
	return nil
}
