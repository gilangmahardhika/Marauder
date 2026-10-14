package app

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"orchestrator/backend/internal/board"
	"orchestrator/backend/internal/identity"
	"orchestrator/backend/internal/settings"
)

const (
	// RunnerProtocol is the runner API version this binary serves (RFC-109).
	RunnerProtocol = 1
	// HeartbeatSeconds is how often a runner reports.
	HeartbeatSeconds = 10
	// RunnerOffline is how old a heartbeat may be before a runner is offline.
	RunnerOffline    = 60 * time.Second
	registrationLife = time.Hour
)

var (
	ErrRunnerAuth   = errors.New("runner unauthenticated")
	ErrProtocol     = errors.New("protocol")
	ErrRunnerAction = errors.New("action is not listed for the runner")
)

// RunnerView is a runner with its derived status and legal actions.
type RunnerView struct {
	board.Runner
	Busy           int
	Status         string
	UpdateRequired bool
	Leases         []board.Lease
	Actions        []string
}

// RunnerStatus derives a runner's status: offline when its heartbeat is empty
// or older than RunnerOffline, then draining, then busy with an open lease,
// else idle.
func RunnerStatus(r board.Runner, open int, now time.Time) string {
	at, err := time.Parse(time.RFC3339, r.HeartbeatAt)
	if err != nil || now.Sub(at) > RunnerOffline {
		return "offline"
	}
	if r.State == "draining" {
		return "draining"
	}
	if open > 0 {
		return "busy"
	}
	return "idle"
}

func runnerActions(r board.Runner) []string {
	if r.State == "draining" {
		return []string{"resume", "remove"}
	}
	return []string{"drain", "remove"}
}

