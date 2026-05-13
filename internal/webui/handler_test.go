package webui_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"claudio/internal/webui"
)

func newMux(t *testing.T) *http.ServeMux {
	t.Helper()
	mux := http.NewServeMux()
	webui.RegisterHandler(mux)
	return mux
}

// Task 3.1 — GET / returns 200 with index.html content.
func TestRegisterHandler_ServesIndexAtRoot(t *testing.T) {
	mux := newMux(t)
	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 at /, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "claudio") {
		t.Errorf("expected index.html content at /, got %q", body)
	}
}

// Task 3.2 — GET /web/app.js returns 200.
func TestRegisterHandler_ServesAssetAtWebPath(t *testing.T) {
	mux := newMux(t)
	req := httptest.NewRequest("GET", "/web/app.js", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for /web/app.js, got %d", w.Code)
	}
}

// Task 3.3 — GET /web/nonexistent.png returns 404.
func TestRegisterHandler_UnknownAsset_Returns404(t *testing.T) {
	mux := newMux(t)
	req := httptest.NewRequest("GET", "/web/nonexistent.png", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for missing asset, got %d", w.Code)
	}
}
