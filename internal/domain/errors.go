// Package domain defines the core types, errors, and business rules for claudio.
package domain

import "errors"

var (
	ErrNotFound          = errors.New("not found")
	ErrHasActiveSessions = errors.New("agent has active sessions")
	ErrSessionBusy       = errors.New("session has an in-flight request")
	ErrNameRequired      = errors.New("name is required")
	ErrPromptRequired    = errors.New("system_prompt is required")
	ErrUnknownModel      = errors.New("model not recognized")
	ErrDescRequired      = errors.New("description is required")
)
