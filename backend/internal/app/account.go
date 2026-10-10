package app

import (
	"context"
	"database/sql"

	"orchestrator/backend/internal/identity"
)

var (
	ErrUnauthenticated = identity.ErrUnauthenticated
	ErrLocked          = identity.ErrLocked
	ErrForbidden       = identity.ErrForbidden
	ErrLastAdmin       = identity.ErrLastAdmin
	ErrNotFoundUser    = identity.ErrNotFound
	ErrAdminExists     = identity.ErrExists
)

type User = identity.User
type Session = identity.Session
type Validation = identity.Validation

func Bootstrap(ctx context.Context, db *sql.DB, email, password string) (User, error) {
	return identity.Bootstrap(ctx, db, email, password)
}

func CreateUser(ctx context.Context, db *sql.DB, actor User, email, password, role string, admin bool) (User, error) {
	return identity.CreateUser(ctx, db, actor, email, password, role, admin)
}

func UpdateUser(ctx context.Context, db *sql.DB, actor User, id string, role *string, admin, disabled *bool, password string) (User, error) {
	return identity.UpdateUser(ctx, db, actor, id, role, admin, disabled, password)
}

func ChangePassword(ctx context.Context, db *sql.DB, user User, current, next string) error {
	return identity.ChangePassword(ctx, db, user, current, next)
}

func Login(ctx context.Context, db *sql.DB, email, password string) (Session, error) {
	return identity.Login(ctx, db, email, password)
}

func Logout(ctx context.Context, db *sql.DB, bearer string) error {
	return identity.Logout(ctx, db, bearer)
}

func UserByBearer(ctx context.Context, db *sql.DB, bearer string) (User, error) {
	return identity.UserByBearer(ctx, db, bearer)
}

func ListUsers(ctx context.Context, db *sql.DB, actor User) ([]User, error) {
	return identity.ListUsers(ctx, db, actor)
}

func SaveUserGitHub(ctx context.Context, db *sql.DB, userID, host, login string, token *string, clear bool, key []byte) error {
	return identity.SaveGitHub(ctx, db, userID, host, login, token, clear, key)
}

func OpenUserToken(key []byte, ciphertext string) (string, error) {
	return identity.OpenToken(key, ciphertext)
}

func UserGitHubToken(ctx context.Context, db *sql.DB, userID string, key []byte) (string, error) {
	return identity.Token(ctx, db, userID, key)
}

type userContextKey struct{}

func WithUser(ctx context.Context, user User) context.Context {
	return context.WithValue(ctx, userContextKey{}, user)
}

func CurrentUser(ctx context.Context) (User, bool) {
	user, ok := ctx.Value(userContextKey{}).(User)
	return user, ok
}
