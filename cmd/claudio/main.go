package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"claudio/internal/domain"
	"claudio/internal/mcp"
	"claudio/internal/server"
	"claudio/internal/service"
	"claudio/internal/store"
	"claudio/internal/tui"
)

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func loadConfig() domain.Config {
	return domain.Config{
		Port:         getenv("PORT", "18080"),
		APIKey:       os.Getenv("ADAPTER_API_KEY"),
		DefaultModel: getenv("DEFAULT_MODEL", "sonnet"),
		WorkDir:      getenv("WORK_DIR", "."),
		DataDir:      getenv("DATA_DIR", "./data"),
	}
}

func validateConfig(cfg domain.Config) {
	if !domain.KnownModels[cfg.DefaultModel] {
		log.Fatalf("DEFAULT_MODEL %q is not a recognized model", cfg.DefaultModel)
	}
}

func checkAuth(fatal bool) {
	authStatus, _ := server.CheckClaudeAuth()
	if authStatus != "authenticated" {
		if fatal {
			log.Fatalf("Claude CLI is not authenticated. Run: claude auth login")
		}
		log.Printf("WARNING: Claude CLI is not authenticated. Running in degraded mode.")
	}
}

func openStore(cfg domain.Config) *store.Store {
	if err := os.MkdirAll(cfg.DataDir, 0755); err != nil {
		log.Fatalf("failed to create data dir %s: %v", cfg.DataDir, err)
	}
	dbPath := filepath.Join(cfg.DataDir, "adapter.db")
	st, err := store.NewStore(dbPath)
	if err != nil {
		log.Fatalf("failed to open store: %v", err)
	}
	return st
}

func openBrowser(url string) {
	var cmd string
	switch runtime.GOOS {
	case "darwin":
		cmd = "open"
	case "linux":
		cmd = "xdg-open"
	case "windows":
		cmd = "start"
	default:
		return
	}
	exec.Command(cmd, url).Start()
}

func isPortOpen(port string) bool {
	conn, err := net.DialTimeout("tcp", "localhost:"+port, 500*time.Millisecond)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

func main() {
	subcmd := ""
	if len(os.Args) > 1 {
		subcmd = os.Args[1]
	}

	switch subcmd {
	case "web":
		cmdWeb()
	case "mcp":
		cmdMCP()
	case "prompt":
		cmdPrompt()
	case "service":
		cmdService()
	case "version":
		fmt.Printf("claudio %s\n", domain.Version)
	case "help", "--help", "-h":
		printUsage()
	default:
		cmdTUI()
	}
}

func cmdTUI() {
	cfg := loadConfig()
	validateConfig(cfg)
	checkAuth(true)
	st := openStore(cfg)
	defer st.Close()

	p := tea.NewProgram(tui.NewModel(cfg, st), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		log.Fatalf("tui error: %v", err)
	}
}

func cmdWeb() {
	cfg := loadConfig()
	validateConfig(cfg)
	url := fmt.Sprintf("http://localhost:%s", cfg.Port)

	if isPortOpen(cfg.Port) {
		fmt.Printf("claudio already running at %s — opening browser\n", url)
		openBrowser(url)
		return
	}

	checkAuth(false)
	st := openStore(cfg)
	defer st.Close()

	mux := server.BuildMux(cfg, st)
	addr := fmt.Sprintf(":%s", cfg.Port)
	srv := &http.Server{Addr: addr, Handler: mux}

	log.Printf("claudio v%s — web UI at %s", domain.Version, url)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGTERM, syscall.SIGINT)

	go func() {
		<-quit
		log.Printf("shutting down...")
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		srv.Shutdown(ctx)
	}()

	go func() {
		time.Sleep(300 * time.Millisecond)
		openBrowser(url)
	}()

	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server error: %v", err)
	}
}

func cmdMCP() {
	cfg := loadConfig()
	checkAuth(true)
	st := openStore(cfg)
	defer st.Close()

	log.SetOutput(os.Stderr)
	log.Printf("claudio v%s — MCP server (stdio)", domain.Version)
	if err := mcp.RunMCPServer(cfg, st, os.Stdin, os.Stdout); err != nil {
		log.Fatalf("mcp server error: %v", err)
	}
}

func cmdService() {
	action := ""
	if len(os.Args) > 2 {
		action = os.Args[2]
	}

	cfg := loadConfig()

	switch action {
	case "install":
		checkAuth(false)
		if err := service.Install(cfg.Port); err != nil {
			log.Fatalf("install failed: %v", err)
		}
		fmt.Printf("claudio service installed — API available at http://localhost:%s\n", cfg.Port)
		fmt.Println("The service starts automatically at login and restarts on crash.")

	case "uninstall":
		if err := service.Uninstall(); err != nil {
			log.Fatalf("uninstall failed: %v", err)
		}
		fmt.Println("claudio service removed.")

	case "status":
		running, pid, err := service.Status()
		if err != nil {
			log.Fatalf("status check failed: %v", err)
		}
		if running {
			fmt.Printf("claudio service is running (PID %s) — http://localhost:%s\n", pid, cfg.Port)
		} else {
			fmt.Println("claudio service is not running.")
		}

	default:
		fmt.Print(`Usage:
  claudio service install    Install and start background service
  claudio service uninstall  Stop and remove background service
  claudio service status     Check if service is running
`)
	}
}

