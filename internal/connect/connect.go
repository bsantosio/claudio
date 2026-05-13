// Package connect manages claudio's registration as an MCP server
// in Claude Code and Claude Desktop configuration files.
package connect

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

// Target identifiers for supported Claude products.
const (
	TargetClaudeCode    = "claude-code"
	TargetClaudeDesktop = "claude-desktop"
)

var mu sync.Mutex

const pluginName = "claudio"

// configPathOverride is set only during testing (via SetConfigPathOverride).
var configPathOverride string

// SetConfigPathOverride sets the test override directory for config file paths.
func SetConfigPathOverride(dir string) {
	configPathOverride = dir
}

// runClaudeMCP executes a `claude mcp` subcommand. Overridable for testing.
var runClaudeMCP = defaultRunClaudeMCP

func defaultRunClaudeMCP(args ...string) ([]byte, error) {
	cmd := exec.Command("claude", append([]string{"mcp"}, args...)...)
	return cmd.CombinedOutput()
}

// findBinary returns the absolute path of the running executable.
func findBinary() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("connect: cannot determine executable path: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(exe)
	if err != nil {
		resolved = exe
	}
	if strings.Contains(resolved, "go-build") {
		fmt.Fprintf(os.Stderr, "warning: claudio binary path %q looks like a temporary go build artifact; MCP server path may not persist\n", resolved)
	}
	return resolved, nil
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
	case TargetClaudeDesktop:
		return filepath.Join(home, "Library", "Application Support", "Claude", "claude_desktop_config.json"), nil
	default:
		return "", fmt.Errorf("connect: unknown target %q", target)
	}
}

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
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("connect: write temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("connect: close temp file: %w", err)
	}
	if err := os.Chmod(tmpPath, perm); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("connect: chmod temp file: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("connect: rename config file: %w", err)
	}
	return nil
}

// Connect registers claudio as an MCP server in the target application.
// For Claude Code, uses `claude mcp add` (the official CLI).
// For Claude Desktop, edits the config file directly.
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
	// Remove first to ensure idempotency (ignore errors — might not exist)
	runClaudeMCP("remove", "-s", "user", pluginName) //nolint:errcheck

	out, err := runClaudeMCP("add", "-s", "user", pluginName, "--", bin, "mcp")
	if err != nil {
		return fmt.Errorf("connect: claude mcp add failed: %w\noutput: %s", err, string(out))
	}
	return nil
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
	out, err := runClaudeMCP("remove", "-s", "user", pluginName)
	if err != nil {
		return fmt.Errorf("connect: claude mcp remove failed: %w\noutput: %s", err, string(out))
	}
	return nil
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

	mcp, ok := cfg["mcpServers"].(map[string]any)
	if !ok {
		return nil
	}

	delete(mcp, pluginName)
	cfg["mcpServers"] = mcp

	return writeConfigAtomic(path, cfg)
}

// Status returns true if claudio is registered in the target application.
func Status(target string) (bool, error) {
	switch target {
	case TargetClaudeCode:
		out, err := runClaudeMCP("get", pluginName)
		if err != nil {
			return false, nil
		}
		return len(out) > 0 && !strings.Contains(string(out), "not found"), nil
	case TargetClaudeDesktop:
		path, err := configPath(target)
		if err != nil {
			return false, err
		}
		cfg, err := readConfig(path)
		if err != nil {
			return false, err
		}
		mcp, ok := cfg["mcpServers"].(map[string]any)
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
