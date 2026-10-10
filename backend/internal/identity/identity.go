package identity

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"golang.org/x/crypto/argon2"

	"orchestrator/backend/internal/board"
)

const (
	minPassword = 12
	sessionLife = 12 * time.Hour
)

var (
	ErrUnauthenticated = errors.New("unauthenticated")
	ErrLocked          = errors.New("locked")
	ErrForbidden       = errors.New("forbidden")
	ErrValidation      = errors.New("validation")
	ErrLastAdmin       = errors.New("last admin")
	ErrNotFound        = errors.New("not found")
	ErrExists          = errors.New("admin already exists")
)

type Validation struct {
	Fields map[string]string
}

func (v *Validation) Error() string {
	for _, msg := range v.Fields {
		return msg
	}
	return "validation"
}

func (v *Validation) Unwrap() error { return ErrValidation }

type User struct {
	ID             string
	Email          string
	Role           string
	Admin          bool
	Disabled       bool
	GitHubHost     string
	GitHubLogin    string
	GitHubTokenSet bool
}

type Session struct {
	Bearer    string
	ExpiresAt time.Time
	User      User
}

var roles = map[string]bool{
	"lead_developer":   true,
	"senior_developer": true,
	"developer":        true,
	"qa_lead":          true,
	"qa":               true,
	"product_owner":    true,
}

func Bootstrap(ctx context.Context, db *sql.DB, email, password string) (User, error) {
	var n int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM users`).Scan(&n); err != nil {
		return User{}, err
	}
	if n > 0 {
		return User{}, ErrExists
	}
	return Insert(ctx, db, email, password, "product_owner", true)
}

func CreateUser(ctx context.Context, db *sql.DB, actor User, email, password, role string, admin bool) (User, error) {
	if !actor.Admin {
		return User{}, ErrForbidden
	}
	return Insert(ctx, db, email, password, role, admin)
}

func Insert(ctx context.Context, db *sql.DB, email, password, role string, admin bool) (User, error) {
	return insertUser(ctx, db, email, password, role, admin)
}

func UpdateUser(ctx context.Context, db *sql.DB, actor User, id string, role *string, admin *bool, disabled *bool, password string) (User, error) {
	if !actor.Admin {
		return User{}, ErrForbidden
	}
	current, err := userByID(ctx, db, id)
	if err != nil {
		return User{}, err
	}
	nextRole := current.Role
	if role != nil {
		if !roles[*role] {
			return User{}, &Validation{Fields: map[string]string{"role": "role is not a job role"}}
		}
		nextRole = *role
	}
	nextAdmin := current.Admin
	if admin != nil {
		nextAdmin = *admin
	}
	nextDisabled := current.Disabled
	if disabled != nil {
		nextDisabled = *disabled
	}
	if current.Admin && !current.Disabled && (!nextAdmin || nextDisabled) {
		if err := guardLastAdmin(ctx, db, current.ID); err != nil {
			return User{}, err
		}
	}
	if password != "" {
		hash, err := hashPassword(password)
		if err != nil {
			return User{}, err
		}
		if _, err := db.ExecContext(ctx, `UPDATE users SET password_hash = $1 WHERE id = $2`, hash, id); err != nil {
			return User{}, err
		}
	}
	if _, err := db.ExecContext(ctx, `UPDATE users SET role = $1, admin = $2, disabled = $3 WHERE id = $4`, nextRole, nextAdmin, nextDisabled, id); err != nil {
		return User{}, err
	}
	return userByID(ctx, db, id)
}

func ChangePassword(ctx context.Context, db *sql.DB, user User, current, next string) error {
	var hash string
	if err := db.QueryRowContext(ctx, `SELECT password_hash FROM users WHERE id = $1`, user.ID).Scan(&hash); err != nil {
		return err
	}
	ok, err := verifyPassword(current, hash)
	if err != nil {
		return err
	}
	if !ok {
		return ErrUnauthenticated
	}
	nextHash, err := hashPassword(next)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, `UPDATE users SET password_hash = $1 WHERE id = $2`, nextHash, user.ID)
	return err
}

func Login(ctx context.Context, db *sql.DB, email, password string) (Session, error) {
	email = normalizeEmail(email)
	if locked, err := isLocked(ctx, db, email); err != nil {
		return Session{}, err
	} else if locked {
		return Session{}, ErrLocked
	}
	var user User
	var hash string
	err := db.QueryRowContext(ctx, `
		SELECT id, email, password_hash, role, admin, disabled
		FROM users WHERE email = $1`, email).Scan(&user.ID, &user.Email, &hash, &user.Role, &user.Admin, &user.Disabled)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && user.Disabled) {
		if err := noteFailure(ctx, db, email); err != nil {
			return Session{}, err
		}
		return Session{}, ErrUnauthenticated
	}
	if err != nil {
		return Session{}, err
	}
	ok, err := verifyPassword(password, hash)
	if err != nil {
		return Session{}, err
	}
	if !ok {
		if err := noteFailure(ctx, db, email); err != nil {
			return Session{}, err
		}
		return Session{}, ErrUnauthenticated
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO login_lockouts (email, fails, paused_until) VALUES ($1, 0, NULL)
		ON CONFLICT (email) DO UPDATE SET fails = 0, paused_until = NULL`, email); err != nil {
		return Session{}, err
	}
	raw, exp, err := issue(ctx, db, user.ID)
	if err != nil {
		return Session{}, err
	}
	full, err := userByID(ctx, db, user.ID)
	if err != nil {
		return Session{}, err
	}
	return Session{Bearer: raw, ExpiresAt: exp, User: full}, nil
}

