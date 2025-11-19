package auth

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
)

type RedisKeyManager struct {
	client *redis.Client
	prefix string
}

func NewRedisKeyManager(client *redis.Client, namespace string) KeyManager {
	p := namespace
	if p == "" {
		p = "aibrix"
	}
	return &RedisKeyManager{client: client, prefix: p + ":apikey:"}
}

func (r *RedisKeyManager) keyName(apiKey string) string {
	return r.prefix + apiKey
}

func (r *RedisKeyManager) GetByAPIKey(ctx context.Context, apiKey string) (KeyRecord, error) {
	var rec KeyRecord
	val, err := r.client.Get(ctx, r.keyName(apiKey)).Result()
	if err == redis.Nil {
		return rec, ErrUnauthorized
	}
	if err != nil {
		return rec, err
	}
	if err := json.Unmarshal([]byte(val), &rec); err != nil {
		return rec, err
	}
	// Basic expiry check
	if rec.ExpiresAt != nil && time.Now().After(*rec.ExpiresAt) {
		return rec, ErrUnauthorized
	}
	return rec, nil
}

func (r *RedisKeyManager) CreateKey(ctx context.Context, rec KeyRecord) error {
	if rec.APIKeyID == "" {
		return errors.New("api_key_id required")
	}
	b, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	key := r.keyName(rec.APIKeyID)
	var ttl time.Duration
	if rec.ExpiresAt != nil {
		ttl = time.Until(*rec.ExpiresAt)
		if ttl < 0 {
			ttl = 0
		}
	}
	return r.client.Set(ctx, key, b, ttl).Err()
}

func (r *RedisKeyManager) RevokeKey(ctx context.Context, apiKey string) error {
	return r.client.Del(ctx, r.keyName(apiKey)).Err()
}
