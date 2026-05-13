package domain

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
)

var Version = "dev"

type Config struct {
	Port         string
	APIKey       string
	DefaultModel string
	WorkDir      string
	DataDir      string
	RequireAuth  bool
}

type Agent struct {
	ID              string         `json:"id"`
	Name            string         `json:"name"`
	SystemPrompt    string         `json:"system_prompt"`
	Model           string         `json:"model"`
	MaxTurns        int            `json:"max_turns,omitempty"`
	PermissionMode  string         `json:"permission_mode,omitempty"`
	AllowedTools    []string       `json:"allowed_tools,omitempty"`
	DisallowedTools []string       `json:"disallowed_tools,omitempty"`
	MCPConfig       map[string]any `json:"mcp_config,omitempty"`
	CreatedAt       string         `json:"created_at"`
	UpdatedAt       string         `json:"updated_at"`
}

var KnownModels = map[string]bool{
	"haiku":                    true,
	"sonnet":                   true,
	"opus":                     true,
	"claude-haiku-4-5":         true,
	"claude-haiku-4-5-20251001": true,
	"claude-sonnet-4-6":        true,
	"claude-opus-4-6":          true,
	"claude-opus-4-6[1m]":      true,
	"claude-opus-4-7":          true,
}

type Session struct {
	ID         string `json:"id"`
	AgentID    string `json:"agent_id"`
	Name       string `json:"name,omitempty"`
	TurnCount  int    `json:"turn_count"`
	CreatedAt  string `json:"created_at"`
	LastActive string `json:"last_active,omitempty"`
}

type Message struct {
	SessionID string `json:"session_id,omitempty"`
	Role      string `json:"role"`
	Content   string `json:"content"`
}

func NewUUID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("uuid generation failed: %w", err)
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80

	return fmt.Sprintf("%s-%s-%s-%s-%s",
		hex.EncodeToString(b[0:4]),
		hex.EncodeToString(b[4:6]),
		hex.EncodeToString(b[6:8]),
		hex.EncodeToString(b[8:10]),
		hex.EncodeToString(b[10:16]),
	), nil
}

// SessionMutexMap manages per-session mutexes for concurrent request serialization.
type SessionMutexMap struct {
	m sync.Map
}

func NewSessionMutexMap() *SessionMutexMap {
	return &SessionMutexMap{}
}

func (sm *SessionMutexMap) Get(sessionID string) *sync.Mutex {
	actual, _ := sm.m.LoadOrStore(sessionID, &sync.Mutex{})
	return actual.(*sync.Mutex)
}

func (sm *SessionMutexMap) Delete(sessionID string) {
	sm.m.Delete(sessionID)
}
