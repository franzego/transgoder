package service

import (
	"context"
	"errors"
	"testing"

	"github.com/franzego/transgoder/internal/config"
	"github.com/franzego/transgoder/internal/repository"
	"github.com/redis/go-redis/v9"
)

// fakeRedisClient implements repository.RedisClientCmdable for testing
type fakeRedisClient struct {
	xAdd       func(ctx context.Context, args *redis.XAddArgs) *redis.StringCmd
	xReadGroup func(ctx context.Context, args *redis.XReadGroupArgs) *redis.XStreamSliceCmd
	xAutoClaim func(ctx context.Context, args *redis.XAutoClaimArgs) *redis.XAutoClaimCmd
}

func newStringCmd(val string, err error) *redis.StringCmd {
	cmd := redis.NewStringCmd(context.Background())
	cmd.SetVal(val)
	cmd.SetErr(err)
	return cmd
}

func newStreamCmd(val []redis.XStream, err error) *redis.XStreamSliceCmd {
	cmd := redis.NewXStreamSliceCmd(context.Background())
	cmd.SetVal(val)
	cmd.SetErr(err)
	return cmd
}

func newAutoClaimCmd(err error) *redis.XAutoClaimCmd {
	cmd := redis.NewXAutoClaimCmd(context.Background())
	cmd.SetErr(err)
	return cmd
}

// Implement repository.RedisClientCmdable interface
func (f *fakeRedisClient) XAdd(ctx context.Context, args *redis.XAddArgs) *redis.StringCmd {
	if f.xAdd == nil {
		return newStringCmd("", errors.New("xAdd not configured"))
	}
	return f.xAdd(ctx, args)
}

func (f *fakeRedisClient) XReadGroup(ctx context.Context, args *redis.XReadGroupArgs) *redis.XStreamSliceCmd {
	if f.xReadGroup == nil {
		return newStreamCmd(nil, errors.New("xReadGroup not configured"))
	}
	return f.xReadGroup(ctx, args)
}

func (f *fakeRedisClient) XAutoClaim(ctx context.Context, args *redis.XAutoClaimArgs) *redis.XAutoClaimCmd {
	if f.xAutoClaim == nil {
		return newAutoClaimCmd(errors.New("xAutoClaim not configured"))
	}
	return f.xAutoClaim(ctx, args)
}

func buildRedisServiceWithClient(client repository.RedisClientCmdable) *RedisService {
	cfg := &config.RedisConfig{
		StreamName: "jobs",
		GroupName:  "workers",
	}
	repo := repository.NewRedisRepo(cfg, client)
	return NewRedisService(repo)
}

func TestNewRedisService(t *testing.T) {
	if got := NewRedisService(nil); got != nil {
		t.Fatalf("expected nil service with nil repo, got %#v", got)
	}
}

func TestRedisService_Enqueue(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mockClient := &fakeRedisClient{
			xAdd: func(_ context.Context, args *redis.XAddArgs) *redis.StringCmd {
				return newStringCmd("1-0", nil)
			},
		}

		svc := buildRedisServiceWithClient(mockClient)
		err := svc.Enqueue(context.Background(), "job-123")
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("Error", func(t *testing.T) {
		mockClient := &fakeRedisClient{
			xAdd: func(_ context.Context, args *redis.XAddArgs) *redis.StringCmd {
				return newStringCmd("", errors.New("redis error"))
			},
		}

		svc := buildRedisServiceWithClient(mockClient)
		err := svc.Enqueue(context.Background(), "job-123")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}

func TestRedisService_Dequeue(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mockClient := &fakeRedisClient{
			xReadGroup: func(_ context.Context, _ *redis.XReadGroupArgs) *redis.XStreamSliceCmd {
				return newStreamCmd([]redis.XStream{
					{
						Stream: "jobs",
						Messages: []redis.XMessage{
							{
								ID: "1-0",
								Values: map[string]interface{}{
									"job": "job-123",
								},
							},
						},
					},
				}, nil)
			},
		}

		svc := buildRedisServiceWithClient(mockClient)
		jobID, msgID, err := svc.Dequeue(context.Background(), "worker-1")
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if jobID != "job-123" {
			t.Fatalf("expected jobID 'job-123', got %s", jobID)
		}
		if msgID != "1-0" {
			t.Fatalf("expected msgID '1-0', got %s", msgID)
		}
	})

	t.Run("NoMessages", func(t *testing.T) {
		mockClient := &fakeRedisClient{
			xReadGroup: func(_ context.Context, _ *redis.XReadGroupArgs) *redis.XStreamSliceCmd {
				return newStreamCmd([]redis.XStream{}, nil)
			},
		}

		svc := buildRedisServiceWithClient(mockClient)
		jobID, msgID, err := svc.Dequeue(context.Background(), "worker-1")
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if jobID != "" || msgID != "" {
			t.Fatalf("expected empty strings for no messages, got jobID=%s, msgID=%s", jobID, msgID)
		}
	})

	t.Run("RedisNil", func(t *testing.T) {
		mockClient := &fakeRedisClient{
			xReadGroup: func(_ context.Context, _ *redis.XReadGroupArgs) *redis.XStreamSliceCmd {
				return newStreamCmd(nil, redis.Nil)
			},
		}

		svc := buildRedisServiceWithClient(mockClient)
		jobID, msgID, err := svc.Dequeue(context.Background(), "worker-1")
		if err != nil {
			t.Fatalf("expected no error for redis.Nil, got %v", err)
		}
		if jobID != "" || msgID != "" {
			t.Fatalf("expected empty strings, got jobID=%s, msgID=%s", jobID, msgID)
		}
	})
}
