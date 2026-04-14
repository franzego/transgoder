package connection

import (
	"context"
	"log/slog"

	"github.com/franzego/transgoder/internal/config"
	"github.com/redis/go-redis/v9"
)

type RedisClient struct {
	*redis.Client
}

func NewRedisConnection(ctx context.Context, c *config.Config, logg *slog.Logger) (*RedisClient, error) {
	logg.Info("Starting connection to redis container")
	client := redis.NewClient(&redis.Options{
		Addr:     c.RedisAddr(),
		Password: c.Redis.Password,
		DB:       c.Redis.DB,
	})
	logg.Info("Pingging redis")
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, err
	}

	return &RedisClient{
		client,
	}, nil
}
