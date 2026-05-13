package connect

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// setConfigOverride sets the package-level test override and returns a cleanup func.
func setConfigOverride(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Cleanup(func() { configPathOverride = "" })
	configPathOverride = dir
	return dir
}

// writeJSON writes a JSON object to a file, creating parent dirs as needed.
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

// readJSON reads a JSON file into map[string]any.
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

// --- configPath tests (Tasks 1.1 / 1.2) ---

func TestConfigPath_ClaudeCode_ContainsSettingsJSON(t *testing.T) {
	setConfigOverride(t)
	path, err := configPath(TargetClaudeCode)
	if err != nil {
		t.Fatalf("configPath(claude-code): %v", err)
	}
	if !strings.HasSuffix(path, "claude-code.json") {
		t.Errorf("expected path to end with claude-code.json, got %q", path)
	}
}

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
	if !strings.Contains(err.Error(), "unknown-target") {
		t.Errorf("error should mention target, got %q", err.Error())
	}
}

// --- Connect tests (Tasks 1.3 / 1.4) ---

func TestConnect_ClaudeCode_CreatesEntry(t *testing.T) {
	dir := setConfigOverride(t)
	if err := Connect(TargetClaudeCode); err != nil {
		t.Fatalf("Connect(claude-code): %v", err)
	}

	// Verify plugin .mcp.json in cache directory
	mcpPath := filepath.Join(dir, "plugins", "cache", "claudio", "claudio", pluginVersion, ".mcp.json")
	mcpCfg := readJSON(t, mcpPath)
	servers, ok := mcpCfg["mcpServers"].(map[string]any)
	if !ok {
		t.Fatalf("mcpServers not a map in .mcp.json, got %T", mcpCfg["mcpServers"])
	}
	claudio, ok := servers["claudio"].(map[string]any)
	if !ok {
		t.Fatalf("mcpServers.claudio not a map, got %T", servers["claudio"])
	}
	if claudio["args"] == nil {
		t.Error("mcpServers.claudio.args should be set")
	}

	// Verify plugin.json in cache
	pluginPath := filepath.Join(dir, "plugins", "cache", "claudio", "claudio", pluginVersion, ".claude-plugin", "plugin.json")
	pluginCfg := readJSON(t, pluginPath)
	if pluginCfg["name"] != "claudio" {
		t.Errorf("plugin.json name = %v, want claudio", pluginCfg["name"])
	}

	// Verify installed_plugins.json
	ipPath := filepath.Join(dir, "plugins", "installed_plugins.json")
	ipCfg := readJSON(t, ipPath)
	plugins, ok := ipCfg["plugins"].(map[string]any)
	if !ok {
		t.Fatal("plugins not in installed_plugins.json")
	}
	if plugins["claudio@claudio"] == nil {
		t.Error("claudio@claudio should be in installed_plugins.json")
	}

	// Verify enabledPlugins in settings.json
	settingsPath := filepath.Join(dir, "claude-code.json")
	settings := readJSON(t, settingsPath)
	enabled, ok := settings["enabledPlugins"].(map[string]any)
	if !ok {
		t.Fatal("enabledPlugins not in settings.json")
	}
	if enabled["claudio@claudio"] != true {
		t.Error("claudio@claudio should be enabled in settings.json")
	}
}

func TestConnect_ClaudeDesktop_CreatesEntry(t *testing.T) {
	dir := setConfigOverride(t)
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
	path := filepath.Join(dir, "claude-code.json")

	// Confirm file doesn't exist before connect.
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("file should not exist yet")
	}

	if err := Connect(TargetClaudeCode); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	if _, err := os.Stat(path); err != nil {
		t.Errorf("file should have been created: %v", err)
	}
}

func TestConnect_PreservesExistingKeys(t *testing.T) {
	dir := setConfigOverride(t)
	path := filepath.Join(dir, "claude-code.json")

	// Pre-populate with existing keys.
	writeJSON(t, path, map[string]any{
		"someKey": "someValue",
		"mcpServers": map[string]any{
			"other-server": map[string]any{"command": "/bin/other"},
		},
	})

	if err := Connect(TargetClaudeCode); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	cfg := readJSON(t, path)

	// Existing top-level key preserved.
	if cfg["someKey"] != "someValue" {
		t.Errorf("someKey should be preserved, got %v", cfg["someKey"])
	}

	// Existing mcp server preserved.
	servers, _ := cfg["mcpServers"].(map[string]any)
	if _, ok := servers["other-server"]; !ok {
		t.Error("other-server entry should be preserved")
	}

	// enabledPlugins entry present in settings.json
	enabled, _ := cfg["enabledPlugins"].(map[string]any)
	if enabled["claudio@claudio"] != true {
		t.Error("claudio@claudio should be in enabledPlugins")
	}

	// Plugin cache should exist
	mcpPath := filepath.Join(dir, "plugins", "cache", "claudio", "claudio", pluginVersion, ".mcp.json")
	if _, err := os.Stat(mcpPath); err != nil {
		t.Errorf("plugin .mcp.json should exist: %v", err)
	}
}

func TestConnect_Idempotent(t *testing.T) {
	setConfigOverride(t)

	if err := Connect(TargetClaudeCode); err != nil {
		t.Fatalf("first Connect: %v", err)
	}
	if err := Connect(TargetClaudeCode); err != nil {
		t.Fatalf("second Connect (idempotent): %v", err)
	}
}

