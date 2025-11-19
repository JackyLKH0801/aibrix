package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// dynamicServer lets tests update the JWKS payload on the fly.
type dynamicServer struct {
	mu      sync.RWMutex
	payload interface{}
	status  int
}

func (d *dynamicServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	w.Header().Set("Content-Type", "application/json")
	if d.status == 0 {
		d.status = http.StatusOK
	}
	w.WriteHeader(d.status)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"keys": d.payload})
}

func (d *dynamicServer) set(payload interface{}) {
	d.mu.Lock()
	d.payload = payload
	d.status = http.StatusOK
	d.mu.Unlock()
}

func (d *dynamicServer) setStatus(status int, payload interface{}) {
	d.mu.Lock()
	d.status = status
	d.payload = payload
	d.mu.Unlock()
}

func TestFetchAndGetKey_Success(t *testing.T) {
	d := &dynamicServer{}
	key1 := map[string]interface{}{"kid": "k1", "k": "v1"}
	key2 := map[string]interface{}{"kid": "k2", "k": "v2"}
	d.set([]interface{}{key1, key2})

	srv := httptest.NewServer(d)
	defer srv.Close()

	j := NewJWKSManager(srv.URL, 0)
	j.client = srv.Client()

	if err := j.fetchAndStore(context.Background()); err != nil {
		t.Fatalf("fetchAndStore failed: %v", err)
	}

	raw, ok := j.GetKey("k1")
	if !ok {
		t.Fatalf("expected key k1 to be present")
	}
	var decoded map[string]interface{}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("failed to unmarshal stored key: %v", err)
	}
	if decoded["kid"] != "k1" {
		t.Fatalf("unexpected kid: %v", decoded["kid"])
	}
}

func TestFetch_SkipMissingKid(t *testing.T) {
	d := &dynamicServer{}
	keyWith := map[string]interface{}{"kid": "exists", "k": "v"}
	keyWithout := map[string]interface{}{"k": "no-kid"}
	d.set([]interface{}{keyWith, keyWithout})

	srv := httptest.NewServer(d)
	defer srv.Close()

	j := NewJWKSManager(srv.URL, 0)
	j.client = srv.Client()

	if err := j.fetchAndStore(context.Background()); err != nil {
		t.Fatalf("fetchAndStore failed: %v", err)
	}

	if _, ok := j.GetKey("exists"); !ok {
		t.Fatalf("expected key with kid to be present")
	}
	if _, ok := j.GetKey(""); ok {
		t.Fatalf("did not expect a key with empty kid to be stored")
	}
}

func TestFetch_Non2xxResponse(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, "internal error")
	})
	srv := httptest.NewServer(handler)
	defer srv.Close()

	j := NewJWKSManager(srv.URL, 0)
	j.client = srv.Client()

	err := j.fetchAndStore(context.Background())
	if err == nil {
		t.Fatalf("expected error for non-2xx response")
	}
	if !strings.Contains(err.Error(), "unexpected status") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestFetch_MalformedJSON(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "not-json")
	})
	srv := httptest.NewServer(handler)
	defer srv.Close()

	j := NewJWKSManager(srv.URL, 0)
	j.client = srv.Client()

	if err := j.fetchAndStore(context.Background()); err == nil {
		t.Fatalf("expected JSON decode error, got nil")
	}
}

func TestStart_BackgroundRefresh(t *testing.T) {
	d := &dynamicServer{}
	initial := map[string]interface{}{"kid": "initial", "k": "v0"}
	updated := map[string]interface{}{"kid": "updated", "k": "v1"}
	d.set([]interface{}{initial})

	srv := httptest.NewServer(d)
	defer srv.Close()

	// refresh interval short for test
	j := NewJWKSManager(srv.URL, 100*time.Millisecond)
	j.client = srv.Client()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := j.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// wait for initial fetch to complete (up to 1s)
	deadline := time.After(1 * time.Second)
	for {
		if _, ok := j.GetKey("initial"); ok {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for initial key")
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}

	// update server payload to new key and wait for refresh to pick it up
	d.set([]interface{}{updated})
	deadline = time.After(2 * time.Second)
	for {
		if _, ok := j.GetKey("updated"); ok {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for updated key")
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}
}
