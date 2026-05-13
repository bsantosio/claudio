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
	"time"
)

var timeNow = time.Now

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

const pluginVersion = "0.1.0"

// pluginCacheDir returns the path to claudio's plugin cache directory.
func pluginCacheDir() (string, error) {
	ch, err := claudeHome()
	if err != nil {
		return "", err
	}
	return filepath.Join(ch, "plugins", "cache", pluginName, pluginName, pluginVersion), nil
}

// installedPluginsPath returns the path to installed_plugins.json.
func installedPluginsPath() (string, error) {
	ch, err := claudeHome()
	if err != nil {
		return "", err
	}
	return filepath.Join(ch, "plugins", "installed_plugins.json"), nil
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
	cacheDir, err := pluginCacheDir()
	if err != nil {
		return err
	}

	// Create plugin cache directory structure
	pluginMetaDir := filepath.Join(cacheDir, ".claude-plugin")
	if err := os.MkdirAll(pluginMetaDir, 0o755); err != nil {
		return fmt.Errorf("connect: create plugin dir: %w", err)
	}

	// Write .claude-plugin/plugin.json
	pluginJSON := map[string]any{
		"name":        pluginName,
		"description": "Claude CLI proxy — manage AI agents with persistent sessions over your Claude subscription.",
		"version":     pluginVersion,
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
	if err := os.WriteFile(filepath.Join(cacheDir, ".mcp.json"), mcpData, 0o644); err != nil {
		return fmt.Errorf("connect: write .mcp.json: %w", err)
	}

	// Register in installed_plugins.json
	ipPath, err := installedPluginsPath()
	if err != nil {
		return err
	}
	ipData, err := os.ReadFile(ipPath)
	var ip map[string]any
	if err != nil {
		ip = map[string]any{"version": float64(2), "plugins": map[string]any{}}
	} else {
		if err := json.Unmarshal(ipData, &ip); err != nil {
			ip = map[string]any{"version": float64(2), "plugins": map[string]any{}}
		}
	}
	pluginsRaw, _ := ip["plugins"].(map[string]any)
	if pluginsRaw == nil {
		pluginsRaw = map[string]any{}
	}
	pluginKey := pluginName + "@" + pluginName
	now := fmt.Sprintf("%04d-%02d-%02dT%02d:%02d:%02d.000Z",
		timeNow().Year(), timeNow().Month(), timeNow().Day(),
		timeNow().Hour(), timeNow().Minute(), timeNow().Second())
	pluginsRaw[pluginKey] = []any{
		map[string]any{
			"scope":       "user",
			"installPath": cacheDir,
			"version":     pluginVersion,
			"installedAt": now,
			"lastUpdated": now,
		},
	}
	ip["plugins"] = pluginsRaw
	if err := writeConfigAtomic(ipPath, ip); err != nil {
		return fmt.Errorf("connect: update installed_plugins.json: %w", err)
	}

	// Enable in settings.json
	settingsPath, err := configPath(TargetClaudeCode)
	if err != nil {
		return err
	}
	cfg, err := readConfig(settingsPath)
	if err != nil {
		return err
	}

	enabledRaw, _ := cfg["enabledPlugins"].(map[string]any)
	if enabledRaw == nil {
		enabledRaw = map[string]any{}
	}
	enabledRaw[pluginKey] = true
	cfg["enabledPlugins"] = enabledRaw

	// Cleanup old mcpServers entry if present
	if mcpRaw, ok := cfg["mcpServers"].(map[string]any); ok {
		delete(mcpRaw, pluginName)
		cfg["mcpServers"] = mcpRaw
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
	// Remove plugin cache directory
	cacheDir, err := pluginCacheDir()
	if err != nil {
		return err
	}
	// Remove the claudio dir under cache: cache/claudio/
	claudioCacheRoot := filepath.Dir(filepath.Dir(cacheDir))
	if err := os.RemoveAll(claudioCacheRoot); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("connect: remove plugin cache: %w", err)
	}

	// Also remove old marketplace dir if it exists
	ch, err := claudeHome()
	if err != nil {
		return err
	}
	oldMarketplaceDir := filepath.Join(ch, "plugins", "marketplaces", pluginName)
	os.RemoveAll(oldMarketplaceDir) //nolint:errcheck — best-effort cleanup

	// Remove from installed_plugins.json
	pluginKey := pluginName + "@" + pluginName
	ipPath, err := installedPluginsPath()
	if err != nil {
		return err
	}
	ipData, err := os.ReadFile(ipPath)
	if err == nil {
		var ip map[string]any
		if json.Unmarshal(ipData, &ip) == nil {
			if plugins, ok := ip["plugins"].(map[string]any); ok {
				delete(plugins, pluginKey)
				ip["plugins"] = plugins
				writeConfigAtomic(ipPath, ip) //nolint:errcheck
			}
		}
	}

	// Remove from enabledPlugins in settings.json
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

	if enabled, ok := cfg["enabledPlugins"].(map[string]any); ok {
		delete(enabled, pluginKey)
		cfg["enabledPlugins"] = enabled
	}
	if mcpRaw, ok := cfg["mcpServers"].(map[string]any); ok {
		delete(mcpRaw, pluginName)
		cfg["mcpServers"] = mcpRaw
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
		cacheDir, err := pluginCacheDir()
		if err != nil {
			return false, err
		}
		_, err = os.Stat(filepath.Join(cacheDir, ".mcp.json"))
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
