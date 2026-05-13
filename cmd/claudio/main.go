package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"claudio/internal/domain"
	"claudio/internal/mcp"
	"claudio/internal/server"
	"claudio/internal/store"
	"claudio/internal/tui"
)

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func main() {
	cfg := domain.Config{
		Port:         getenv("PORT", "8080"),
		APIKey:       os.Getenv("ADAPTER_API_KEY"),
		DefaultModel: getenv("DEFAULT_MODEL", "sonnet"),
		WorkDir:      getenv("WORK_DIR", "."),
		DataDir:      getenv("DATA_DIR", "./data"),
	}

	requireAuth := flag.Bool("require-auth", true, "exit if Claude CLI is not authenticated")
	mcpMode := flag.Bool("mcp", false, "run as MCP server over stdio instead of HTTP")
	tuiMode := flag.Bool("tui", false, "run interactive TUI")
	flag.Parse()
	cfg.RequireAuth = *requireAuth

	authStatus, _ := server.CheckClaudeAuth()
	if authStatus != "authenticated" {
		msg := "Claude CLI is not authenticated. Run: claude auth login"
		if cfg.RequireAuth {
			log.Fatalf("FATAL: %s", msg)
		} else {
			log.Printf("WARNING: %s (running in degraded mode)", msg)
		}
	}

	if err := os.MkdirAll(cfg.DataDir, 0755); err != nil {
		log.Fatalf("failed to create data dir %s: %v", cfg.DataDir, err)
	}

	dbPath := filepath.Join(cfg.DataDir, "adapter.db")
	st, err := store.NewStore(dbPath)
	if err != nil {
		log.Fatalf("failed to open store: %v", err)
	}
	defer st.Close()

	if *tuiMode {
		p := tea.NewProgram(tui.NewModel(cfg, st), tea.WithAltScreen())
		if _, err := p.Run(); err != nil {
			log.Fatalf("tui error: %v", err)
		}
		return
	}

	if *mcpMode {
		log.SetOutput(os.Stderr)
		log.Printf("claudio v%s — MCP server mode (stdio)", domain.Version)
		if err := mcp.RunMCPServer(cfg, st, os.Stdin, os.Stdout); err != nil {
			log.Fatalf("mcp server error: %v", err)
		}
		return
	}

	mux := server.BuildMux(cfg, st)

	addr := fmt.Sprintf(":%s", cfg.Port)
	srv := &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	log.Printf("claudio v%s — routes: /health, /agents, /sessions, /v1/models, /v1/chat/completions", domain.Version)
	log.Printf("claudio v%s listening on %s", domain.Version, addr)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGTERM, syscall.SIGINT)

	go func() {
		<-quit
		log.Printf("shutting down gracefully...")
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			log.Fatalf("forced shutdown: %v", err)
		}
	}()

	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server error: %v", err)
	}
	log.Printf("claudio stopped")
}
