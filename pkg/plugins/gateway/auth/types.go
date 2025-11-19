package auth

import (
	"errors"
	"time"
)

type AuthResult struct {
	Valid    bool                   `json:"valid"`
	TenantID string                 `json:"tenant_id,omitempty"`
	UserID   string                 `json:"user_id,omitempty"`
	Claims   map[string]interface{} `json:"claims,omitempty"`
	Error    string                 `json:"error,omitempty"`
}

type KeyRecord struct {
	APIKeyID  string     `json:"api_key_id"`
	TenantID  string     `json:"tenant_id"`
	CreatedAt time.Time  `json:"created_at"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	Scopes    []string   `json:"scopes,omitempty"`
}

var (
	ErrUnauthorized = errors.New("unauthorized")
	ErrInvalidToken = errors.New("invalid token")
)
