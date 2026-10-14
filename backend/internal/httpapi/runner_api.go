package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"orchestrator/backend/internal/app"
	"orchestrator/backend/internal/board"
	"orchestrator/backend/internal/runnerapi"
)

// maxBundle caps a runner's bundle upload (RFC-108).
const maxBundle = 64 << 20

// maxModelBody caps a runner's model request, which carries the attempt's
// whole history.
const maxModelBody = 16 << 20

func isBundleRoute(r *http.Request) bool {
	return strings.HasPrefix(r.URL.Path, "/api/runner/jobs/") && strings.HasSuffix(r.URL.Path, "/bundle")
}

func isModelRoute(r *http.Request) bool {
	return strings.HasPrefix(r.URL.Path, "/api/runner/jobs/") && strings.HasSuffix(r.URL.Path, "/model")
}

func runnerSecretOf(r *http.Request) string {
	value, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Runner ")
	if !ok {
		return ""
	}
	return strings.TrimSpace(value)
}

// runnerAuth admits a request on /api/runner/* with a live runner's secret.
// A user bearer never passes here.
func (s *Server) runnerAuth(next http.Handler, w http.ResponseWriter, r *http.Request) {
	if s.DB == nil {
		writeErr(w, 401, "unauthenticated", "runner")
		return
	}
	runner, err := app.RunnerBySecret(r.Context(), s.DB, runnerSecretOf(r))
	if err != nil {
		writeErr(w, 401, "unauthenticated", "runner")
		return
	}
	next.ServeHTTP(w, r.WithContext(app.WithRunner(r.Context(), runner)))
}

func (s *Server) runnerRoutes(mux *http.ServeMux) {
	if s.Remote == nil {
		s.Remote = app.NewRemoteJobs()
	}
	mux.HandleFunc("POST /api/runner/register", s.runnerRegister)
	mux.HandleFunc("POST /api/runner/heartbeat", s.runnerHeartbeat)
	mux.HandleFunc("POST /api/runner/claim", s.runnerClaim)
	mux.HandleFunc("POST /api/runner/jobs/{lease}/start", s.leaseRoute(s.runnerStart))
	mux.HandleFunc("POST /api/runner/jobs/{lease}/board", s.leaseRoute(s.runnerBoard))
	mux.HandleFunc("POST /api/runner/jobs/{lease}/model", s.leaseRoute(s.runnerModel))
	mux.HandleFunc("GET /api/runner/jobs/{lease}/mcp", s.leaseRoute(s.runnerMCPTools))
	mux.HandleFunc("POST /api/runner/jobs/{lease}/mcp", s.leaseRoute(s.runnerMCPCall))
	mux.HandleFunc("POST /api/runner/jobs/{lease}/bundle", s.leaseRoute(s.runnerBundle))
	mux.HandleFunc("POST /api/runner/jobs/{lease}/report", s.leaseRoute(s.runnerReport))
	mux.HandleFunc("POST /api/runner/jobs/{lease}/end", s.leaseRoute(s.runnerEnd))
	mux.HandleFunc("POST /api/runner/jobs/{lease}/release", s.leaseRoute(s.runnerRelease))
	mux.HandleFunc("GET /runner/install.sh", s.runnerInstall)
	mux.HandleFunc("GET /runner/download/{file}", s.runnerDownload)
}

func (s *Server) remoteDeps() app.JobDeps {
	return app.JobDeps{
		Data: s.Data, Key: s.TokenKey, Tools: s.Tools, Provider: s.JobProvider,
		Secrets: s.Secrets, Live: s.Live, MCP: s.MCP,
	}
}

// leaseRoute resolves {lease} to an open lease of the calling runner.
func (s *Server) leaseRoute(handle func(http.ResponseWriter, *http.Request, board.Lease)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		runner, ok := app.CurrentRunner(r.Context())
		if !ok {
			writeErr(w, 401, "unauthenticated", "runner")
			return
		}
		lease, err := app.RemoteLease(r.Context(), s.DB, runner, r.PathValue("lease"))
		if err != nil {
			if errors.Is(err, board.ErrNotFound) {
				s.Remote.Drop(r.PathValue("lease"))
			}
			writeRunnerErr(w, err)
			return
		}
		handle(w, r, lease)
	}
}

func decode(w http.ResponseWriter, r *http.Request, into any) bool {
	if err := json.NewDecoder(r.Body).Decode(into); err != nil {
		writeErr(w, 400, "validation", "body")
		return false
	}
	return true
}

func (s *Server) runnerRegister(w http.ResponseWriter, r *http.Request) {
	if s.DB == nil {
		writeErr(w, 401, "unauthenticated", "runner")
		return
	}
	var body runnerapi.Register
	if !decode(w, r, &body) {
		return
	}
	reg, err := app.Register(r.Context(), s.DB, app.RegisterRunner{
		Token: body.Token, Name: body.Name, Labels: body.Labels, Slots: body.Slots,
		Version: body.Version, Protocol: body.Protocol,
	})
	if err != nil {
		writeRunnerErr(w, err)
		return
	}
	writeJSON(w, 200, runnerapi.Registered{
		RunnerID: reg.ID, Secret: reg.Secret, HeartbeatSeconds: app.HeartbeatSeconds, Protocol: app.RunnerProtocol,
	})
}

func (s *Server) runnerHeartbeat(w http.ResponseWriter, r *http.Request) {
	runner, _ := app.CurrentRunner(r.Context())
	var body runnerapi.Heartbeat
	if !decode(w, r, &body) {
		return
	}
	state, leases, err := app.RunnerHeartbeat(r.Context(), s.DB, runner, body.Version, body.Protocol)
	if err != nil {
		writeRunnerErr(w, err)
		return
	}
	writeJSON(w, 200, runnerapi.HeartbeatReply{State: state, Protocol: app.RunnerProtocol, Leases: leases})
}

