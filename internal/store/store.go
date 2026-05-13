// Package store provides SQLite persistence for agents and sessions.
package store

import (
	"bufio"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"claudio/internal/domain"

	_ "modernc.org/sqlite"
)

type Store struct {
	db      *sql.DB
	Mutexes *domain.SessionMutexMap
}

func NewStore(dbPath string) (*Store, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	if _, err := db.Exec("PRAGMA journal_mode=WAL;"); err != nil {
		db.Close()
		return nil, fmt.Errorf("set WAL mode: %w", err)
	}
	if _, err := db.Exec("PRAGMA foreign_keys=ON;"); err != nil {
		db.Close()
		return nil, fmt.Errorf("enable foreign keys: %w", err)
	}
	s := &Store{db: db, Mutexes: domain.NewSessionMutexMap()}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return s, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) migrate() error {
	const ddl = `
CREATE TABLE IF NOT EXISTS agents (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    system_prompt TEXT NOT NULL,
    model TEXT NOT NULL,
    max_turns INTEGER DEFAULT 0,
    permission_mode TEXT DEFAULT '',
    allowed_tools TEXT DEFAULT '',
    disallowed_tools TEXT DEFAULT '',
    mcp_config TEXT DEFAULT '',
    sub_agents TEXT DEFAULT '',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS sessions (
    id TEXT PRIMARY KEY,
    agent_id TEXT NOT NULL REFERENCES agents(id),
    name TEXT DEFAULT '',
    turn_count INTEGER DEFAULT 0,
    created_at TEXT NOT NULL,
    last_active TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_sessions_agent_id ON sessions(agent_id);
`
	if _, err := s.db.Exec(ddl); err != nil {
		return err
	}
	// Migration: add sub_agents column for existing databases.
	// ALTER TABLE fails with "duplicate column" if column exists — that's expected.
	if _, err := s.db.Exec(`ALTER TABLE agents ADD COLUMN sub_agents TEXT DEFAULT ''`); err != nil {
		if !strings.Contains(err.Error(), "duplicate column") {
			return fmt.Errorf("migrate sub_agents: %w", err)
		}
	}
	return nil
}

// withTx runs fn inside a database transaction. If fn returns an error, the
// transaction is rolled back; otherwise it is committed.
func (s *Store) withTx(fn func(tx *sql.Tx) error) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	if err := fn(tx); err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit()
}

// ─── Agent methods ────────────────────────────────────────────────────────────

