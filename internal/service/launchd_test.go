package service

import (
	"strings"
	"testing"
)

func TestGeneratePlist_ContainsCorrectFields(t *testing.T) {
	binaryPath := "/usr/local/bin/claudio"
	port := "8080"

	plist, err := generatePlist(binaryPath, port)
	if err != nil {
		t.Fatalf("generatePlist returned unexpected error: %v", err)
	}

	checks := []struct {
		name    string
		contain string
	}{
		{"label", "io.bsantos.claudio"},
		{"binary path", binaryPath},
		{"port", port},
		{"RunAtLoad true", "<key>RunAtLoad</key>\n    <true/>"},
		{"KeepAlive true", "<key>KeepAlive</key>\n    <true/>"},
		{"PATH homebrew", "/opt/homebrew/bin"},
		{"DATA_DIR claudio data", ".claudio/data"},
		{"web argument", "<string>web</string>"},
	}

	for _, c := range checks {
		t.Run(c.name, func(t *testing.T) {
			if !strings.Contains(plist, c.contain) {
				t.Errorf("plist does not contain %q\ngot:\n%s", c.contain, plist)
			}
		})
	}
}

func TestGeneratePlist_CustomPort(t *testing.T) {
	plist, err := generatePlist("/usr/local/bin/claudio", "9999")
	if err != nil {
		t.Fatalf("generatePlist returned unexpected error: %v", err)
	}

	if !strings.Contains(plist, "9999") {
		t.Errorf("plist does not contain custom port 9999\ngot:\n%s", plist)
	}
}

func TestPlistPath_InUserHome(t *testing.T) {
	path, err := plistPath()
	if err != nil {
		t.Fatalf("plistPath returned unexpected error: %v", err)
	}

	suffix := "Library/LaunchAgents/io.bsantos.claudio.plist"
	if !strings.HasSuffix(path, suffix) {
		t.Errorf("plistPath = %q, want suffix %q", path, suffix)
	}
}

// TestStatus_NotInstalled verifies that Status never returns an error and that
// the (running, pid) pair is internally consistent: if running is false then
// pid must be empty, and if running is true then pid must be non-empty.
func TestStatus_NotInstalled(t *testing.T) {
	running, pid, err := Status()
	if err != nil {
		t.Fatalf("Status returned unexpected error: %v", err)
	}

	// running=false  →  pid must be empty
	if !running && pid != "" {
		t.Errorf("Status returned running=false but pid=%q; expected empty pid", pid)
	}

	// running=true  →  pid must be non-empty
	if running && pid == "" {
		t.Error("Status returned running=true but pid is empty; expected a PID")
	}
}
