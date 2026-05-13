// Package connect manages claudio's registration as an MCP server
// in Claude Code and Claude Desktop configuration files.
package connect

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Target identifiers for supported Claude products.
const (
	TargetClaudeCode    = "claude-code"
	TargetClaudeDesktop = "claude-desktop"
)

// mu serializes read-modify-write cycles in Connect and Disconnect.
var mu sync.Mutex

// configPathOverride is set only during testing (via SetConfigPathOverride).
// Tests must not use t.Parallel() when overriding this value.
var configPathOverride string

// SetConfigPathOverride sets the test override directory for config file paths.
// Pass an empty string to restore the real home-directory-based paths.
// This is exported for use by integration tests in other packages.
func SetConfigPathOverride(dir string) {
	configPathOverride = dir
}

// configPath returns the absolute path to the config file for the given target.
// In tests, configPathOverride redirects to a temp dir.
func configPath(target string) (string, error) {
	if configPathOverride != "" {
		switch target {
		case TargetClaudeCode, TargetClaudeDesktop:
			return filepath.Join(configPathOverride, target+".json"), nil
		default:
			return "", fmt.Errorf("connect: unknown target %q", target)
		}
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("connect: cannot determine home directory: %w", err)
	}
	switch target {
	case TargetClaudeCode:
		return filepath.Join(home, ".claude", "settings.json"), nil
	case TargetClaudeDesktop:
		return filepath.Join(home, "Library", "Application Support", "Claude", "claude_desktop_config.json"), nil
	default:
		return "", fmt.Errorf("connect: unknown target %q", target)
	}
}

// findBinary returns the absolute path of the running executable,
// resolving any symlinks. If the path contains "go-build" (a temporary
// build artifact from `go run`), a warning is printed to stderr.
func findBinary() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("connect: cannot determine executable path: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(exe)
	if err != nil {
		// EvalSymlinks can fail on some systems; fall back to the raw path.
		resolved = exe
	}
	if strings.Contains(resolved, "go-build") {
		fmt.Fprintf(os.Stderr, "warning: claudio binary path %q looks like a temporary go build artifact; MCP server path may not persist\n", resolved)
	}
	return resolved, nil
}

// readConfig reads and unmarshals a JSON config file into a map.
// If the file does not exist, it returns an empty map (no error).
func readConfig(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return make(map[string]any), nil
		}
		return nil, fmt.Errorf("connect: read config %s: %w", path, err)
	}
	var cfg map[string]any
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("connect: parse config %s: %w", path, err)
	}
	return cfg, nil
}

// writeConfigAtomic writes data to path atomically using a temp file in the
// same directory followed by os.Rename. This prevents corrupt configs on crash.
func writeConfigAtomic(path string, data map[string]any) error {
	out, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("connect: marshal config: %w", err)
	}
	out = append(out, '\n')

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("connect: create config dir %s: %w", dir, err)
	}

	// Preserve original file permissions if the file already exists.
	perm := os.FileMode(0o644)
	if info, err := os.Stat(path); err == nil {
		perm = info.Mode().Perm()
	}

	tmp, err := os.CreateTemp(dir, ".claudio-config-*.tmp")
	if err != nil {
		return fmt.Errorf("connect: create temp file: %w", err)
	}
	tmpPath := tmp.Name()

	if _, err := tmp.Write(out); err != nil {
		tmp.Close()        //nolint:errcheck
		os.Remove(tmpPath) //nolint:errcheck
		return fmt.Errorf("connect: write temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath) //nolint:errcheck
		return fmt.Errorf("connect: close temp file: %w", err)
	}
	if err := os.Chmod(tmpPath, perm); err != nil {
		os.Remove(tmpPath) //nolint:errcheck
		return fmt.Errorf("connect: chmod temp file: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath) //nolint:errcheck
		return fmt.Errorf("connect: rename config file: %w", err)
	}
	return nil
}

// Connect registers claudio as an MCP server in the target's config file.
// It preserves all existing keys and is idempotent — calling Connect when
// already registered updates the entry with the current binary path.
func Connect(target string) error {
	mu.Lock()
	defer mu.Unlock()

	path, err := configPath(target)
	if err != nil {
		return err
	}
	bin, err := findBinary()
	if err != nil {
		return err
	}

	cfg, err := readConfig(path)
	if err != nil {
		return err
	}

	// Ensure mcpServers key exists.
	mcpRaw, ok := cfg["mcpServers"]
	if !ok {
		mcpRaw = map[string]any{}
	}
	mcp, ok := mcpRaw.(map[string]any)
	if !ok {
		return fmt.Errorf("connect: unexpected type for mcpServers in %s", path)
	}

	mcp["claudio"] = map[string]any{
		"command": bin,
		"args":    []any{"mcp"},
	}
	cfg["mcpServers"] = mcp

	return writeConfigAtomic(path, cfg)
}

// Disconnect removes claudio from the target's config file. It preserves all
// other keys and is idempotent — calling Disconnect when not registered is a no-op.
func Disconnect(target string) error {
	mu.Lock()
	defer mu.Unlock()

	path, err := configPath(target)
	if err != nil {
		return err
	}

	cfg, err := readConfig(path)
	if err != nil {
		return err
	}
	if len(cfg) == 0 {
		// File did not exist — nothing to remove.
		return nil
	}

	mcpRaw, ok := cfg["mcpServers"]
	if !ok {
		// No mcpServers key — nothing to remove.
		return nil
	}
	mcp, ok := mcpRaw.(map[string]any)
	if !ok {
		return fmt.Errorf("connect: unexpected type for mcpServers in %s", path)
	}

	if _, exists := mcp["claudio"]; !exists {
		// Already not connected — idempotent success.
		return nil
	}

	delete(mcp, "claudio")
	cfg["mcpServers"] = mcp

	return writeConfigAtomic(path, cfg)
}

// Status returns true if mcpServers.claudio is present in the target config file.
// If the file does not exist, false is returned without error.
func Status(target string) (bool, error) {
	path, err := configPath(target)
	if err != nil {
		return false, err
	}
	cfg, err := readConfig(path)
	if err != nil {
		return false, err
	}
	mcpRaw, ok := cfg["mcpServers"]
	if !ok {
		return false, nil
	}
	mcp, ok := mcpRaw.(map[string]any)
	if !ok {
		return false, nil
	}
	_, connected := mcp["claudio"]
	return connected, nil
}

// StatusAll returns the connection state for all known targets.
// A target is considered connected if mcpServers.claudio exists in its config file.
// Errors are treated as disconnected (returns false).
func StatusAll() map[string]bool {
	result := map[string]bool{}
	for _, target := range []string{TargetClaudeCode, TargetClaudeDesktop} {
		connected, err := Status(target)
		if err != nil {
			connected = false
		}
		result[target] = connected
	}
	return result
}
