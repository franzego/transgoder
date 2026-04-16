package repository

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/franzego/transgoder/internal/config"
	"github.com/franzego/transgoder/internal/connection"
	"github.com/redis/go-redis/v9"
)

type RedisRepo struct {
	cfg  *config.RedisConfig
	conn *connection.RedisClient
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

func (re *RedisRepo) AddtoStream(ctx context.Context, jobID string) error {
	if err := re.conn.XGroupCreateMkStream(ctx, re.cfg.StreamName, re.cfg.GroupName, "$").Err(); err != nil {
		return err
	}
	_, err := re.conn.XAdd(ctx, &redis.XAddArgs{
		Stream: re.cfg.StreamName,
		Values: map[string]string{
			"job": jobID,
		},
	}).Result()
	if err != nil {
		return err
	}
	return nil
}

func (re *RedisRepo) GetFromStream(ctx context.Context, workerID string) (string, string, error) {
	res, err := re.conn.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    re.cfg.GroupName,
		Consumer: workerID,
		Streams:  []string{re.cfg.StreamName, ">"},
		Count:    1,
	}).Result()
	if err != nil {
		// Handle redis.Nil specifically (timeout when Block > 0)
		if errors.Is(err, redis.Nil) {
			return "", "", nil
		}
		slog.Error("An error has occurred reading from redis group", "error", err)
		return "", "", err
	}
	if len(res) == 0 || len(res[0].Messages) == 0 {
		return "", "", nil
	}
	message := res[0].Messages[0]
	jobID, ok := message.Values["job"].(string)
	if !ok {
		return "", "", fmt.Errorf("job field missing or not a string in message %s", message.ID)
	}
	// Always return the jobID and message ID, even if there's an error, so the worker can ack or claim as needed.
	// The worker needs this ID to call XACK after the job is finished.
	return jobID, message.ID, nil
}

func (re *RedisRepo) ClaimStuckMessages(ctx context.Context, workerID string, duration time.Duration) ([]string, error) {

	res, _, err := re.conn.XAutoClaim(ctx, &redis.XAutoClaimArgs{
		Stream:   re.cfg.StreamName,
		Group:    re.cfg.GroupName,
		Consumer: workerID,
		MinIdle:  duration, // Only claim messages idle for longer than this
		Start:    "0-0",    // Start from the beginning of the PEL
		Count:    4,        // Limit how many messages to claim at once
	}).Result()

	if err != nil && !errors.Is(err, redis.Nil) {
		return nil, err
	}

	jobIDs := make([]string, 0, len(res))
	for _, msg := range res {
		if jobID, ok := msg.Values["job"].(string); ok {
			jobIDs = append(jobIDs, jobID)
			// Remember to ack.
		}
	}
	return jobIDs, nil
}
