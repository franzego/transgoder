package connection

import (
	"context"
	"log/slog"
	"strings"

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
	logg.Info("Starting creation of group and stream", "streamname", c.Redis.StreamName, "groupname", c.Redis.GroupName)
	err := client.XGroupCreateMkStream(ctx, c.Redis.StreamName, c.Redis.GroupName, "0").Err()
	if err != nil && !strings.HasPrefix(err.Error(), "BUSYGROUP") {
		return nil, err
	}

	return &RedisClient{
		client,
	}, nil
}
