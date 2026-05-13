package server

import (
	"encoding/json"
	"net/http"
	"os/exec"
	"strings"

	"claudio/internal/claude"
	"claudio/internal/domain"
	"claudio/internal/store"
	"claudio/internal/webui"
)

func CheckClaudeAuth() (authStatus string, versionStr string) {
	cmd := exec.Command("claude", "--version")
	out, err := cmd.Output()
	if err != nil {
		return "unknown", ""
	}
	versionStr = strings.TrimSpace(string(out))
	return "authenticated", versionStr
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	authStatus, ver := CheckClaudeAuth()
	status := "ok"
	if authStatus != "authenticated" {
		status = "degraded"
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status":         status,
		"claude_auth":    authStatus,
		"version":        domain.Version,
		"claude_version": ver,
	})
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Agent-Id, X-Session-Id")
		w.Header().Set("Access-Control-Expose-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func apiKeyMiddleware(apiKey string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" || r.URL.Path == "/" || strings.HasPrefix(r.URL.Path, "/web/") {
			next.ServeHTTP(w, r)
			return
		}
		if apiKey == "" {
			next.ServeHTTP(w, r)
			return
		}
		auth := r.Header.Get("Authorization")
		expected := "Bearer " + apiKey
		if auth != expected {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func WriteError(w http.ResponseWriter, status int, msg string) {
	WriteJSON(w, status, map[string]string{"error": msg})
}

func BuildMux(cfg domain.Config, st *store.Store) http.Handler {
	return BuildMuxWithRunner(cfg, st, nil)
}

func BuildMuxWithRunner(cfg domain.Config, st *store.Store, runner claude.Runner) http.Handler {
	mux := http.NewServeMux()
	webui.RegisterHandler(mux)
	mux.HandleFunc("GET /health", healthHandler)
	if st != nil {
		RegisterAgentHandlers(mux, cfg, st)
	}
	if st != nil {
		RegisterSessionHandlers(mux, cfg, st)
	}
	if st != nil {
		var claudeRunner claude.Runner
		if runner != nil {
			claudeRunner = runner
		} else {
			claudeRunner = claude.RunClaude
		}
		RegisterMessageHandler(mux, cfg, st, claudeRunner)
		RegisterOpenAIHandlers(mux, cfg, st, claudeRunner)
	}
	return corsMiddleware(apiKeyMiddleware(cfg.APIKey, mux))
}
