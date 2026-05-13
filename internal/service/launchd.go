package service

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const plistLabel = "io.bsantos.claudio"

func plistPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Library", "LaunchAgents", plistLabel+".plist"), nil
}

func findBinary() (string, error) {
	path, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("cannot determine executable path: %w", err)
	}
	path, err = filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}
	return path, nil
}

func logDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".claudio", "logs")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	return dir, nil
}

func generatePlist(binaryPath, port string) (string, error) {
	logs, err := logDir()
	if err != nil {
		return "", err
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dataDir := filepath.Join(home, ".claudio", "data")

	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>%s</string>
    <key>ProgramArguments</key>
    <array>
        <string>%s</string>
        <string>web</string>
    </array>
    <key>EnvironmentVariables</key>
    <dict>
        <key>PORT</key>
        <string>%s</string>
        <key>DATA_DIR</key>
        <string>%s</string>
        <key>PATH</key>
        <string>/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin</string>
    </dict>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>StandardOutPath</key>
    <string>%s</string>
    <key>StandardErrorPath</key>
    <string>%s</string>
</dict>
</plist>
`, plistLabel, binaryPath, port, dataDir,
		filepath.Join(logs, "claudio.out.log"),
		filepath.Join(logs, "claudio.err.log")), nil
}

func Install(port string) error {
	bin, err := findBinary()
	if err != nil {
		return err
	}

	plist, err := generatePlist(bin, port)
	if err != nil {
		return err
	}

	path, err := plistPath()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}

	if err := os.WriteFile(path, []byte(plist), 0644); err != nil {
		return fmt.Errorf("write plist: %w", err)
	}

	cmd := exec.Command("launchctl", "load", path)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("launchctl load: %s — %w", strings.TrimSpace(string(out)), err)
	}

	return nil
}

func Uninstall() error {
	path, err := plistPath()
	if err != nil {
		return err
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("service not installed")
	}

	cmd := exec.Command("launchctl", "unload", path)
	cmd.CombinedOutput()

	if err := os.Remove(path); err != nil {
		return fmt.Errorf("remove plist: %w", err)
	}

	return nil
}

func Status() (running bool, pid string, err error) {
	cmd := exec.Command("launchctl", "list", plistLabel)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return false, "", nil
	}

	output := string(out)
	for _, line := range strings.Split(output, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, `"PID"`) {
			parts := strings.Split(trimmed, "=")
			if len(parts) == 2 {
				p := strings.TrimSpace(parts[1])
				p = strings.TrimRight(p, ";")
				p = strings.TrimSpace(p)
				if p != "" && p != "0" {
					return true, p, nil
				}
			}
		}
	}

	return false, "", nil
}
