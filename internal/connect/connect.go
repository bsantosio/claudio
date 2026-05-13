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

const pluginName = "claudio"

// claudeHome returns the base Claude config directory (~/.claude).
func claudeHome() (string, error) {
	if configPathOverride != "" {
		return configPathOverride, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("connect: cannot determine home directory: %w", err)
	}
	return filepath.Join(home, ".claude"), nil
}

// configPath returns the absolute path to the config file for the given target.
func configPath(target string) (string, error) {
	if configPathOverride != "" {
		return filepath.Join(configPathOverride, target+".json"), nil
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

// pluginDir returns the path to claudio's plugin directory for Claude Code.
func pluginDir() (string, error) {
	ch, err := claudeHome()
	if err != nil {
		return "", err
	}
	return filepath.Join(ch, "plugins", "marketplaces", pluginName, "plugin", "claude-code"), nil
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

// Connect registers claudio as an MCP server in the target application.
// For Claude Code, it creates a plugin directory with .mcp.json and plugin.json,
// then enables the plugin in settings.json. For Claude Desktop, it adds an
// mcpServers entry to the config file. Idempotent — safe to call multiple times.
func Connect(target string) error {
	mu.Lock()
	defer mu.Unlock()

	bin, err := findBinary()
	if err != nil {
		return err
	}

	switch target {
	case TargetClaudeCode:
		return connectClaudeCode(bin)
	case TargetClaudeDesktop:
		return connectClaudeDesktop(bin)
	default:
		return fmt.Errorf("connect: unknown target %q", target)
	}
}

func connectClaudeCode(bin string) error {
	pdir, err := pluginDir()
	if err != nil {
		return err
	}

	// Create plugin directory structure
	pluginMetaDir := filepath.Join(pdir, ".claude-plugin")
	if err := os.MkdirAll(pluginMetaDir, 0o755); err != nil {
		return fmt.Errorf("connect: create plugin dir: %w", err)
	}

	// Write plugin.json
	pluginJSON := map[string]any{
		"name":        pluginName,
		"description": "Claude CLI proxy — manage AI agents with persistent sessions over your Claude subscription.",
		"version":     "0.1.0",
		"author":      map[string]any{"name": "claudio"},
		"license":     "MIT",
	}
	pluginData, _ := json.MarshalIndent(pluginJSON, "", "  ")
	pluginData = append(pluginData, '\n')
	if err := os.WriteFile(filepath.Join(pluginMetaDir, "plugin.json"), pluginData, 0o644); err != nil {
		return fmt.Errorf("connect: write plugin.json: %w", err)
	}

	// Write .mcp.json
	mcpJSON := map[string]any{
		"mcpServers": map[string]any{
			pluginName: map[string]any{
				"command": bin,
				"args":    []any{"mcp"},
			},
		},
	}
	mcpData, _ := json.MarshalIndent(mcpJSON, "", "  ")
	mcpData = append(mcpData, '\n')
	if err := os.WriteFile(filepath.Join(pdir, ".mcp.json"), mcpData, 0o644); err != nil {
		return fmt.Errorf("connect: write .mcp.json: %w", err)
	}

	// Enable plugin in settings.json
	settingsPath, err := configPath(TargetClaudeCode)
	if err != nil {
		return err
	}
	cfg, err := readConfig(settingsPath)
	if err != nil {
		return err
	}

	// Add to enabledPlugins
	enabledRaw, ok := cfg["enabledPlugins"]
	if !ok {
		enabledRaw = map[string]any{}
	}
	enabled, ok := enabledRaw.(map[string]any)
	if !ok {
		enabled = map[string]any{}
	}
	enabled[pluginName+"@"+pluginName] = true
	cfg["enabledPlugins"] = enabled

	// Add to extraKnownMarketplaces
	mktsRaw, ok := cfg["extraKnownMarketplaces"]
	if !ok {
		mktsRaw = map[string]any{}
	}
	mkts, ok := mktsRaw.(map[string]any)
	if !ok {
		mkts = map[string]any{}
	}
	mkts[pluginName] = map[string]any{
		"source": map[string]any{
			"repo":   "bsantosio/claudio",
			"source": "github",
		},
	}
	cfg["extraKnownMarketplaces"] = mkts

	// Remove from mcpServers if previously added there (cleanup old approach)
	if mcpRaw, ok := cfg["mcpServers"]; ok {
		if mcp, ok := mcpRaw.(map[string]any); ok {
			delete(mcp, pluginName)
			cfg["mcpServers"] = mcp
		}
	}

	return writeConfigAtomic(settingsPath, cfg)
}

func connectClaudeDesktop(bin string) error {
	path, err := configPath(TargetClaudeDesktop)
	if err != nil {
		return err
	}

	cfg, err := readConfig(path)
	if err != nil {
		return err
	}

	mcpRaw, ok := cfg["mcpServers"]
	if !ok {
		mcpRaw = map[string]any{}
	}
	mcp, ok := mcpRaw.(map[string]any)
	if !ok {
		return fmt.Errorf("connect: unexpected type for mcpServers in %s", path)
	}

	mcp[pluginName] = map[string]any{
		"command": bin,
		"args":    []any{"mcp"},
	}
	cfg["mcpServers"] = mcp

	return writeConfigAtomic(path, cfg)
}

// Disconnect removes claudio from the target application. Idempotent.
func Disconnect(target string) error {
	mu.Lock()
	defer mu.Unlock()

	switch target {
	case TargetClaudeCode:
		return disconnectClaudeCode()
	case TargetClaudeDesktop:
		return disconnectClaudeDesktop()
	default:
		return fmt.Errorf("connect: unknown target %q", target)
	}
}

func disconnectClaudeCode() error {
	// Remove plugin directory
	pdir, err := pluginDir()
	if err != nil {
		return err
	}
	// Go up to the marketplace root: plugins/marketplaces/claudio/
	marketplaceDir := filepath.Dir(filepath.Dir(filepath.Dir(pdir)))
	if err := os.RemoveAll(marketplaceDir); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("connect: remove plugin dir: %w", err)
	}

	// Remove from enabledPlugins and extraKnownMarketplaces in settings.json
	settingsPath, err := configPath(TargetClaudeCode)
	if err != nil {
		return err
	}
	cfg, err := readConfig(settingsPath)
	if err != nil {
		return err
	}
	if len(cfg) == 0 {
		return nil
	}

	if enabledRaw, ok := cfg["enabledPlugins"]; ok {
		if enabled, ok := enabledRaw.(map[string]any); ok {
			delete(enabled, pluginName+"@"+pluginName)
			cfg["enabledPlugins"] = enabled
		}
	}
	if mktsRaw, ok := cfg["extraKnownMarketplaces"]; ok {
		if mkts, ok := mktsRaw.(map[string]any); ok {
			delete(mkts, pluginName)
			cfg["extraKnownMarketplaces"] = mkts
		}
	}
	// Also clean up mcpServers if old approach left an entry
	if mcpRaw, ok := cfg["mcpServers"]; ok {
		if mcp, ok := mcpRaw.(map[string]any); ok {
			delete(mcp, pluginName)
			cfg["mcpServers"] = mcp
		}
	}

	return writeConfigAtomic(settingsPath, cfg)
}

func disconnectClaudeDesktop() error {
	path, err := configPath(TargetClaudeDesktop)
	if err != nil {
		return err
	}

	cfg, err := readConfig(path)
	if err != nil {
		return err
	}
	if len(cfg) == 0 {
		return nil
	}

	mcpRaw, ok := cfg["mcpServers"]
	if !ok {
		return nil
	}
	mcp, ok := mcpRaw.(map[string]any)
	if !ok {
		return nil
	}

	delete(mcp, pluginName)
	cfg["mcpServers"] = mcp

	return writeConfigAtomic(path, cfg)
}

// Status returns true if claudio is registered in the target application.
// For Claude Code, checks for the plugin .mcp.json file.
// For Claude Desktop, checks for mcpServers.claudio in the config.
func Status(target string) (bool, error) {
	switch target {
	case TargetClaudeCode:
		pdir, err := pluginDir()
		if err != nil {
			return false, err
		}
		_, err = os.Stat(filepath.Join(pdir, ".mcp.json"))
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		return true, nil
	case TargetClaudeDesktop:
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
		_, connected := mcp[pluginName]
		return connected, nil
	default:
		return false, fmt.Errorf("connect: unknown target %q", target)
	}
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