func TestConnect_InvalidTarget_ReturnsError(t *testing.T) {
	setConfigOverride(t)
	err := Connect("bogus")
	if err == nil {
		t.Fatal("expected error for invalid target")
	}
}

// --- Disconnect tests (Tasks 1.5 / 1.6) ---

func TestDisconnect_RemovesEntry(t *testing.T) {
	dir := setConfigOverride(t)
	path := filepath.Join(dir, "claude-code.json")

	writeJSON(t, path, map[string]any{
		"mcpServers": map[string]any{
			"claudio":      map[string]any{"command": "/bin/claudio", "args": []any{"mcp"}},
			"other-server": map[string]any{"command": "/bin/other"},
		},
	})

	if err := Disconnect(TargetClaudeCode); err != nil {
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

func TestDisconnect_NotConnected_NoError(t *testing.T) {
	dir := setConfigOverride(t)
	path := filepath.Join(dir, "claude-code.json")
	writeJSON(t, path, map[string]any{"mcpServers": map[string]any{}})

	if err := Disconnect(TargetClaudeCode); err != nil {
		t.Fatalf("Disconnect when not connected should not error: %v", err)
	}
}

func TestDisconnect_FileNotExist_NoError(t *testing.T) {
	setConfigOverride(t)
	// No file created — should be a no-op.
	if err := Disconnect(TargetClaudeCode); err != nil {
		t.Fatalf("Disconnect with missing file should not error: %v", err)
	}
}

func TestDisconnect_InvalidTarget_ReturnsError(t *testing.T) {
	setConfigOverride(t)
	err := Disconnect("bogus")
	if err == nil {
		t.Fatal("expected error for invalid target")
	}
}

// --- Status tests (Tasks 1.7 / 1.8) ---

func TestStatus_Connected_ReturnsTrue(t *testing.T) {
	dir := setConfigOverride(t)
	// Claude Code status checks for .mcp.json in cache directory
	cacheDir := filepath.Join(dir, "plugins", "cache", "claudio", "claudio", pluginVersion)
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeJSON(t, filepath.Join(cacheDir, ".mcp.json"), map[string]any{
		"mcpServers": map[string]any{
			"claudio": map[string]any{"command": "/bin/claudio", "args": []any{"mcp"}},
		},
	})

	connected, err := Status(TargetClaudeCode)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if !connected {
		t.Error("expected Status to return true when cache .mcp.json exists")
	}
}

func TestStatus_NotConnected_ReturnsFalse(t *testing.T) {
	dir := setConfigOverride(t)
	path := filepath.Join(dir, "claude-code.json")
	writeJSON(t, path, map[string]any{"mcpServers": map[string]any{}})

	connected, err := Status(TargetClaudeCode)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if connected {
		t.Error("expected Status to return false when claudio entry is absent")
	}
}

func TestStatus_FileNotExist_ReturnsFalse(t *testing.T) {
	setConfigOverride(t)
	// No file created.
	connected, err := Status(TargetClaudeCode)
	if err != nil {
		t.Fatalf("Status with missing file should not error: %v", err)
	}
	if connected {
		t.Error("expected Status to return false when file does not exist")
	}
}

func TestStatus_InvalidTarget_ReturnsError(t *testing.T) {
	setConfigOverride(t)
	_, err := Status("bogus")
	if err == nil {
		t.Fatal("expected error for invalid target")
	}
}

// --- StatusAll tests ---

func TestStatusAll_ReturnsBothTargets(t *testing.T) {
	dir := setConfigOverride(t)

	// Create Claude Code plugin cache structure
	cacheDir := filepath.Join(dir, "plugins", "cache", "claudio", "claudio", pluginVersion)
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeJSON(t, filepath.Join(cacheDir, ".mcp.json"), map[string]any{
		"mcpServers": map[string]any{
			"claudio": map[string]any{"command": "/bin/claudio", "args": []any{"mcp"}},
		},
	})

	all := StatusAll()
	if len(all) != 2 {
		t.Fatalf("StatusAll should return 2 entries, got %d", len(all))
	}
	if !all[TargetClaudeCode] {
		t.Error("claude-code should be connected")
	}
	if all[TargetClaudeDesktop] {
		t.Error("claude-desktop should be disconnected")
	}
}

// --- writeConfigAtomic test: atomic write does not corrupt on success ---

func TestWriteConfigAtomic_ProducesValidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.json")
	cfg := map[string]any{"key": "value"}

	if err := writeConfigAtomic(path, cfg); err != nil {
		t.Fatalf("writeConfigAtomic: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		t.Errorf("output is not valid JSON: %v", err)
	}
	if out["key"] != "value" {
		t.Errorf("expected key=value, got %v", out["key"])
	}
}

// binaryPath is exercised indirectly by Connect (it is called internally).
// If it were a go-build path, a warning is printed to stderr — not an error.
// The test below exercises Connect end-to-end which invokes binaryPath().
func TestConnect_BinaryPath_Resolved(t *testing.T) {
	setConfigOverride(t)
	// If binaryPath() fails, Connect returns an error.
	if err := Connect(TargetClaudeCode); err != nil {
		t.Fatalf("Connect should succeed even in test binary context: %v", err)
	}
}
