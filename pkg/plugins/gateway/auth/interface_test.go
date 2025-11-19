package auth

import (
	"context"
	"testing"
	"time"
)

// fake authenticator that satisfies Authenticator
type fakeAuthenticator struct {
	res AuthResult
	err error
}

func (f *fakeAuthenticator) Authenticate(ctx context.Context, authHeader string) (AuthResult, error) {
	return f.res, f.err
}

// fake key manager that satisfies KeyManager
type fakeKeyManager struct {
	store map[string]KeyRecord
}

func newFakeKeyManager() *fakeKeyManager {
	return &fakeKeyManager{store: make(map[string]KeyRecord)}
}

func (f *fakeKeyManager) GetByAPIKey(ctx context.Context, apiKey string) (KeyRecord, error) {
	if rec, ok := f.store[apiKey]; ok {
		// enforce expiry semantics if present
		if rec.ExpiresAt != nil && time.Now().After(*rec.ExpiresAt) {
			return KeyRecord{}, ErrUnauthorized
		}
		return rec, nil
	}
	return KeyRecord{}, ErrUnauthorized
}

func (f *fakeKeyManager) CreateKey(ctx context.Context, rec KeyRecord) error {
	if rec.APIKeyID == "" {
		return ErrUnauthorized
	}
	f.store[rec.APIKeyID] = rec
	return nil
}

func (f *fakeKeyManager) RevokeKey(ctx context.Context, apiKey string) error {
	delete(f.store, apiKey)
	return nil
}

func TestAuthenticatorInterfaceAndBehavior(t *testing.T) {
	// compile-time check
	var _ Authenticator = (*fakeAuthenticator)(nil)

	ctx := context.Background()
	fa := &fakeAuthenticator{res: AuthResult{Valid: true}, err: nil}

	res, err := fa.Authenticate(ctx, "Bearer token")
	if err != nil {
		t.Fatalf("unexpected error from Authenticate: %v", err)
	}
	if !res.Valid {
		t.Fatalf("expected Valid=true, got %+v", res)
	}
}

func TestKeyManagerInterfaceAndBehavior(t *testing.T) {
	// compile-time check
	var _ KeyManager = (*fakeKeyManager)(nil)

	ctx := context.Background()
	km := newFakeKeyManager()

	rec := KeyRecord{
		APIKeyID: "test-key-1",
		// other fields may be zero; presence of APIKeyID is sufficient for this test
	}

	// Create
	if err := km.CreateKey(ctx, rec); err != nil {
		t.Fatalf("CreateKey failed: %v", err)
	}

	// Get exists
	got, err := km.GetByAPIKey(ctx, "test-key-1")
	if err != nil {
		t.Fatalf("GetByAPIKey failed: %v", err)
	}
	if got.APIKeyID != rec.APIKeyID {
		t.Fatalf("GetByAPIKey returned wrong record: want %q got %q", rec.APIKeyID, got.APIKeyID)
	}

	// Revoke and ensure unauthorized afterwards
	if err := km.RevokeKey(ctx, "test-key-1"); err != nil {
		t.Fatalf("RevokeKey failed: %v", err)
	}
	if _, err := km.GetByAPIKey(ctx, "test-key-1"); err != ErrUnauthorized {
		t.Fatalf("expected ErrUnauthorized after revoke, got: %v", err)
	}

	// expiry behavior
	exp := time.Now().Add(-time.Hour)
	expRec := KeyRecord{APIKeyID: "expired", ExpiresAt: &exp}
	if err := km.CreateKey(ctx, expRec); err != nil {
		t.Fatalf("CreateKey for expired record failed: %v", err)
	}
	if _, err := km.GetByAPIKey(ctx, "expired"); err != ErrUnauthorized {
		t.Fatalf("expected ErrUnauthorized for expired key, got: %v", err)
	}
}