func (s *Server) runnerClaim(w http.ResponseWriter, r *http.Request) {
	runner, _ := app.CurrentRunner(r.Context())
	var body runnerapi.Claim
	if !decode(w, r, &body) {
		return
	}
	wait := min(max(body.WaitSeconds, 0), 25)
	deadline := time.Now().Add(time.Duration(wait) * time.Second)
	for {
		lease, err := app.ClaimForRunner(r.Context(), s.DB, s.Data, runner)
		if err == nil {
			writeJSON(w, 200, runnerapi.Claimed{LeaseID: lease.ID, CardID: lease.CardID, ProjectID: lease.ProjectID})
			return
		}
		if !errors.Is(err, board.ErrNotFound) {
			writeRunnerErr(w, err)
			return
		}
		if !time.Now().Before(deadline) {
			w.WriteHeader(204)
			return
		}
		select {
		case <-r.Context().Done():
			return
		case <-time.After(time.Second):
		}
	}
}

func (s *Server) runnerStart(w http.ResponseWriter, r *http.Request, lease board.Lease) {
	job, err := app.StartRemote(r.Context(), s.DB, s.remoteDeps(), s.Remote, lease)
	if err != nil {
		writeRunnerErr(w, err)
		return
	}
	writeJSON(w, 200, job)
}

func (s *Server) runnerBoard(w http.ResponseWriter, r *http.Request, lease board.Lease) {
	var op runnerapi.BoardOp
	if !decode(w, r, &op) {
		return
	}
	reply, err := app.RemoteBoardOp(r.Context(), s.DB, s.remoteDeps(), s.Remote, lease, op)
	if err != nil {
		writeRunnerErr(w, err)
		return
	}
	writeJSON(w, 200, reply)
}

func (s *Server) runnerModel(w http.ResponseWriter, r *http.Request, lease board.Lease) {
	var call runnerapi.ModelCall
	if !decode(w, r, &call) {
		return
	}
	resp, err := app.RemoteModel(r.Context(), s.DB, s.remoteDeps(), s.Remote, lease, call)
	if err != nil {
		writeRunnerErr(w, err)
		return
	}
	writeJSON(w, 200, runnerapi.ModelReply{Response: resp})
}

func (s *Server) runnerMCPTools(w http.ResponseWriter, r *http.Request, lease board.Lease) {
	tools, err := app.RemoteMCPTools(r.Context(), s.DB, s.remoteDeps(), s.Remote, lease)
	if err != nil {
		writeRunnerErr(w, err)
		return
	}
	writeJSON(w, 200, tools)
}

func (s *Server) runnerMCPCall(w http.ResponseWriter, r *http.Request, lease board.Lease) {
	var call runnerapi.MCPCall
	if !decode(w, r, &call) {
		return
	}
	res, err := app.RemoteMCPCall(r.Context(), s.DB, s.remoteDeps(), s.Remote, lease, call.Name, call.Args)
	if err != nil {
		writeRunnerErr(w, err)
		return
	}
	writeJSON(w, 200, runnerapi.MCPResult{Text: res.Text, IsError: res.IsError})
}

func (s *Server) runnerBundle(w http.ResponseWriter, r *http.Request, lease board.Lease) {
	if err := app.ImportBundle(r.Context(), s.DB, s.remoteDeps(), s.Remote, lease, r.URL.Query().Get("sha"), r.Body); err != nil {
		writeRunnerErr(w, err)
		return
	}
	w.WriteHeader(204)
}

func (s *Server) runnerReport(w http.ResponseWriter, r *http.Request, lease board.Lease) {
	var rep runnerapi.Report
	if !decode(w, r, &rep) {
		return
	}
	reply, err := app.ReportRemote(r.Context(), s.DB, s.remoteDeps(), s.Remote, lease, rep)
	if err != nil {
		writeRunnerErr(w, err)
		return
	}
	writeJSON(w, 200, reply)
}

func (s *Server) runnerEnd(w http.ResponseWriter, r *http.Request, lease board.Lease) {
	if err := app.EndRemote(r.Context(), s.DB, s.Remote, lease); err != nil {
		writeRunnerErr(w, err)
		return
	}
	w.WriteHeader(204)
}

func (s *Server) runnerRelease(w http.ResponseWriter, r *http.Request, lease board.Lease) {
	if err := app.ReleaseRemote(r.Context(), s.DB, s.Remote, lease); err != nil {
		writeRunnerErr(w, err)
		return
	}
	w.WriteHeader(204)
}

var runnerFiles = map[string]bool{
	"SHA256SUMS": true, "orchestrator-linux-amd64": true, "orchestrator-linux-arm64": true, "orchestrator-runner.service": true,
}

func (s *Server) runnerInstall(w http.ResponseWriter, r *http.Request) {
	s.serveRelease(w, r, "install-runner.sh", "text/x-shellscript")
}

func (s *Server) runnerDownload(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("file")
	if !runnerFiles[name] {
		writeErr(w, 404, "not_found", "not found")
		return
	}
	kind := "application/octet-stream"
	if name == "SHA256SUMS" {
		kind = "text/plain"
	}
	s.serveRelease(w, r, name, kind)
}

func (s *Server) serveRelease(w http.ResponseWriter, r *http.Request, name, kind string) {
	if s.RunnerReleases == "" {
		writeErr(w, 404, "not_found", "not found")
		return
	}
	path := filepath.Join(s.RunnerReleases, name)
	w.Header().Set("Content-Type", kind)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeFile(w, r, path)
}
