// Package server implements the HTTP API and OpenAI-compatible endpoints.
package server

import (
	"encoding/json"
	"log"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"claudio/internal/claude"
	"claudio/internal/domain"
	"claudio/internal/store"
	"claudio/internal/webui"
)

// Server holds shared dependencies for all HTTP handlers.
type Server struct {
	cfg    domain.Config
	store  *store.Store
	runner claude.Runner
}

// New creates a Server. If runner is nil, claude.RunClaude is used.
func New(cfg domain.Config, st *store.Store, runner claude.Runner) *Server {
	if runner == nil {
		runner = claude.RunClaude
	}
	return &Server{cfg: cfg, store: st, runner: runner}
}

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

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %v", r.Method, r.URL.Path, time.Since(start))
	})
}

func apiKeyMiddleware(apiKey string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" || r.URL.Path == "/api/prompt" || r.URL.Path == "/api/templates" || r.URL.Path == "/" || strings.HasPrefix(r.URL.Path, "/web/") {
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

func templatesHandler(w http.ResponseWriter, r *http.Request) {
	WriteJSON(w, http.StatusOK, domain.AgentTemplates)
}

func promptHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(domain.AIPrompt))
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
	s := New(cfg, st, runner)
	mux := http.NewServeMux()
	webui.RegisterHandler(mux)
	mux.HandleFunc("GET /health", healthHandler)
	mux.HandleFunc("GET /api/prompt", promptHandler)
	mux.HandleFunc("GET /api/templates", templatesHandler)
	if st != nil {
		s.registerAgentHandlers(mux)
		s.registerSessionHandlers(mux)
		s.registerMessageHandlers(mux)
		s.registerOpenAIHandlers(mux)
	}
	return corsMiddleware(loggingMiddleware(apiKeyMiddleware(cfg.APIKey, mux)))
}
