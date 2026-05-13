package connect

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func setConfigOverride(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Cleanup(func() { configPathOverride = "" })
	configPathOverride = dir
	return dir
}

func writeJSON(t *testing.T, path string, v any) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

func readJSON(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile %s: %v", path, err)
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	return out
}

// mockClaudeMCP replaces the real `claude mcp` runner with a test double.
// Returns a pointer to a slice that captures all calls.
func mockClaudeMCP(t *testing.T) *[][]string {
	t.Helper()
	var calls [][]string
	old := runClaudeMCP
	t.Cleanup(func() { runClaudeMCP = old })
	runClaudeMCP = func(args ...string) ([]byte, error) {
		calls = append(calls, args)
		if len(args) >= 1 && args[0] == "get" {
			if len(calls) > 0 {
				for _, c := range calls {
					if c[0] == "add" {
						return []byte(`{"name":"claudio"}`), nil
					}
				}
			}
			return nil, fmt.Errorf("not found")
		}
		return []byte("ok"), nil
	}
	return &calls
}

// --- configPath tests ---

func TestConfigPath_ClaudeDesktop_ContainsDesktop(t *testing.T) {
	setConfigOverride(t)
	path, err := configPath(TargetClaudeDesktop)
	if err != nil {
		t.Fatalf("configPath(claude-desktop): %v", err)
	}
	if !strings.HasSuffix(path, "claude-desktop.json") {
		t.Errorf("expected path to end with claude-desktop.json, got %q", path)
	}
}

func TestConfigPath_InvalidTarget_ReturnsError(t *testing.T) {
	_, err := configPath("unknown-target")
	if err == nil {
		t.Fatal("expected error for unknown target, got nil")
	}
}

// --- Connect Claude Code tests (via `claude mcp add`) ---

func TestConnect_ClaudeCode_CallsClaudeMCPAdd(t *testing.T) {
	calls := mockClaudeMCP(t)
	if err := Connect(TargetClaudeCode); err != nil {
		t.Fatalf("Connect(claude-code): %v", err)
	}

	// First call: remove (idempotency cleanup)
	// Second call: add
	if len(*calls) < 2 {
		t.Fatalf("expected at least 2 claude mcp calls, got %d", len(*calls))
	}
	if (*calls)[0][0] != "remove" {
		t.Errorf("first call should be remove, got %v", (*calls)[0])
	}
	addCall := (*calls)[1]
	if addCall[0] != "add" {
		t.Errorf("second call should be add, got %v", addCall)
	}
	hasName := false
	for _, arg := range addCall {
		if arg == "claudio" {
			hasName = true
		}
	}
	if !hasName {
		t.Errorf("add call should include name 'claudio', got %v", addCall)
	}
}

func TestConnect_ClaudeCode_Idempotent(t *testing.T) {
	mockClaudeMCP(t)
	if err := Connect(TargetClaudeCode); err != nil {
		t.Fatalf("first Connect: %v", err)
	}
	if err := Connect(TargetClaudeCode); err != nil {
		t.Fatalf("second Connect (idempotent): %v", err)
	}
}

// --- Connect Claude Desktop tests (file-based) ---

func TestConnect_ClaudeDesktop_CreatesEntry(t *testing.T) {
	dir := setConfigOverride(t)
	mockClaudeMCP(t)
	if err := Connect(TargetClaudeDesktop); err != nil {
		t.Fatalf("Connect(claude-desktop): %v", err)
	}

	path := filepath.Join(dir, "claude-desktop.json")
	cfg := readJSON(t, path)

	servers, ok := cfg["mcpServers"].(map[string]any)
	if !ok {
		t.Fatalf("mcpServers not a map")
	}
	if _, ok := servers["claudio"]; !ok {
		t.Error("mcpServers.claudio should exist")
	}
}

func TestConnect_CreatesFile_IfNotExist(t *testing.T) {
	dir := setConfigOverride(t)
	mockClaudeMCP(t)
	path := filepath.Join(dir, "claude-desktop.json")

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("file should not exist yet")
	}

	if err := Connect(TargetClaudeDesktop); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	if _, err := os.Stat(path); err != nil {
		t.Errorf("file should have been created: %v", err)
	}
}

func TestConnect_PreservesExistingKeys(t *testing.T) {
	dir := setConfigOverride(t)
	mockClaudeMCP(t)
	path := filepath.Join(dir, "claude-desktop.json")

	writeJSON(t, path, map[string]any{
		"someKey": "someValue",
		"mcpServers": map[string]any{
			"other-server": map[string]any{"command": "/bin/other"},
		},
	})

	if err := Connect(TargetClaudeDesktop); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	cfg := readJSON(t, path)
	if cfg["someKey"] != "someValue" {
		t.Errorf("someKey should be preserved, got %v", cfg["someKey"])
	}

	servers, _ := cfg["mcpServers"].(map[string]any)
	if _, ok := servers["other-server"]; !ok {
		t.Error("other-server entry should be preserved")
	}
	if _, ok := servers["claudio"]; !ok {
		t.Error("claudio entry should be added")
	}
}