func secretHash(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func randomHex(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

// MintRunnerToken makes a single-use registration token that expires in one
// hour. Only its hash is stored (RFC-106).
func MintRunnerToken(ctx context.Context, db *sql.DB, actor identity.User) (string, time.Time, error) {
	if !actor.Admin {
		return "", time.Time{}, identity.ErrForbidden
	}
	token, err := randomHex(32)
	if err != nil {
		return "", time.Time{}, err
	}
	expires := time.Now().UTC().Add(registrationLife).Truncate(time.Second)
	if err := board.InsertRegistration(ctx, db, actor.ID, secretHash(token), expires); err != nil {
		return "", time.Time{}, err
	}
	return token, expires, nil
}

type RegisterRunner struct {
	Token    string
	Name     string
	Labels   []string
	Slots    int
	Version  string
	Protocol int
}

type RegisteredRunner struct {
	ID     string
	Secret string
}

// Register trades a registration token for a runner id and secret. The
// token works once; the secret is returned only here.
func Register(ctx context.Context, db *sql.DB, in RegisterRunner) (RegisteredRunner, error) {
	name := strings.TrimSpace(in.Name)
	fields := map[string]string{}
	if name == "" || utf8.RuneCountInString(name) > 64 || strings.IndexFunc(name, func(r rune) bool { return !unicode.IsPrint(r) }) >= 0 {
		fields["name"] = "name must be 1 to 64 printable characters"
	}
	if in.Slots < 1 || in.Slots > 16 {
		fields["slots"] = "slots must be from 1 to 16"
	}
	labels := make([]string, 0, len(in.Labels))
	for _, label := range in.Labels {
		label = strings.TrimSpace(label)
		if label == "" {
			continue
		}
		if len(label) > 64 || strings.IndexFunc(label, func(r rune) bool { return !unicode.IsPrint(r) || unicode.IsSpace(r) }) >= 0 {
			fields["labels"] = "a label is at most 64 printable characters without spaces"
			break
		}
		labels = append(labels, label)
	}
	if len(labels) > 32 {
		fields["labels"] = "use at most 32 labels"
	}
	id, err := board.NewID("rnr_")
	if err != nil {
		return RegisteredRunner{}, err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return RegisteredRunner{}, err
	}
	defer tx.Rollback()
	createdBy, err := board.ConsumeRegistration(ctx, tx, secretHash(in.Token), id)
	if errors.Is(err, board.ErrNotFound) {
		return RegisteredRunner{}, ErrRunnerAuth
	}
	if err != nil {
		return RegisteredRunner{}, err
	}
	if len(fields) > 0 {
		return RegisteredRunner{}, &settings.Invalid{Fields: fields}
	}
	secret, err := randomHex(32)
	if err != nil {
		return RegisteredRunner{}, err
	}
	runner := board.Runner{ID: id, Name: name, Labels: labels, Slots: in.Slots, Version: in.Version, Protocol: in.Protocol, CreatedBy: createdBy}
	if err := board.InsertRunner(ctx, tx, runner, secretHash(secret)); err != nil {
		return RegisteredRunner{}, err
	}
	if err := tx.Commit(); err != nil {
		return RegisteredRunner{}, err
	}
	return RegisteredRunner{ID: id, Secret: secret}, nil
}

// RunnerBySecret authenticates a runner secret. A removed runner, or an
// unknown secret, is ErrRunnerAuth.
func RunnerBySecret(ctx context.Context, db *sql.DB, secret string) (board.Runner, error) {
	if secret == "" {
		return board.Runner{}, ErrRunnerAuth
	}
	r, err := board.RunnerBySecret(ctx, db, secretHash(secret))
	if errors.Is(err, board.ErrNotFound) {
		return board.Runner{}, ErrRunnerAuth
	}
	return r, err
}

type runnerKey struct{}

func WithRunner(ctx context.Context, r board.Runner) context.Context {
	return context.WithValue(ctx, runnerKey{}, r)
}

func CurrentRunner(ctx context.Context) (board.Runner, bool) {
	r, ok := ctx.Value(runnerKey{}).(board.Runner)
	return r, ok
}

// RunnerHeartbeat records a heartbeat and returns the runner's state and the
// leases it still holds open.
func RunnerHeartbeat(ctx context.Context, db *sql.DB, r board.Runner, version string, protocol int) (string, []string, error) {
	if err := board.Heartbeat(ctx, db, r.ID, version, protocol); err != nil {
		return "", nil, err
	}
	if err := board.ExtendLeases(ctx, db, r.ID); err != nil {
		return "", nil, err
	}
	leases, err := board.OpenLeases(ctx, db, r.ID)
	if err != nil {
		return "", nil, err
	}
	ids := make([]string, 0, len(leases))
	for _, l := range leases {
		ids = append(ids, l.ID)
	}
	current, err := board.GetRunner(ctx, db, r.ID)
	if err != nil {
		return "", nil, err
	}
	return current.State, ids, nil
}

// ClaimForRunner leases the next queued implement job to the runner when
// jobs.executor is runners. Nothing to claim, or the local executor, is
// board.ErrNotFound; a runner on another protocol is ErrProtocol.
func ClaimForRunner(ctx context.Context, db *sql.DB, data string, r board.Runner) (board.Lease, error) {
	if r.Protocol != RunnerProtocol {
		return board.Lease{}, ErrProtocol
	}
	cfg, _, err := settings.Load(data)
	if err != nil {
		return board.Lease{}, err
	}
	if cfg.Executor() != settings.ExecutorRunners {
		return board.Lease{}, board.ErrNotFound
	}
	return board.ClaimRun(ctx, db, r.ID, func(projectID string) int {
		return ImplementationCap(data, projectID)
	}, RunnerOffline)
}

// ImplementationCap is the project's scheduler.implementationCap, 2 when
// unset or unreadable.
func ImplementationCap(data, projectID string) int {
	if data != "" {
		if cfg, _, err := settings.LoadProject(data, projectID); err == nil && cfg.Scheduler.ImplementationCap > 0 {
			return cfg.Scheduler.ImplementationCap
		}
	}
	return 2
}

// ListRunners lists runners with derived status for an admin.
func ListRunners(ctx context.Context, db *sql.DB, actor identity.User) ([]RunnerView, error) {
	if !actor.Admin {
		return nil, identity.ErrForbidden
	}
	runners, err := board.ListRunners(ctx, db)
	if err != nil {
		return nil, err
	}
	leases, err := board.OpenLeases(ctx, db, "")
	if err != nil {
		return nil, err
	}
	byRunner := map[string][]board.Lease{}
	for _, l := range leases {
		byRunner[l.RunnerID] = append(byRunner[l.RunnerID], l)
	}
	now := time.Now()
	out := make([]RunnerView, 0, len(runners))
	for _, r := range runners {
		out = append(out, viewRunner(r, byRunner[r.ID], now))
	}
	return out, nil
}

func viewRunner(r board.Runner, leases []board.Lease, now time.Time) RunnerView {
	if leases == nil {
		leases = []board.Lease{}
	}
	return RunnerView{
		Runner:         r,
		Busy:           len(leases),
		Status:         RunnerStatus(r, len(leases), now),
		UpdateRequired: r.Protocol != RunnerProtocol,
		Leases:         leases,
		Actions:        runnerActions(r),
	}
}

// RunnerAction runs drain, resume, or remove on a runner. An action the
// runner does not list is ErrRunnerAction. Remove ends the runner's open
// leases as the reaper does and returns an empty view.
func RunnerAction(ctx context.Context, db *sql.DB, actor identity.User, id, action string) (RunnerView, error) {
	if !actor.Admin {
		return RunnerView{}, identity.ErrForbidden
	}
	r, err := board.GetRunner(ctx, db, id)
	if err != nil {
		return RunnerView{}, err
	}
	listed := false
	for _, name := range runnerActions(r) {
		listed = listed || name == action
	}
	if !listed {
		return RunnerView{}, ErrRunnerAction
	}
	switch action {
	case "drain":
		err = board.SetRunnerState(ctx, db, id, "active", "draining")
	case "resume":
		err = board.SetRunnerState(ctx, db, id, "draining", "active")
	case "remove":
		if err = board.SetRunnerState(ctx, db, id, r.State, "removed"); err == nil {
			_, err = board.ReapLeases(ctx, db, board.ReapFilter{RunnerID: id}, "removed")
		}
		return RunnerView{}, err
	}
	if errors.Is(err, board.ErrConflict) {
		return RunnerView{}, ErrRunnerAction
	}
	if err != nil {
		return RunnerView{}, err
	}
	r, err = board.GetRunner(ctx, db, id)
	if err != nil {
		return RunnerView{}, err
	}
	leases, err := board.OpenLeases(ctx, db, id)
	if err != nil {
		return RunnerView{}, err
	}
	return viewRunner(r, leases, time.Now()), nil
}
