package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"orchestrator/backend/internal/app"
	"orchestrator/backend/internal/trace"
)

func (s *Server) mcpServers(w http.ResponseWriter, r *http.Request) {
	actor, _ := app.CurrentUser(r.Context())
	list, err := app.ListMCPServers(r.Context(), s.DB, actor, r.PathValue("id"))
	if err != nil {
		writeAccountErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"servers": list})
}

func (s *Server) createMCPServer(w http.ResponseWriter, r *http.Request) {
	actor, _ := app.CurrentUser(r.Context())
	var body app.MCPServerInput
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, 422, "validation", "body")
		return
	}
	srv, fields, err := app.CreateMCPServer(r.Context(), s.DB, actor, s.TokenKey, s.MCP, r.PathValue("id"), body)
	if err != nil {
		writeAccountErr(w, err)
		return
	}
	s.mcpChanged(r, srv.TribeID, srv.ID, fields)
	writeJSON(w, 201, srv)
}

func (s *Server) saveMCPServer(w http.ResponseWriter, r *http.Request) {
	actor, _ := app.CurrentUser(r.Context())
	var body app.MCPServerInput
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, 422, "validation", "body")
		return
	}
	srv, fields, err := app.SaveMCPServer(r.Context(), s.DB, actor, s.TokenKey, s.MCP, r.PathValue("id"), r.PathValue("serverId"), body)
	if err != nil {
		writeAccountErr(w, err)
		return
	}
	s.mcpChanged(r, srv.TribeID, srv.ID, fields)
	writeJSON(w, 200, srv)
}

func (s *Server) deleteMCPServer(w http.ResponseWriter, r *http.Request) {
	actor, _ := app.CurrentUser(r.Context())
	if err := app.DeleteMCPServer(r.Context(), s.DB, actor, r.PathValue("id"), r.PathValue("serverId")); err != nil {
		writeAccountErr(w, err)
		return
	}
	s.mcpChanged(r, r.PathValue("id"), r.PathValue("serverId"), []string{"deleted"})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) discoverMCPTools(w http.ResponseWriter, r *http.Request) {
	actor, _ := app.CurrentUser(r.Context())
	tools, err := app.DiscoverMCPTools(r.Context(), s.DB, actor, s.TokenKey, s.MCP, r.PathValue("id"), r.PathValue("serverId"))
	var de *app.DiscoverError
	if errors.As(err, &de) {
		writeJSON(w, 200, map[string]any{"ok": false, "message": de.Reason, "tools": []any{}})
		return
	}
	if err != nil {
		writeAccountErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "tools": tools})
}

func (s *Server) mcpCatalog(w http.ResponseWriter, r *http.Request) {
	actor, _ := app.CurrentUser(r.Context())
	q := r.URL.Query()
	if id := q.Get("id"); id != "" {
		entry, err := app.MCPCatalogItem(r.Context(), s.DB, actor, s.Catalog, r.PathValue("id"), id)
		if errors.Is(err, app.ErrRegistry) {
			writeErr(w, 502, "registry", err.Error())
			return
		}
		if err != nil {
			writeAccountErr(w, err)
			return
		}
		writeJSON(w, 200, map[string]any{"entries": []app.MCPCatalogEntry{entry}, "nextCursor": ""})
		return
	}
	page, err := app.MCPCatalog(r.Context(), s.DB, actor, s.Catalog, r.PathValue("id"), q.Get("source"), q.Get("q"), q.Get("cursor"))
	if err != nil {
		writeAccountErr(w, err)
		return
	}
	writeJSON(w, 200, page)
}

func (s *Server) pullMCPImage(w http.ResponseWriter, r *http.Request) {
	actor, _ := app.CurrentUser(r.Context())
	var body struct {
		Image string `json:"image"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, 422, "validation", "body")
		return
	}
	pinned, err := app.PullMCPImage(r.Context(), s.DB, actor, s.Images, r.PathValue("id"), body.Image)
	if err != nil {
		writeAccountErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"image": pinned})
}

// mcpChanged records a tribe MCP save with field names only (RFC-092).
func (s *Server) mcpChanged(r *http.Request, tribeID, serverID string, fields []string) {
	user, _ := app.CurrentUser(r.Context())
	_, _ = trace.Append(r.Context(), s.DB, trace.Event{
		Kind: "config_changed", ActorID: user.ID,
		Payload: map[string]any{"section": "mcp", "tribeId": tribeID, "serverId": serverID, "fields": fields},
	}, nil)
}