func TestConnect_InvalidTarget_ReturnsError(t *testing.T) {
	mockClaudeMCP(t)
	err := Connect("bogus")
	if err == nil {
		t.Fatal("expected error for invalid target")
	}
}

// --- Disconnect tests ---

func TestDisconnect_ClaudeCode_CallsClaudeMCPRemove(t *testing.T) {
	calls := mockClaudeMCP(t)
	if err := Disconnect(TargetClaudeCode); err != nil {
		t.Fatalf("Disconnect: %v", err)
	}
	if len(*calls) != 1 || (*calls)[0][0] != "remove" {
		t.Errorf("expected single remove call, got %v", *calls)
	}
}

func TestDisconnect_ClaudeDesktop_RemovesEntry(t *testing.T) {
	dir := setConfigOverride(t)
	mockClaudeMCP(t)
	path := filepath.Join(dir, "claude-desktop.json")

	writeJSON(t, path, map[string]any{
		"mcpServers": map[string]any{
			"claudio":      map[string]any{"command": "/bin/claudio", "args": []any{"mcp"}},
			"other-server": map[string]any{"command": "/bin/other"},
		},
	})

	if err := Disconnect(TargetClaudeDesktop); err != nil {
		t.Fatalf("Disconnect: %v", err)
	}

	cfg := readJSON(t, path)
	servers, _ := cfg["mcpServers"].(map[string]any)
	if _, ok := servers["claudio"]; ok {
		t.Error("claudio entry should have been removed")
	}
	if _, ok := servers["other-server"]; !ok {
		t.Error("other-server entry should be preserved")
	}
}

func TestDisconnect_ClaudeDesktop_FileNotExist_NoError(t *testing.T) {
	setConfigOverride(t)
	mockClaudeMCP(t)
	if err := Disconnect(TargetClaudeDesktop); err != nil {
		t.Fatalf("Disconnect with missing file should not error: %v", err)
	}
}

func TestDisconnect_InvalidTarget_ReturnsError(t *testing.T) {
	mockClaudeMCP(t)
	err := Disconnect("bogus")
	if err == nil {
		t.Fatal("expected error for invalid target")
	}
}

// --- Status tests ---

func TestStatus_ClaudeCode_UsesClaudeMCPGet(t *testing.T) {
	calls := mockClaudeMCP(t)

	// Before any add call, status should be false
	connected, err := Status(TargetClaudeCode)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if connected {
		t.Error("expected Status to return false before connect")
	}

	// Simulate a connect (adds an "add" call to history)
	Connect(TargetClaudeCode)

	connected, err = Status(TargetClaudeCode)
	if err != nil {
		t.Fatalf("Status after connect: %v", err)
	}
	if !connected {
		t.Error("expected Status to return true after connect")
	}

	// Verify get was called
	getFound := false
	for _, c := range *calls {
		if c[0] == "get" {
			getFound = true
		}
	}
	if !getFound {
		t.Error("expected claude mcp get to be called")
	}
}

func TestStatus_ClaudeDesktop_Connected(t *testing.T) {
	dir := setConfigOverride(t)
	mockClaudeMCP(t)
	path := filepath.Join(dir, "claude-desktop.json")
	writeJSON(t, path, map[string]any{
		"mcpServers": map[string]any{
			"claudio": map[string]any{"command": "/bin/claudio"},
		},
	})

	connected, err := Status(TargetClaudeDesktop)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if !connected {
		t.Error("expected Status to return true")
	}
}

func TestStatus_ClaudeDesktop_NotConnected(t *testing.T) {
	dir := setConfigOverride(t)
	mockClaudeMCP(t)
	path := filepath.Join(dir, "claude-desktop.json")
	writeJSON(t, path, map[string]any{"mcpServers": map[string]any{}})

	connected, err := Status(TargetClaudeDesktop)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if connected {
		t.Error("expected Status to return false")
	}
}

func TestStatus_InvalidTarget_ReturnsError(t *testing.T) {
	mockClaudeMCP(t)
	_, err := Status("bogus")
	if err == nil {
		t.Fatal("expected error for invalid target")
	}
}

// --- StatusAll tests ---

func TestStatusAll_ReturnsBothTargets(t *testing.T) {
	mockClaudeMCP(t)
	all := StatusAll()
	if len(all) != 2 {
		t.Fatalf("StatusAll should return 2 entries, got %d", len(all))
	}
}

// --- writeConfigAtomic ---

func TestWriteConfigAtomic_ProducesValidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.json")
	cfg := map[string]any{"key": "value"}

	if err := writeConfigAtomic(path, cfg); err != nil {
		t.Fatalf("writeConfigAtomic: %v", err)
	}

	out := readJSON(t, path)
	if out["key"] != "value" {
		t.Errorf("expected key=value, got %v", out["key"])
	}
}

func TestConnect_BinaryPath_Resolved(t *testing.T) {
	mockClaudeMCP(t)
	if err := Connect(TargetClaudeCode); err != nil {
		t.Fatalf("Connect should succeed: %v", err)
	}
}