func Logout(ctx context.Context, db *sql.DB, bearer string) error {
	sum := sha256.Sum256([]byte(bearer))
	_, err := db.ExecContext(ctx, `DELETE FROM sessions WHERE bearer_hash = $1`, hex.EncodeToString(sum[:]))
	return err
}

func UserByBearer(ctx context.Context, db *sql.DB, bearer string) (User, error) {
	sum := sha256.Sum256([]byte(bearer))
	var id, exp string
	err := db.QueryRowContext(ctx, `
		SELECT user_id, expires_at FROM sessions WHERE bearer_hash = $1`, hex.EncodeToString(sum[:])).Scan(&id, &exp)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrUnauthenticated
	}
	if err != nil {
		return User{}, err
	}
	until, err := time.Parse(time.RFC3339, exp)
	if err != nil || !time.Now().Before(until) {
		return User{}, ErrUnauthenticated
	}
	user, err := userByID(ctx, db, id)
	if err != nil {
		return User{}, err
	}
	if user.Disabled {
		return User{}, ErrUnauthenticated
	}
	return user, nil
}

func GitHub(ctx context.Context, db *sql.DB, userID string) (host, login string, tokenSet bool, err error) {
	err = db.QueryRowContext(ctx, `
		SELECT host, login, token_ciphertext <> '' FROM user_github WHERE user_id = $1`, userID).Scan(&host, &login, &tokenSet)
	if errors.Is(err, sql.ErrNoRows) {
		return "github.com", "", false, nil
	}
	return host, login, tokenSet, err
}

func SaveGitHub(ctx context.Context, db *sql.DB, userID, host, login string, token *string, clear bool, key []byte) error {
	if strings.TrimSpace(host) == "" || strings.Contains(host, "/") || strings.Contains(host, ":") {
		return &Validation{Fields: map[string]string{"host": "host must be a hostname"}}
	}
	currentHost, currentLogin, _, err := GitHub(ctx, db, userID)
	if err != nil {
		return err
	}
	if host == "" {
		host = currentHost
	}
	if login == "" && token == nil && !clear {
		login = currentLogin
	}
	var cipherText string
	if err := db.QueryRowContext(ctx, `SELECT token_ciphertext FROM user_github WHERE user_id = $1`, userID).Scan(&cipherText); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if clear {
		cipherText = ""
	} else if token != nil {
		if len(*token) < 8 {
			return &Validation{Fields: map[string]string{"token": "token is too short"}}
		}
		sealed, err := Seal(key, []byte(*token))
		if err != nil {
			return err
		}
		cipherText = sealed
	}
	_, err = db.ExecContext(ctx, `
		INSERT INTO user_github (user_id, host, login, token_ciphertext) VALUES ($1, $2, $3, $4)
		ON CONFLICT (user_id) DO UPDATE SET host = EXCLUDED.host, login = EXCLUDED.login, token_ciphertext = EXCLUDED.token_ciphertext`,
		userID, host, login, cipherText)
	return err
}

