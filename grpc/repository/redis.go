package repository

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
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

// Claim only clearly abandoned messages.
// Normal transcodes can run for several minutes, so a short idle window causes duplicate processing.
const reclaimMinIdle = 35 * time.Minute

// extractJobID looks for common keys that might contain the job ID and returns the first non-empty value as a string.
// This allows for flexibility in the structure of the Redis stream messages while still reliably extracting the job ID for processing.
func extractJobID(values map[string]any) (string, bool) {
	keys := []string{"job", "job_id"}
	for _, k := range keys {
		v, ok := values[k]
		if !ok || v == nil {
			continue
		}
		switch t := v.(type) {
		case string:
			id := strings.TrimSpace(t)
			if id != "" {
				return id, true
			}
		case []byte:
			id := strings.TrimSpace(string(t))
			if id != "" {
				return id, true
			}
		default:
			id := strings.TrimSpace(fmt.Sprint(t))
			if id != "" && id != "<nil>" {
				return id, true
			}
		}
	}
	return "", false
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
	parseMessage := func(message redis.XMessage) (JobStuff, error) {
		jobID, ok := extractJobID(message.Values)
		if !ok {
			// Poison message: ack it so one bad payload does not block this consumer forever.
			if _, ackErr := re.conn.XAck(ctx, re.cfg.StreamName, re.cfg.GroupName, message.ID).Result(); ackErr != nil {
				return JobStuff{}, fmt.Errorf("job field missing in message %s (values=%v); additionally failed to ack malformed message: %w", message.ID, message.Values, ackErr)
			}
			slog.Error("Acked malformed redis stream message", "message_id", message.ID, "values", message.Values)
			return JobStuff{}, nil
		}
		return JobStuff{
			JobID:         jobID,
			StreamMessage: message.ID,
		}, nil
	}

	// Reclaim only stale pending entries so in-flight messages are not duplicated.
	stale, _, err := re.conn.XAutoClaim(ctx, &redis.XAutoClaimArgs{
		Stream:   re.cfg.StreamName,
		Group:    re.cfg.GroupName,
		Consumer: workerID,
		MinIdle:  reclaimMinIdle,
		Start:    "0-0",
		Count:    1,
	}).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		return JobStuff{}, err
	}
	if len(stale) > 0 {
		return parseMessage(stale[0])
	}

	// Then read new messages.
	res, err := re.conn.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    re.cfg.GroupName,
		Consumer: workerID,
		Streams:  []string{re.cfg.StreamName, ">"},
		Count:    1,
		Block:    5 * time.Second,
	}).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return JobStuff{}, nil
		}
		slog.Info("No job in stream")
		return JobStuff{}, err
	}
	if len(res) == 0 || len(res[0].Messages) == 0 {
		return JobStuff{}, nil
	}
	return parseMessage(res[0].Messages[0])
}

func (re *RedisRepo) Ack(ctx context.Context, messageID string) error {
	if messageID == "" {
		return nil
	}
	_, err := re.conn.XAck(ctx, re.cfg.StreamName, re.cfg.GroupName, messageID).Result()
	return err
}
