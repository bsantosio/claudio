package server_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"claudio/internal/connect"
	"claudio/internal/domain"
	"claudio/internal/server"
)

// newConnectHandler returns a BuildMux handler with no store (connect is stateless).
func newConnectHandler(t *testing.T) http.Handler {
	t.Helper()
	cfg := domain.Config{DefaultModel: "sonnet"}
	return server.BuildMux(cfg, nil)
}

// setConnectTestDir redirects the connect package's config path to a temp dir
// so tests do not touch ~/.claude/settings.json or ~/Library/.../claude_desktop_config.json.
func setConnectTestDir(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	connect.SetConfigPathOverride(dir)
	t.Cleanup(func() { connect.SetConfigPathOverride("") })
}

// --- GET /api/connect/status ---

func TestGetConnectStatus_Returns200WithBothTargets(t *testing.T) {
	setConnectTestDir(t)
	h := newConnectHandler(t)

	req := httptest.NewRequest("GET", "/api/connect/status", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]bool
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if _, ok := resp[connect.TargetClaudeCode]; !ok {
		t.Errorf("response missing claude-code field, got %v", resp)
	}
	if _, ok := resp[connect.TargetClaudeDesktop]; !ok {
		t.Errorf("response missing claude-desktop field, got %v", resp)
	}
}

func TestGetConnectStatus_ContentTypeIsJSON(t *testing.T) {
	setConnectTestDir(t)
	h := newConnectHandler(t)

	req := httptest.NewRequest("GET", "/api/connect/status", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	ct := w.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %q", ct)
	}
}

// --- POST /api/connect/{target} ---

func TestPostConnectTarget_ValidTarget_Returns200(t *testing.T) {
	setConnectTestDir(t)
	h := newConnectHandler(t)

	req := httptest.NewRequest("POST", "/api/connect/claude-code", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestPostConnectTarget_ClaudeDesktop_Returns200(t *testing.T) {
	setConnectTestDir(t)
	h := newConnectHandler(t)

	req := httptest.NewRequest("POST", "/api/connect/claude-desktop", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestPostConnectTarget_InvalidTarget_Returns400(t *testing.T) {
	setConnectTestDir(t)
	h := newConnectHandler(t)

	req := httptest.NewRequest("POST", "/api/connect/not-a-target", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestPostConnectTarget_InvalidTarget_ResponseHasErrorField(t *testing.T) {
	setConnectTestDir(t)
	h := newConnectHandler(t)

	req := httptest.NewRequest("POST", "/api/connect/not-a-target", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode error response: %v", err)
	}
	if _, ok := resp["error"]; !ok {
		t.Errorf("error response should have 'error' field, got %v", resp)
	}
}

// --- DELETE /api/connect/{target} ---

func TestDeleteConnectTarget_ValidTarget_Returns200(t *testing.T) {
	setConnectTestDir(t)
	h := newConnectHandler(t)

	// Connect first, then disconnect.
	postReq := httptest.NewRequest("POST", "/api/connect/claude-code", nil)
	postW := httptest.NewRecorder()
	h.ServeHTTP(postW, postReq)

	req := httptest.NewRequest("DELETE", "/api/connect/claude-code", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestDeleteConnectTarget_WhenNotConnected_Returns200(t *testing.T) {
	setConnectTestDir(t)
	h := newConnectHandler(t)

	req := httptest.NewRequest("DELETE", "/api/connect/claude-code", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for idempotent disconnect, got %d", w.Code)
	}
}

func TestDeleteConnectTarget_InvalidTarget_Returns400(t *testing.T) {
	setConnectTestDir(t)
	h := newConnectHandler(t)

	req := httptest.NewRequest("DELETE", "/api/connect/not-a-target", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestDeleteConnectTarget_InvalidTarget_ResponseHasErrorField(t *testing.T) {
	setConnectTestDir(t)
	h := newConnectHandler(t)

	req := httptest.NewRequest("DELETE", "/api/connect/not-a-target", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode error response: %v", err)
	}
	if _, ok := resp["error"]; !ok {
		t.Errorf("error response should have 'error' field, got %v", resp)
	}
}

// --- Status reflects actual state ---

func TestConnectStatus_ReflectsRealState(t *testing.T) {
	setConnectTestDir(t)
	h := newConnectHandler(t)

	// Initially disconnected.
	req := httptest.NewRequest("GET", "/api/connect/status", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	var before map[string]bool
	json.NewDecoder(w.Body).Decode(&before)
	if before[connect.TargetClaudeCode] {
		t.Error("claude-code should be disconnected initially")
	}

	// Connect claude-code.
	postReq := httptest.NewRequest("POST", "/api/connect/claude-code", nil)
	postW := httptest.NewRecorder()
	h.ServeHTTP(postW, postReq)

	// Status should now show claude-code as connected.
	req2 := httptest.NewRequest("GET", "/api/connect/status", nil)
	w2 := httptest.NewRecorder()
	h.ServeHTTP(w2, req2)
	var after map[string]bool
	json.NewDecoder(w2.Body).Decode(&after)
	if !after[connect.TargetClaudeCode] {
		t.Error("claude-code should be connected after POST")
	}
}
