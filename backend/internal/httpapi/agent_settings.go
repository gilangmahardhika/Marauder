package httpapi

import (
	"encoding/json"
	"net/http"

	"orchestrator/backend/internal/claudecode"
	"orchestrator/backend/internal/settings"
)

func agentJSON(cfg settings.Config) settings.AgentConfig {
	agent := cfg.Agent
	agent.Kind = cfg.AgentKind()
	return agent
}

// agentSettings reads or saves the company's implement agent (RFC-120). The
// reply is the saved setting, which holds no key.
func (s *Server) agentSettings(w http.ResponseWriter, r *http.Request) {
	if !requireAdmin(w, r) {
		return
	}
	if r.Method == http.MethodGet {
		cfg, _, err := s.loadSettings()
		if err != nil {
			writeErr(w, 500, "internal", "settings")
			return
		}
		writeJSON(w, 200, agentJSON(cfg))
		return
	}
	var body settings.AgentConfig
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, 400, "validation", "body")
		return
	}
	cfg, err := settings.SaveAgent(s.Data, body)
	if err != nil {
		writeAppErr(w, err)
		return
	}
	s.configChanged(r, "", "agent", []string{"kind", "claudeCode"})
	writeJSON(w, 200, agentJSON(cfg))
}

// agentTest runs claude --version on this server with the saved binary.
func (s *Server) agentTest(w http.ResponseWriter, r *http.Request) {
	if !requireAdmin(w, r) {
		return
	}
	cfg, _, err := s.loadSettings()
	if err != nil {
		writeErr(w, 500, "internal", "settings")
		return
	}
	bin, err := claudecode.Find(cfg.Agent.ClaudeCode.Binary)
	if err != nil {
		writeJSON(w, 200, map[string]any{"ok": false, "message": err.Error()})
		return
	}
	version, err := claudecode.Version(r.Context(), bin)
	if err != nil {
		writeJSON(w, 200, map[string]any{"ok": false, "message": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "version": version})
}
