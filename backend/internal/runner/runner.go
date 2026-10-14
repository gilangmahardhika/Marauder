package runner

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"time"

	"orchestrator/backend/internal/runnerapi"
)

// ErrRemoved means the control plane no longer accepts this runner's secret.
var ErrRemoved = errors.New("runner secret refused; register the runner again")

// Runner heartbeats and runs up to Slots leased jobs at once.
type Runner struct {
	Client  *Client
	Name    string
	Data    string
	Slots   int
	Version string
	Tools   Tools
	Logf    func(string, ...any)
	// Heartbeat and Retry default to 10 and 5 seconds, ClaimWait to 20.
	Heartbeat time.Duration
	Retry     time.Duration
	ClaimWait int

	mu       sync.Mutex
	state    string
	running  map[string]context.CancelFunc
	orphans  map[string]bool
	outdated bool
}

func (r *Runner) logf(format string, args ...any) {
	if r.Logf != nil {
		r.Logf(format, args...)
	}
}

func (r *Runner) heartbeatEvery() time.Duration {
	if r.Heartbeat > 0 {
		return r.Heartbeat
	}
	return 10 * time.Second
}

func (r *Runner) retry() time.Duration {
	if r.Retry > 0 {
		return r.Retry
	}
	return 5 * time.Second
}

// Run heartbeats and claims until ctx is cancelled or the control plane
// refuses the secret. Jobs still running when ctx ends give their lease back.
func (r *Runner) Run(ctx context.Context) error {
	r.mu.Lock()
	r.state = "active"
	r.running = map[string]context.CancelFunc{}
	r.orphans = map[string]bool{}
	r.mu.Unlock()
	if err := r.beat(ctx); err != nil {
		return err
	}
	ctx, stop := context.WithCancelCause(ctx)
	defer stop(nil)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(r.heartbeatEvery())
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
			if err := r.beat(ctx); errors.Is(err, ErrRemoved) {
				stop(err)
				return
			}
		}
	}()
	slots := max(r.Slots, 1)
	for range slots {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r.claimLoop(ctx)
		}()
	}
	wg.Wait()
	if cause := context.Cause(ctx); errors.Is(cause, ErrRemoved) {
		return cause
	}
	return nil
}

// beat sends one heartbeat. A lease the control plane holds open for this
// runner that no job runs, seen on two heartbeats in a row, is given back.
// A lease no longer open stops its job.
func (r *Runner) beat(ctx context.Context) error {
	reply, err := r.Client.Heartbeat(ctx, runnerapi.Heartbeat{Version: r.Version, Protocol: runnerapi.Protocol})
	if StatusOf(err) == http.StatusUnauthorized {
		return ErrRemoved
	}
	if err != nil {
		if ctx.Err() == nil {
			r.logf("runner: heartbeat: %v", err)
		}
		return nil
	}
	open := map[string]bool{}
	for _, id := range reply.Leases {
		open[id] = true
	}
	var release []string
	r.mu.Lock()
	r.state = reply.State
	r.outdated = reply.Protocol != runnerapi.Protocol
	for id, cancel := range r.running {
		if !open[id] {
			r.logf("runner: lease %s is no longer open; stopping its job", id)
			cancel()
		}
	}
	orphans := map[string]bool{}
	for id := range open {
		if _, ok := r.running[id]; ok {
			continue
		}
		if r.orphans[id] {
			release = append(release, id)
		} else {
			orphans[id] = true
		}
	}
	r.orphans = orphans
	r.mu.Unlock()
	for _, id := range release {
		r.logf("runner: giving back lease %s that no job runs", id)
		if err := r.Client.Release(ctx, id); err != nil && ctx.Err() == nil {
			r.logf("runner: release %s: %v", id, err)
		}
	}
	return nil
}

func (r *Runner) claimable() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.state == "active" && !r.outdated
}

func (r *Runner) sleep(ctx context.Context, d time.Duration) {
	select {
	case <-ctx.Done():
	case <-time.After(d):
	}
}

func (r *Runner) claimLoop(ctx context.Context) {
	wait := r.ClaimWait
	if wait <= 0 {
		wait = 20
	}
	for ctx.Err() == nil {
		if !r.claimable() {
			r.sleep(ctx, r.heartbeatEvery())
			continue
		}
		claimed, ok, err := r.Client.Claim(ctx, wait)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			if StatusOf(err) == http.StatusConflict {
				r.mu.Lock()
				r.outdated = true
				r.mu.Unlock()
				r.logf("runner: the control plane runs another protocol; update this runner")
			} else {
				r.logf("runner: claim: %v", err)
			}
			r.sleep(ctx, r.retry())
			continue
		}
		if !ok {
			continue
		}
		r.runLease(ctx, claimed)
	}
}

// runLease runs one claimed job. When the job stops before its end, from a
// shutdown or an error, its lease is given back so the card is requeued now
// rather than when the lease expires.
func (r *Runner) runLease(ctx context.Context, claimed runnerapi.Claimed) {
	jobCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	r.mu.Lock()
	r.running[claimed.LeaseID] = cancel
	r.mu.Unlock()
	defer func() {
		r.mu.Lock()
		delete(r.running, claimed.LeaseID)
		r.mu.Unlock()
	}()
	r.logf("runner: card %s lease %s started", claimed.CardID, claimed.LeaseID)
	err := r.RunJob(jobCtx, claimed.LeaseID)
	if err == nil {
		r.logf("runner: card %s lease %s done", claimed.CardID, claimed.LeaseID)
		return
	}
	if ctx.Err() == nil && jobCtx.Err() != nil {
		r.logf("runner: card %s lease %s stopped by the control plane", claimed.CardID, claimed.LeaseID)
		return
	}
	r.logf("runner: card %s lease %s: %v", claimed.CardID, claimed.LeaseID, err)
	release, done := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer done()
	if err := r.Client.Release(release, claimed.LeaseID); err != nil && StatusOf(err) != http.StatusNotFound {
		r.logf("runner: release %s: %v", claimed.LeaseID, err)
	}
}
