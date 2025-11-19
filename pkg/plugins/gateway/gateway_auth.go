package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/vllm-project/aibrix/pkg/plugins/gateway/auth"
)

type GatewayAuth interface {
	Authenticate(ctx context.Context, authHeader string) (auth.AuthResult, error)
}

type GatewayAuthConfig struct {
	SidecarAddr string        // e.g., http://127.0.0.1:8081 or 127.0.0.1:8081
	Timeout     time.Duration // client timeout
	FailOpen    bool
}

// NewGatewayAuthFromConfig builds a GatewayAuth that calls the local sidecar HTTP validate endpoint.
func NewGatewayAuthFromConfig(cfg GatewayAuthConfig) (GatewayAuth, error) {
	addr := cfg.SidecarAddr
	if addr == "" {
		addr = "127.0.0.1:8081"
	}
	// ensure scheme
	if !strings.HasPrefix(addr, "http://") && !strings.HasPrefix(addr, "https://") {
		addr = "http://" + addr
	}
	// trim trailing slash (use TrimSuffix for clarity)
	addr = strings.TrimSuffix(addr, "/")

	// default timeout
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 2 * time.Second
	}

	hc := &http.Client{Timeout: timeout}
	sc := &sidecarClient{addr: addr, client: hc, failOpen: cfg.FailOpen}
	return sc, nil
}

type sidecarClient struct {
	addr     string
	client   *http.Client
	failOpen bool
}

func (s *sidecarClient) Authenticate(ctx context.Context, authHeader string) (auth.AuthResult, error) {
	var res auth.AuthResult

	// early reject empty auth header
	if strings.TrimSpace(authHeader) == "" {
		if s.failOpen {
			return auth.AuthResult{Valid: true, Error: "fail-open"}, nil
		}
		return res, auth.ErrUnauthorized
	}

	// normalize common auth header formats (e.g. "Bearer <token>") before sending
	normalized := normalizeAuth(authHeader)
	body := map[string]string{"authorization": normalized}
	b, err := json.Marshal(body)
	if err != nil {
		if s.failOpen {
			return auth.AuthResult{Valid: true, Error: "fail-open"}, nil
		}
		return res, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", s.addr+"/validate", bytes.NewReader(b))
	if err != nil {
		if s.failOpen {
			return auth.AuthResult{Valid: true, Error: "fail-open"}, nil
		}
		return res, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		if s.failOpen {
			return auth.AuthResult{Valid: true, Error: "fail-open"}, nil
		}
		return res, err
	}
	defer resp.Body.Close()

	// explicit unauthorized handling: return sentinel error so callers can switch on it
	if resp.StatusCode == http.StatusUnauthorized {
		if s.failOpen {
			return auth.AuthResult{Valid: true, Error: "fail-open"}, nil
		}
		return res, auth.ErrUnauthorized
	}

	// 200: decode result and return as-is, but treat Valid==false as unauthorized
	if resp.StatusCode == http.StatusOK {
		if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
			if s.failOpen {
				return auth.AuthResult{Valid: true, Error: "fail-open"}, nil
			}
			return res, err
		}
		if !res.Valid {
			if s.failOpen {
				return auth.AuthResult{Valid: true, Error: "fail-open"}, nil
			}
			return res, auth.ErrUnauthorized
		}
		return res, nil
	}

	// other non-200 -> try to decode error payload; if decode fails, handle according to failOpen
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		if s.failOpen {
			return auth.AuthResult{Valid: true, Error: "fail-open"}, nil
		}
		return res, err
	}

	// if decoded payload indicates unauthorized, surface sentinel error
	if !res.Valid {
		if s.failOpen {
			return auth.AuthResult{Valid: true, Error: "fail-open"}, nil
		}
		return res, auth.ErrUnauthorized
	}

	// non-200 but payload indicates valid (rare): return payload
	return res, nil
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
