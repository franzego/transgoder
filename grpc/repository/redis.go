package repository

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/franzego/transcoder/grpc/config"
	"github.com/franzego/transcoder/grpc/connection"
	"github.com/redis/go-redis/v9"
)

type RedisRepo struct {
	cfg  *config.RedisConfig
	conn *connection.RedisClient
}

type JobStuff struct {
	JobID         string
	StreamMessage string //Need this for ack later.
}

func NewRedisRepo(cfg *config.RedisConfig, conn *connection.RedisClient) *RedisRepo {
	if conn == nil || cfg == nil {
		return nil
	}
	return &RedisRepo{
		cfg:  cfg,
		conn: conn,
	}
}

func (re *RedisRepo) GetFromStream(ctx context.Context, workerID string) (JobStuff, error) {
	res, err := re.conn.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    re.cfg.GroupName,
		Consumer: workerID,
		Streams:  []string{re.cfg.StreamName, ">"},
		Count:    1,
		Block:    5 * time.Second,
	}).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return JobStuff{
				JobID: "",
			}, nil
		}
		slog.Error("An error has occurred reading from redis group", "error", err)
		return JobStuff{}, err
	}
	if len(res) == 0 || len(res[0].Messages) == 0 {
		return JobStuff{}, nil
	}
	message := res[0].Messages[0]
	jobID, ok := message.Values["job"].(string)
	if !ok {
		return JobStuff{}, fmt.Errorf("job field missing or not a string in message %s", message.ID)
	}
	// Always return the jobID and message ID, even if there's an error, so the worker can ack or claim as needed.
	// The worker needs this ID to call XACK after the job is finished.
	return JobStuff{
		JobID:         jobID,
		StreamMessage: message.ID,
	}, nil
}

func (re *RedisRepo) Ack(ctx context.Context, messageID string) error {
	if messageID == "" {
		return nil
	}
	_, err := re.conn.XAck(ctx, re.cfg.StreamName, re.cfg.GroupName, messageID).Result()
	return err
}
