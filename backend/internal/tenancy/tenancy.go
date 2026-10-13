package tenancy

import (
	"context"
	"database/sql"
	"errors"
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
	ID         string `json:"id"`
	Name       string `json:"name"`
	Email      string `json:"email"`
	Role       string `json:"role"`
	Admin      bool   `json:"admin"`
	Disabled   bool   `json:"disabled"`
	TribeAdmin bool   `json:"tribeAdmin"`
}

// Candidate is a user who could be added to a tribe.
type Candidate struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
	Role  string `json:"role"`
}

// IsTribeAdmin reports whether userID holds the tribe admin flag on tribeID.
func IsTribeAdmin(ctx context.Context, db *sql.DB, userID, tribeID string) (bool, error) {
	var admin bool
	err := db.QueryRowContext(ctx, `
		SELECT admin FROM tribe_members WHERE tribe_id = $1 AND user_id = $2`, tribeID, userID).Scan(&admin)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return admin, err
}

// CanManageTribe is true for a company admin and for a tribe admin of tribeID (RFC-057).
func CanManageTribe(ctx context.Context, db *sql.DB, actor identity.User, tribeID string) (bool, error) {
	if actor.Admin {
		return true, nil
	}
	return IsTribeAdmin(ctx, db, actor.ID, tribeID)
}

// TribeAdminOf lists the tribes where userID holds the tribe admin flag.
func TribeAdminOf(ctx context.Context, db *sql.DB, userID string) ([]string, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT tribe_id FROM tribe_members WHERE user_id = $1 AND admin ORDER BY tribe_id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// manageTribe returns ErrForbidden unless the actor may run the tribe, and
// ErrNotFound when the tribe does not exist.
// ManageTribe returns identity.ErrForbidden unless actor may manage tribeID,
// then identity.ErrNotFound when the tribe does not exist (RFC-057).
func ManageTribe(ctx context.Context, db *sql.DB, actor identity.User, tribeID string) error {
	ok, err := CanManageTribe(ctx, db, actor, tribeID)
	if err != nil {
		return err
	}
	if !ok {
		return identity.ErrForbidden
	}
	var n int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM tribes WHERE id = $1`, tribeID).Scan(&n); err != nil {
		return err
	}
	if n == 0 {
		return identity.ErrNotFound
	}
	return nil
}

// TribeVisible lets a company admin read any tribe, and a member read their
// own. Anyone else gets identity.ErrForbidden; an admin asking for an unknown
// tribe gets identity.ErrNotFound.
func TribeVisible(ctx context.Context, db *sql.DB, actor identity.User, tribeID string) error {
	var n int
	if actor.Admin {
		if err := db.QueryRowContext(ctx, `SELECT count(*) FROM tribes WHERE id = $1`, tribeID).Scan(&n); err != nil {
			return err
		}
		if n == 0 {
			return identity.ErrNotFound
		}
		return nil
	}
	if err := db.QueryRowContext(ctx, `
		SELECT count(*) FROM tribe_members WHERE tribe_id = $1 AND user_id = $2`, tribeID, actor.ID).Scan(&n); err != nil {
		return err
	}
	if n == 0 {
		return identity.ErrForbidden
	}
	return nil
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
	if err := ManageTribe(ctx, db, actor, tribeID); err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx, `
		SELECT u.id, u.name, u.email, u.role, u.admin, u.disabled, m.admin
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
		if err := rows.Scan(&member.ID, &member.Name, &member.Email, &member.Role, &member.Admin, &member.Disabled, &member.TribeAdmin); err != nil {
			return nil, err
		}
		out = append(out, member)
	}
	return out, rows.Err()
}

func AddMember(ctx context.Context, db *sql.DB, actor identity.User, tribeID, userID string) error {
	if err := ManageTribe(ctx, db, actor, tribeID); err != nil {
		return err
	}
	var n int
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
	if err := ManageTribe(ctx, db, actor, tribeID); err != nil {
		return err
	}
	if !actor.Admin && userID == actor.ID {
		return &identity.Validation{Fields: map[string]string{"userId": "a tribe admin cannot remove themselves"}}
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

// SetTribeAdmin sets or clears the tribe admin flag on one member. A tribe
// admin cannot clear their own flag; a company admin can.
func SetTribeAdmin(ctx context.Context, db *sql.DB, actor identity.User, tribeID, userID string, on bool) error {
	if err := ManageTribe(ctx, db, actor, tribeID); err != nil {
		return err
	}
	if !actor.Admin && userID == actor.ID && !on {
		return &identity.Validation{Fields: map[string]string{"tribeAdmin": "a tribe admin cannot clear their own flag"}}
	}
	res, err := db.ExecContext(ctx, `
		UPDATE tribe_members SET admin = $3 WHERE tribe_id = $1 AND user_id = $2`, tribeID, userID, on)
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

// Candidates lists enabled users who are not members of the tribe.
func Candidates(ctx context.Context, db *sql.DB, actor identity.User, tribeID string) ([]Candidate, error) {
	if err := ManageTribe(ctx, db, actor, tribeID); err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx, `
		SELECT u.id, u.name, u.email, u.role FROM users u
		WHERE NOT u.disabled
		  AND NOT EXISTS (SELECT 1 FROM tribe_members m WHERE m.tribe_id = $1 AND m.user_id = u.id)
		ORDER BY u.email`, tribeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Candidate{}
	for rows.Next() {
		var c Candidate
		if err := rows.Scan(&c.ID, &c.Name, &c.Email, &c.Role); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
