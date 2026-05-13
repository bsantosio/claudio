package server

import (
	"net/http"

	"claudio/internal/connect"
)

// registerConnectHandlers wires the connect/disconnect/status endpoints onto
// mux. These handlers are stateless (no store dependency) and are registered
// unconditionally — outside the "if st != nil" guard in BuildMuxWithRunner.
func registerConnectHandlers(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/connect/status", getConnectStatus)
	mux.HandleFunc("POST /api/connect/{target}", connectTarget)
	mux.HandleFunc("DELETE /api/connect/{target}", disconnectTarget)
}

// getConnectStatus returns the connection status for all known targets.
//
// GET /api/connect/status
// Response 200: {"claude-code": false, "claude-desktop": false}
func getConnectStatus(w http.ResponseWriter, r *http.Request) {
	WriteJSON(w, http.StatusOK, connect.StatusAll())
}

// connectTarget registers claudio as an MCP server in the given target's config.
//
// POST /api/connect/{target}
// Response 200: {"status": "connected", "target": "..."}
// Response 400: {"error": "..."} — invalid target
// Response 500: {"error": "..."} — connect failed
func connectTarget(w http.ResponseWriter, r *http.Request) {
	target := r.PathValue("target")
	if target != connect.TargetClaudeCode && target != connect.TargetClaudeDesktop {
		WriteError(w, http.StatusBadRequest, "invalid target: use claude-code or claude-desktop")
		return
	}
	if err := connect.Connect(target); err != nil {
		WriteError(w, http.StatusInternalServerError, "connect failed")
		return
	}
	WriteJSON(w, http.StatusOK, map[string]string{"status": "connected", "target": target})
}

// disconnectTarget removes claudio from the given target's config.
//
// DELETE /api/connect/{target}
// Response 200: {"status": "disconnected", "target": "..."}
// Response 400: {"error": "..."} — invalid target
// Response 500: {"error": "..."} — disconnect failed
func disconnectTarget(w http.ResponseWriter, r *http.Request) {
	target := r.PathValue("target")
	if target != connect.TargetClaudeCode && target != connect.TargetClaudeDesktop {
		WriteError(w, http.StatusBadRequest, "invalid target: use claude-code or claude-desktop")
		return
	}
	if err := connect.Disconnect(target); err != nil {
		WriteError(w, http.StatusInternalServerError, "disconnect failed")
		return
	}
	WriteJSON(w, http.StatusOK, map[string]string{"status": "disconnected", "target": target})
}