func (s *Store) CreateAgent(input domain.Agent, defaultModel string) (*domain.Agent, error) {
	if input.Name == "" {
		return nil, domain.ErrNameRequired
	}
	if input.SystemPrompt == "" {
		return nil, domain.ErrPromptRequired
	}
	model := input.Model
	if model == "" {
		model = defaultModel
	}
	if !domain.KnownModels[model] {
		return nil, fmt.Errorf("%w: %s", domain.ErrUnknownModel, model)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	id, err := domain.NewUUID()
	if err != nil {
		return nil, err
	}
	allowedJSON, err := json.Marshal(input.AllowedTools)
	if err != nil {
		return nil, err
	}
	disallowedJSON, err := json.Marshal(input.DisallowedTools)
	if err != nil {
		return nil, err
	}
	mcpJSON, err := json.Marshal(input.MCPConfig)
	if err != nil {
		return nil, err
	}
	subAgentsJSON, err := json.Marshal(input.SubAgents)
	if err != nil {
		return nil, err
	}
	_, err = s.db.Exec(
		`INSERT INTO agents (id, name, system_prompt, model, max_turns, permission_mode, allowed_tools, disallowed_tools, mcp_config, sub_agents, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, input.Name, input.SystemPrompt, model,
		input.MaxTurns, input.PermissionMode,
		string(allowedJSON), string(disallowedJSON), string(mcpJSON), string(subAgentsJSON),
		now, now,
	)
	if err != nil {
		return nil, fmt.Errorf("insert agent: %w", err)
	}
	return &domain.Agent{
		ID:              id,
		Name:            input.Name,
		SystemPrompt:    input.SystemPrompt,
		Model:           model,
		MaxTurns:        input.MaxTurns,
		PermissionMode:  input.PermissionMode,
		AllowedTools:    input.AllowedTools,
		DisallowedTools: input.DisallowedTools,
		SubAgents:       input.SubAgents,
		MCPConfig:       input.MCPConfig,
		CreatedAt:       now,
		UpdatedAt:       now,
	}, nil
}

func (s *Store) GetAgent(id string) (*domain.Agent, error) {
	row := s.db.QueryRow(
		`SELECT id, name, system_prompt, model, max_turns, permission_mode, allowed_tools, disallowed_tools, mcp_config, sub_agents, created_at, updated_at
		 FROM agents WHERE id = ?`, id)
	a, err := scanAgent(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get agent: %w", err)
	}
	return a, nil
}

func (s *Store) ListAgents() ([]*domain.Agent, error) {
	rows, err := s.db.Query(
		`SELECT id, name, system_prompt, model, max_turns, permission_mode, allowed_tools, disallowed_tools, mcp_config, sub_agents, created_at, updated_at
		 FROM agents ORDER BY created_at`)
	if err != nil {
		return nil, fmt.Errorf("list agents: %w", err)
	}
	defer rows.Close()
	var result []*domain.Agent
	for rows.Next() {
		a, err := scanAgent(rows)
		if err != nil {
			return nil, fmt.Errorf("scan agent: %w", err)
		}
		result = append(result, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate agents: %w", err)
	}
	if result == nil {
		result = []*domain.Agent{}
	}
	return result, nil
}

func (s *Store) UpdateAgent(id string, input domain.Agent, defaultModel string) (*domain.Agent, error) {
	var result *domain.Agent
	err := s.withTx(func(tx *sql.Tx) error {
		// Read within transaction
		row := tx.QueryRow(
			`SELECT id, name, system_prompt, model, max_turns, permission_mode, allowed_tools, disallowed_tools, mcp_config, sub_agents, created_at, updated_at
			 FROM agents WHERE id = ?`, id)
		existing, err := scanAgent(row)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return domain.ErrNotFound
			}
			return fmt.Errorf("get agent: %w", err)
		}

		// Apply updates
		if input.Name != "" {
			existing.Name = input.Name
		}
		if input.SystemPrompt != "" {
			existing.SystemPrompt = input.SystemPrompt
		}
		if input.Model != "" {
			if !domain.KnownModels[input.Model] {
				return fmt.Errorf("%w: %s", domain.ErrUnknownModel, input.Model)
			}
			existing.Model = input.Model
		}
		if input.AllowedTools != nil {
			existing.AllowedTools = input.AllowedTools
		}
		if input.DisallowedTools != nil {
			existing.DisallowedTools = input.DisallowedTools
		}
		if input.SubAgents != nil {
			existing.SubAgents = input.SubAgents
		}
		if input.MCPConfig != nil {
			existing.MCPConfig = input.MCPConfig
		}
		if input.MaxTurns != 0 {
			existing.MaxTurns = input.MaxTurns
		}
		if input.PermissionMode != "" {
			existing.PermissionMode = input.PermissionMode
		}
		existing.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)

		allowedJSON, err := json.Marshal(existing.AllowedTools)
		if err != nil {
			return fmt.Errorf("marshal allowed_tools: %w", err)
		}
		disallowedJSON, err := json.Marshal(existing.DisallowedTools)
		if err != nil {
			return fmt.Errorf("marshal disallowed_tools: %w", err)
		}
		mcpJSON, err := json.Marshal(existing.MCPConfig)
		if err != nil {
			return fmt.Errorf("marshal mcp_config: %w", err)
		}
		subAgentsJSON, err := json.Marshal(existing.SubAgents)
		if err != nil {
			return fmt.Errorf("marshal sub_agents: %w", err)
		}

		_, err = tx.Exec(
			`UPDATE agents SET name=?, system_prompt=?, model=?, max_turns=?, permission_mode=?, allowed_tools=?, disallowed_tools=?, mcp_config=?, sub_agents=?, updated_at=?
			 WHERE id=?`,
			existing.Name, existing.SystemPrompt, existing.Model,
			existing.MaxTurns, existing.PermissionMode,
			string(allowedJSON), string(disallowedJSON), string(mcpJSON), string(subAgentsJSON),
			existing.UpdatedAt, id,
		)
		if err != nil {
			return fmt.Errorf("update agent: %w", err)
		}
		result = existing
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (s *Store) DeleteAgent(id string) error {
	return s.withTx(func(tx *sql.Tx) error {
		var exists bool
		err := tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM agents WHERE id = ?)`, id).Scan(&exists)
		if err != nil {
			return fmt.Errorf("check agent: %w", err)
		}
		if !exists {
			return domain.ErrNotFound
		}

		var count int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM sessions WHERE agent_id = ?`, id).Scan(&count); err != nil {
			return fmt.Errorf("check sessions: %w", err)
		}
		if count > 0 {
			return domain.ErrHasActiveSessions
		}

		if _, err := tx.Exec(`DELETE FROM agents WHERE id = ?`, id); err != nil {
			return fmt.Errorf("delete agent: %w", err)
		}
		return nil
	})
}

type agentScanner interface {
	Scan(dest ...any) error
}

func scanAgent(row agentScanner) (*domain.Agent, error) {
	var a domain.Agent
	var allowedJSON, disallowedJSON, mcpJSON, subAgentsJSON string
	err := row.Scan(
		&a.ID, &a.Name, &a.SystemPrompt, &a.Model,
		&a.MaxTurns, &a.PermissionMode,
		&allowedJSON, &disallowedJSON, &mcpJSON, &subAgentsJSON,
		&a.CreatedAt, &a.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	if allowedJSON != "" && allowedJSON != "null" {
		if err := json.Unmarshal([]byte(allowedJSON), &a.AllowedTools); err != nil {
			return nil, fmt.Errorf("unmarshal allowed_tools: %w", err)
		}
	}
	if disallowedJSON != "" && disallowedJSON != "null" {
		if err := json.Unmarshal([]byte(disallowedJSON), &a.DisallowedTools); err != nil {
			return nil, fmt.Errorf("unmarshal disallowed_tools: %w", err)
		}
	}
	if mcpJSON != "" && mcpJSON != "null" {
		if err := json.Unmarshal([]byte(mcpJSON), &a.MCPConfig); err != nil {
			return nil, fmt.Errorf("unmarshal mcp_config: %w", err)
		}
	}
	if subAgentsJSON != "" && subAgentsJSON != "null" {
		if err := json.Unmarshal([]byte(subAgentsJSON), &a.SubAgents); err != nil {
			return nil, fmt.Errorf("unmarshal sub_agents: %w", err)
		}
	}
	return &a, nil
}

// ─── Session methods ───────────────────────────────────────────────────────────

func (s *Store) CreateSession(agentID, name string) (*domain.Session, error) {
	id, err := domain.NewUUID()
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err = s.db.Exec(
		`INSERT INTO sessions (id, agent_id, name, turn_count, created_at, last_active) VALUES (?, ?, ?, 0, ?, ?)`,
		id, agentID, name, now, now,
	)
	if err != nil {
		return nil, fmt.Errorf("insert session: %w", err)
	}
	sess := &domain.Session{
		ID:         id,
		AgentID:    agentID,
		Name:       name,
		TurnCount:  0,
		CreatedAt:  now,
		LastActive: now,
	}
	return sess, nil
}

func (s *Store) GetSession(id string) (*domain.Session, error) {
	row := s.db.QueryRow(
		`SELECT id, agent_id, name, turn_count, created_at, last_active FROM sessions WHERE id = ?`, id)
	var sess domain.Session
	err := row.Scan(&sess.ID, &sess.AgentID, &sess.Name, &sess.TurnCount, &sess.CreatedAt, &sess.LastActive)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get session: %w", err)
	}
	return &sess, nil
}

func (s *Store) ListSessionsByAgent(agentID string) ([]*domain.Session, error) {
	rows, err := s.db.Query(
		`SELECT id, agent_id, name, turn_count, created_at, last_active FROM sessions WHERE agent_id = ? ORDER BY created_at`,
		agentID)
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}
	defer rows.Close()
	var result []*domain.Session
	for rows.Next() {
		var sess domain.Session
		if err := rows.Scan(&sess.ID, &sess.AgentID, &sess.Name, &sess.TurnCount, &sess.CreatedAt, &sess.LastActive); err != nil {
			return nil, fmt.Errorf("scan session: %w", err)
		}
		result = append(result, &sess)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate sessions: %w", err)
	}
	if result == nil {
		result = []*domain.Session{}
	}
	return result, nil
}

func (s *Store) DeleteSession(id string) error {
	mu := s.Mutexes.Get(id)
	if !mu.TryLock() {
		return domain.ErrSessionBusy
	}
	defer mu.Unlock()

	err := s.withTx(func(tx *sql.Tx) error {
		var exists bool
		if err := tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM sessions WHERE id = ?)`, id).Scan(&exists); err != nil {
			return fmt.Errorf("check session: %w", err)
		}
		if !exists {
			return domain.ErrNotFound
		}
		if _, err := tx.Exec(`DELETE FROM sessions WHERE id = ?`, id); err != nil {
			return fmt.Errorf("delete session: %w", err)
		}
		return nil
	})
	if err != nil {
		return err
	}
	s.Mutexes.Delete(id)
	return nil
}

