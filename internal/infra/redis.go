package infra

import (
	"github.com/goim/goim/internal/config"
	"github.com/redis/go-redis/v9"
)

// NewRedisClient creates a configured Redis client.
func NewRedisClient(cfg *config.RedisConfig) (*redis.Client, error) {
	rdb := redis.NewClient(&redis.Options{
		Addr:     cfg.Addr,
		Password: cfg.Password,
		DB:       cfg.DB,
	})
	return rdb, nil
}
