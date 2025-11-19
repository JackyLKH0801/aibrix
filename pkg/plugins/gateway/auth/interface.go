package auth

import "context"

// Authenticator performs authentication given an Authorization header.
type Authenticator interface {
	// Authenticate accepts the full Authorization header (e.g. "Bearer <token>")
	// and returns an AuthResult. Implementations should return ErrUnauthorized for invalid credentials.
	Authenticate(ctx context.Context, authHeader string) (AuthResult, error)
}

// KeyManager manages API keys and tenant associations.
type KeyManager interface {
	// GetByAPIKey accepts the raw API key (client-provided) and returns the KeyRecord.
	// Implementations are responsible to compare hashed values as needed.
	GetByAPIKey(ctx context.Context, apiKey string) (KeyRecord, error)

	CreateKey(ctx context.Context, rec KeyRecord) error
	RevokeKey(ctx context.Context, apiKey string) error
}
