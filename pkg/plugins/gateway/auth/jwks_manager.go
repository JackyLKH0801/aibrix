package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"k8s.io/klog/v2"
)

// JWKSManager fetches and caches a JWKS document and exposes keys by kid.
type JWKSManager struct {
	url     string
	refresh time.Duration

	client *http.Client
	mu     sync.RWMutex
	keys   map[string]json.RawMessage
}

func NewJWKSManager(url string, refresh time.Duration) *JWKSManager {
	return &JWKSManager{
		url:     url,
		refresh: refresh,
		client:  &http.Client{Timeout: 10 * time.Second},
		keys:    make(map[string]json.RawMessage),
	}
}

// Start fetches JWKS once and starts a background refresher. It returns nil.
func (j *JWKSManager) Start(ctx context.Context) error {
	if j.client == nil {
		j.client = &http.Client{Timeout: 10 * time.Second}
	}
	if j.keys == nil {
		j.keys = make(map[string]json.RawMessage)
	}

	// Initial fetch
	if err := j.fetchAndStore(ctx); err != nil {
		klog.Warningf("jwks: initial fetch failed: %v", err)
	} else {
		klog.Infof("jwks: initial fetch succeeded")
	}

	if j.refresh <= 0 {
		klog.Infof("jwks: refresh disabled (refresh=%v)", j.refresh)
		return nil
	}

	go func() {
		ticker := time.NewTicker(j.refresh)
		defer ticker.Stop()
		klog.Infof("jwks: background refresh started (interval=%v)", j.refresh)
		for {
			select {
			case <-ctx.Done():
				klog.Infof("jwks: background refresh stopping: %v", ctx.Err())
				return
			case <-ticker.C:
				if err := j.fetchAndStore(ctx); err != nil {
					klog.Warningf("jwks: refresh failed: %v", err)
				} else {
					klog.Infof("jwks: refreshed keys successfully")
				}
			}
		}
	}()

	return nil
}

// GetKey returns the raw JSON for a key with the given kid. The boolean
// indicates whether the key was found.
func (j *JWKSManager) GetKey(kid string) (json.RawMessage, bool) {
	j.mu.RLock()
	defer j.mu.RUnlock()
	v, ok := j.keys[kid]
	return v, ok
}

// fetchAndStore downloads the JWKS and updates the internal map.
func (j *JWKSManager) fetchAndStore(ctx context.Context) error {
	if j.url == "" {
		return fmt.Errorf("jwks: empty url")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, j.url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := j.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("jwks: unexpected status %d: %s", resp.StatusCode, string(body))
	}

	var payload struct {
		Keys []json.RawMessage `json:"keys"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return err
	}

	newKeys := make(map[string]json.RawMessage, len(payload.Keys))
	for _, raw := range payload.Keys {
		var meta struct {
			Kid string `json:"kid"`
		}
		if err := json.Unmarshal(raw, &meta); err != nil {
			klog.Warningf("jwks: skipping key with unparsable kid: %v", err)
			continue
		}
		if meta.Kid == "" {
			klog.Warning("jwks: skipping key without kid")
			continue
		}
		newKeys[meta.Kid] = raw
	}

	j.mu.Lock()
	j.keys = newKeys
	j.mu.Unlock()
	return nil
}
