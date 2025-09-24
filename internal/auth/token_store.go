package auth

import (
	"context"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
)

type RedisTokenStore struct {
	client *redis.Client
	prefix string
}

func NewRedisTokenStore(client *redis.Client) *RedisTokenStore {
	return &RedisTokenStore{
		client: client,
		prefix: "im:auth:token:",
	}
}

func (s *RedisTokenStore) SaveToken(ctx context.Context, token string, subject string, ttl time.Duration) error {
	if s.client == nil {
		return errors.New("redis token store: client is nil")
	}
	return s.client.Set(ctx, s.prefix+token, subject, ttl).Err()
}

func (s *RedisTokenStore) DeleteToken(ctx context.Context, token string) error {
	if s.client == nil {
		return errors.New("redis token store: client is nil")
	}
	return s.client.Del(ctx, s.prefix+token).Err()
}

func (s *RedisTokenStore) LookupSubject(ctx context.Context, token string) (string, error) {
	if s.client == nil {
		return "", errors.New("redis token store: client is nil")
	}
	value, err := s.client.Get(ctx, s.prefix+token).Result()
	if errors.Is(err, redis.Nil) {
		return "", errors.New("token not found")
	}
	return value, err
}