func (s *Store) SaveSession(sess *domain.Session) error {
	_, err := s.db.Exec(
		`UPDATE sessions SET turn_count=?, last_active=? WHERE id=?`,
		sess.TurnCount, sess.LastActive, sess.ID,
	)
	return err
}

// ─── Message reading from Claude CLI session files ───────────────────────────

func ReadSessionMessages(workDir, sessionID string) ([]domain.Message, error) {
	absDir, err := filepath.Abs(workDir)
	if err != nil {
		return nil, err
	}
	if resolved, err := filepath.EvalSymlinks(absDir); err == nil {
		absDir = resolved
	}
	encoded := encodePathForClaude(absDir)
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	jsonlPath := filepath.Join(home, ".claude", "projects", encoded, sessionID+".jsonl")
	f, err := os.Open(jsonlPath)
	if err != nil {
		if os.IsNotExist(err) {
			return []domain.Message{}, nil
		}
		return nil, err
	}
	defer f.Close()
	var msgs []domain.Message
	scanner := bufio.NewScanner(f)
	buf := make([]byte, 1024*1024)
	scanner.Buffer(buf, 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}
		var event struct {
			Type    string `json:"type"`
			Message struct {
				Role    string          `json:"role"`
				Content json.RawMessage `json:"content"`
			} `json:"message"`
		}
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			continue
		}
		if event.Type != "user" && event.Type != "assistant" {
			continue
		}
		text := ExtractContentText(event.Message.Content)
		if text == "" {
			continue
		}
		msgs = append(msgs, domain.Message{
			SessionID: sessionID,
			Role:      event.Message.Role,
			Content:   text,
		})
	}
	if msgs == nil {
		msgs = []domain.Message{}
	}
	return msgs, nil
}

func encodePathForClaude(absPath string) string {
	var result []byte
	for _, c := range absPath {
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') {
			result = append(result, byte(c))
		} else {
			result = append(result, '-')
		}
	}
	return string(result)
}

func ExtractContentText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var contents []struct {
		Type    string `json:"type"`
		Text    string `json:"text"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(raw, &contents); err == nil {
		var parts []string
		for _, c := range contents {
			if c.Type == "text" && c.Text != "" {
				parts = append(parts, c.Text)
			}
		}
		return strings.Join(parts, "")
	}
	return ""
}
