package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"orchestrator/backend/internal/ghcli"
	"orchestrator/backend/internal/identity"
)

var (
	ErrUnauthenticated = identity.ErrUnauthenticated
	ErrLocked          = identity.ErrLocked
	ErrForbidden       = identity.ErrForbidden
	ErrLastAdmin       = identity.ErrLastAdmin
	ErrNotFoundUser    = identity.ErrNotFound
	ErrAdminExists     = identity.ErrExists
	ErrLoginTaken      = identity.ErrLoginTaken
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

var ErrNoUserToken = errors.New("no stored token")

// TestUserGitHub asks GitHub which account the user's token belongs to and
// stores that login and id as verified (RFC-054). The typed login is not trusted.
func TestUserGitHub(ctx context.Context, db *sql.DB, key []byte, userID string, gh ghcli.Runner) (string, error) {
	acct, err := identity.GitHubOf(ctx, db, userID)
	if err != nil {
		return "", err
	}
	token, err := identity.Token(ctx, db, userID, key)
	if err != nil {
		return "", err
	}
	if token == "" {
		return "", ErrNoUserToken
	}
	out, err := gh.API(ctx, acct.Host, token, "user", "--jq", ".login, .id")
	if err != nil {
		return "", err
	}
	lines := strings.Fields(string(out))
	if len(lines) != 2 {
		return "", fmt.Errorf("%w: unexpected user reply", ghcli.ErrGitHub)
	}
	id, err := strconv.ParseInt(lines[1], 10, 64)
	if err != nil || id <= 0 || !loginPattern.MatchString(lines[0]) {
		return "", fmt.Errorf("%w: unexpected user reply", ghcli.ErrGitHub)
	}
	if err := identity.VerifyGitHub(ctx, db, userID, lines[0], id); err != nil {
		return "", err
	}
	return lines[0], nil
}

var loginPattern = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9-]{0,38})(?:\[bot\])?$`)

type userContextKey struct{}

func WithUser(ctx context.Context, user User) context.Context {
	return context.WithValue(ctx, userContextKey{}, user)
}

func CurrentUser(ctx context.Context) (User, bool) {
	user, ok := ctx.Value(userContextKey{}).(User)
	return user, ok
}
