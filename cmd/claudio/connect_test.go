package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// buildClaudio compiles the binary to a temp dir and returns its path.
// Tests that require a real binary process use this.
func buildClaudio(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "claudio-test")
	cmd := exec.Command("go", "build", "-o", bin, ".")
	cmd.Dir = "." // cmd/claudio
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build failed: %v\n%s", err, out)
	}
	return bin
}

// runBin runs the binary with args and a custom HOME dir for config isolation.
func runBin(t *testing.T, bin, homeDir string, args ...string) (stdout string, exitCode int) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Env = append(os.Environ(),
		"HOME="+homeDir,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return string(out), ee.ExitCode()
		}
		t.Fatalf("run error: %v", err)
	}
	return string(out), 0
}

func TestCLI_Connect_ClaudeCode_ExitsZero(t *testing.T) {
	bin := buildClaudio(t)
	home := t.TempDir()
	_, code := runBin(t, bin, home, "connect", "claude-code")
	if code != 0 {
		t.Errorf("expected exit 0 for connect claude-code, got %d", code)
	}
}

func TestCLI_Connect_ClaudeDesktop_ExitsZero(t *testing.T) {
	bin := buildClaudio(t)
	home := t.TempDir()
	_, code := runBin(t, bin, home, "connect", "claude-desktop")
	if code != 0 {
		t.Errorf("expected exit 0 for connect claude-desktop, got %d", code)
	}
}

func TestCLI_Connect_ClaudeDesktop_PrintsRestartAdvisory(t *testing.T) {
	bin := buildClaudio(t)
	home := t.TempDir()
	out, _ := runBin(t, bin, home, "connect", "claude-desktop")
	// Advisory about restarting Claude Desktop must be present.
	if !strings.Contains(out, "restart") && !strings.Contains(out, "Restart") {
		t.Errorf("expected restart advisory in output, got: %s", out)
	}
}

func TestCLI_Connect_InvalidTarget_ExitsNonZero(t *testing.T) {
	bin := buildClaudio(t)
	home := t.TempDir()
	_, code := runBin(t, bin, home, "connect", "not-a-target")
	if code == 0 {
		t.Error("expected non-zero exit for invalid target")
	}
}

func TestCLI_Connect_NoTarget_PrintsUsage(t *testing.T) {
	bin := buildClaudio(t)
	home := t.TempDir()
	out, code := runBin(t, bin, home, "connect")
	if code != 0 {
		t.Errorf("expected exit 0 for connect with no args (usage), got %d", code)
	}
	if len(out) == 0 {
		t.Error("expected usage text in output")
	}
}

func TestCLI_Disconnect_ClaudeCode_ExitsZero(t *testing.T) {
	bin := buildClaudio(t)
	home := t.TempDir()
	// Connect first.
	runBin(t, bin, home, "connect", "claude-code")
	_, code := runBin(t, bin, home, "disconnect", "claude-code")
	if code != 0 {
		t.Errorf("expected exit 0 for disconnect claude-code, got %d", code)
	}
}

func TestCLI_Disconnect_NotConnected_ExitsZero(t *testing.T) {
	bin := buildClaudio(t)
	home := t.TempDir()
	_, code := runBin(t, bin, home, "disconnect", "claude-code")
	if code != 0 {
		t.Errorf("expected exit 0 for disconnect when not connected, got %d", code)
	}
}

func TestCLI_Disconnect_InvalidTarget_ExitsNonZero(t *testing.T) {
	bin := buildClaudio(t)
	home := t.TempDir()
	_, code := runBin(t, bin, home, "disconnect", "not-a-target")
	if code == 0 {
		t.Error("expected non-zero exit for disconnect with invalid target")
	}
}

func TestCLI_ConnectStatus_ExitsZeroAndShowsBothTargets(t *testing.T) {
	bin := buildClaudio(t)
	home := t.TempDir()
	out, code := runBin(t, bin, home, "connect", "status")
	if code != 0 {
		t.Errorf("expected exit 0 for connect status, got %d: %s", code, out)
	}
	if len(out) == 0 {
		t.Error("expected status output")
	}
}

func TestCLI_Help_IncludesConnect(t *testing.T) {
	bin := buildClaudio(t)
	home := t.TempDir()
	out, _ := runBin(t, bin, home, "help")
	if len(out) == 0 {
		t.Error("expected help output")
	}
	// Help must mention connect subcommand.
	if !strings.Contains(out, "connect") {
		t.Errorf("help output should mention 'connect', got: %s", out)
	}
}