func Token(ctx context.Context, db *sql.DB, userID string, key []byte) (string, error) {
	var stored string
	err := db.QueryRowContext(ctx, `SELECT token_ciphertext FROM user_github WHERE user_id = $1`, userID).Scan(&stored)
	if errors.Is(err, sql.ErrNoRows) || stored == "" {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return OpenToken(key, stored)
}

func OpenToken(key []byte, ciphertext string) (string, error) {
	if ciphertext == "" {
		return "", nil
	}
	raw, err := hex.DecodeString(ciphertext)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(raw) < gcm.NonceSize() {
		return "", errors.New("ciphertext is short")
	}
	plain, err := gcm.Open(nil, raw[:gcm.NonceSize()], raw[gcm.NonceSize():], nil)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

func Seal(key, plain []byte) (string, error) {
	if len(key) != 32 {
		return "", errors.New("tokenKey must be 32 bytes")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	out := gcm.Seal(nonce, nonce, plain, nil)
	return hex.EncodeToString(out), nil
}

func ListUsers(ctx context.Context, db *sql.DB, actor User) ([]User, error) {
	if !actor.Admin {
		return nil, ErrForbidden
	}
	rows, err := db.QueryContext(ctx, `SELECT id FROM users ORDER BY email`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []User
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		user, err := userByID(ctx, db, id)
		if err != nil {
			return nil, err
		}
		out = append(out, user)
	}
	return out, rows.Err()
}

func insertUser(ctx context.Context, db *sql.DB, email, password, role string, admin bool) (User, error) {
	email = normalizeEmail(email)
	if !strings.Contains(email, "@") || strings.ContainsAny(email, " \t") {
		return User{}, &Validation{Fields: map[string]string{"email": "email is invalid"}}
	}
	if !roles[role] {
		return User{}, &Validation{Fields: map[string]string{"role": "role is not a job role"}}
	}
	hash, err := hashPassword(password)
	if err != nil {
		return User{}, err
	}
	id, err := board.NewID("user_")
	if err != nil {
		return User{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err = db.ExecContext(ctx, `
		INSERT INTO users (id, email, password_hash, role, admin, disabled, created_at)
		VALUES ($1, $2, $3, $4, $5, false, $6)`, id, email, hash, role, admin, now)
	if err != nil {
		if strings.Contains(err.Error(), "users_email") || strings.Contains(err.Error(), "duplicate key") {
			return User{}, &Validation{Fields: map[string]string{"email": "email is already used"}}
		}
		return User{}, err
	}
	return userByID(ctx, db, id)
}

func userByID(ctx context.Context, db *sql.DB, id string) (User, error) {
	var user User
	err := db.QueryRowContext(ctx, `
		SELECT id, email, role, admin, disabled FROM users WHERE id = $1`, id).
		Scan(&user.ID, &user.Email, &user.Role, &user.Admin, &user.Disabled)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrNotFound
	}
	if err != nil {
		return User{}, err
	}
	host, login, set, err := GitHub(ctx, db, id)
	if err != nil {
		return User{}, err
	}
	user.GitHubHost = host
	user.GitHubLogin = login
	user.GitHubTokenSet = set
	return user, nil
}

func guardLastAdmin(ctx context.Context, db *sql.DB, id string) error {
	var n int
	if err := db.QueryRowContext(ctx, `
		SELECT count(*) FROM users WHERE admin = true AND disabled = false AND id <> $1`, id).Scan(&n); err != nil {
		return err
	}
	if n == 0 {
		return ErrLastAdmin
	}
	return nil
}

func issue(ctx context.Context, db *sql.DB, userID string) (string, time.Time, error) {
	var buf [32]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", time.Time{}, err
	}
	raw := hex.EncodeToString(buf[:])
	sum := sha256.Sum256([]byte(raw))
	id, err := board.NewID("ses_")
	if err != nil {
		return "", time.Time{}, err
	}
	exp := time.Now().UTC().Add(sessionLife).Truncate(time.Second)
	_, err = db.ExecContext(ctx, `
		INSERT INTO sessions (id, user_id, bearer_hash, expires_at, created_at)
		VALUES ($1, $2, $3, $4, $5)`,
		id, userID, hex.EncodeToString(sum[:]), exp.Format(time.RFC3339), time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return "", time.Time{}, err
	}
	return raw, exp, nil
}

func isLocked(ctx context.Context, db *sql.DB, email string) (bool, error) {
	var paused sql.NullString
	err := db.QueryRowContext(ctx, `SELECT paused_until FROM login_lockouts WHERE email = $1`, email).Scan(&paused)
	if errors.Is(err, sql.ErrNoRows) || !paused.Valid || paused.String == "" {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	until, err := time.Parse(time.RFC3339, paused.String)
	if err != nil {
		return false, nil
	}
	return time.Now().Before(until), nil
}

func noteFailure(ctx context.Context, db *sql.DB, email string) error {
	var fails int
	err := db.QueryRowContext(ctx, `SELECT fails FROM login_lockouts WHERE email = $1`, email).Scan(&fails)
	if errors.Is(err, sql.ErrNoRows) {
		fails = 0
		err = nil
	}
	if err != nil {
		return err
	}
	fails++
	var paused any
	if fails >= 5 {
		paused = time.Now().UTC().Add(time.Minute).Format(time.RFC3339)
		fails = 0
	}
	_, err = db.ExecContext(ctx, `
		INSERT INTO login_lockouts (email, fails, paused_until) VALUES ($1, $2, $3)
		ON CONFLICT (email) DO UPDATE SET fails = EXCLUDED.fails, paused_until = EXCLUDED.paused_until`,
		email, fails, paused)
	return err
}

func hashPassword(password string) (string, error) {
	if len(password) < minPassword {
		return "", &Validation{Fields: map[string]string{"password": "password must be at least 12 characters"}}
	}
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	sum := argon2.IDKey([]byte(password), salt, 1, 64*1024, 4, 32)
	return "argon2id$" + hex.EncodeToString(salt) + "$" + hex.EncodeToString(sum), nil
}

func verifyPassword(password, stored string) (bool, error) {
	parts := strings.Split(stored, "$")
	if len(parts) != 3 || parts[0] != "argon2id" {
		return false, errors.New("password hash is invalid")
	}
	salt, err := hex.DecodeString(parts[1])
	if err != nil {
		return false, err
	}
	want, err := hex.DecodeString(parts[2])
	if err != nil {
		return false, err
	}
	got := argon2.IDKey([]byte(password), salt, 1, 64*1024, 4, 32)
	return subtle.ConstantTimeCompare(got, want) == 1, nil
}

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}
