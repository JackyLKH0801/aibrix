package auth

import (
	"context"
	"encoding/json"
	"mime"
	"net/http"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

type ServerConfig struct {
	Namespace string
	JWKSURL   string
	// FailClosed is kept for backward compatibility. If true, auth failures close the request path.
	FailClosed bool
	// FailOpen explicitly sets fail-open behavior when true. If unset, FailClosed is used to infer behavior.
	FailOpen bool
}

type Server struct {
	km         KeyManager
	jwks       *JWKSManager
	httpServer *http.Server
	cfg        ServerConfig
}

func (s *Server) failOpen() bool {
	// Prefer explicit FailOpen when set; otherwise infer from FailClosed for backward compatibility.
	if s.cfg.FailOpen {
		return true
	}
	return !s.cfg.FailClosed
}

func normalizeAuth(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return s
	}
	lower := strings.ToLower(s)
	if strings.HasPrefix(lower, "bearer ") {
		return strings.TrimSpace(s[7:])
	}
	return s
}

// NewServer constructs an auth Server which manages API keys in Redis and optionally JWKS.
func NewServer(redisClient *redis.Client, cfg ServerConfig) *Server {
	km := NewRedisKeyManager(redisClient, cfg.Namespace)
	jwks := NewJWKSManager(cfg.JWKSURL, 5*time.Minute)
	_ = jwks.Start(context.Background())

	return &Server{km: km, jwks: jwks, cfg: cfg}
}

// Start starts the HTTP server on the given address (e.g. 127.0.0.1:8081).
func (s *Server) Start(addr string) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/validate", s.ValidateHTTP)
	mux.HandleFunc("/admin/create", s.AdminCreateKeyHTTP)
	mux.HandleFunc("/admin/revoke", s.AdminRevokeKeyHTTP)

	s.httpServer = &http.Server{Addr: addr, Handler: mux}
	go s.httpServer.ListenAndServe()
	return nil
}

// Shutdown gracefully stops the HTTP server.
func (s *Server) Shutdown(ctx context.Context) error {
	if s.httpServer == nil {
		return nil
	}
	return s.httpServer.Shutdown(ctx)
}

// ValidateHTTP validates the Authorization header and returns JSON AuthResult.
func (s *Server) ValidateHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req struct {
		Authorization string `json:"authorization"`
	}
	if ct := r.Header.Get("Content-Type"); ct != "" {
		if mediaType, _, err := mime.ParseMediaType(ct); err == nil && mediaType == "application/json" {
			_ = json.NewDecoder(r.Body).Decode(&req)
		} else if strings.HasPrefix(strings.ToLower(ct), "application/json") {
			// fallback tolerant check for values like "application/json; charset=utf-8"
			_ = json.NewDecoder(r.Body).Decode(&req)
		}
	}
	if req.Authorization == "" {
		req.Authorization = r.Header.Get("Authorization")
	}
	req.Authorization = normalizeAuth(req.Authorization)
	if req.Authorization == "" {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(AuthResult{Valid: false, Error: "missing authorization"})
		return
	}
	rec, err := s.km.GetByAPIKey(ctx, req.Authorization)
	if err == nil {
		res := AuthResult{Valid: true, TenantID: rec.TenantID, Claims: map[string]interface{}{"api_key_id": rec.APIKeyID}}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(res)
		return
	}
	if s.failOpen() {
		// fail-open
		_ = json.NewEncoder(w).Encode(AuthResult{Valid: true, Error: "fail-open"})
		return
	}
	w.WriteHeader(http.StatusUnauthorized)
	_ = json.NewEncoder(w).Encode(AuthResult{Valid: false, Error: "unauthorized"})
}

// AdminCreateKeyHTTP accepts a JSON KeyRecord and stores it in Redis.
func (s *Server) AdminCreateKeyHTTP(w http.ResponseWriter, r *http.Request) {
	var rec KeyRecord
	if err := json.NewDecoder(r.Body).Decode(&rec); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if err := s.km.CreateKey(r.Context(), rec); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusCreated)
}

// AdminRevokeKeyHTTP revokes a key by APIKeyID provided in JSON body {"api_key":"..."}
func (s *Server) AdminRevokeKeyHTTP(w http.ResponseWriter, r *http.Request) {
	var req struct {
		APIKey string `json:"api_key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if err := s.km.RevokeKey(r.Context(), req.APIKey); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}
