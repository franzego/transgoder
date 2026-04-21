package repository

import (
	"context"
	"testing"
	"time"

	"github.com/franzego/transcoder/grpc/config"
	"github.com/franzego/transcoder/grpc/connection"
	"github.com/redis/go-redis/v9"
)

func newFailingRepo() *RedisRepo {
	cfg := &config.RedisConfig{
		StreamName: "jobs",
		GroupName:  "workers",
	}
	client := redis.NewClient(&redis.Options{
		Addr:         "127.0.0.1:0",
		DialTimeout:  20 * time.Millisecond,
		ReadTimeout:  20 * time.Millisecond,
		WriteTimeout: 20 * time.Millisecond,
	})
	return NewRedisRepo(cfg, &connection.RedisClient{Client: client})
}

func TestNewRedisRepo(t *testing.T) {
	cfg := &config.RedisConfig{StreamName: "jobs", GroupName: "workers"}
	conn := &connection.RedisClient{Client: redis.NewClient(&redis.Options{Addr: "127.0.0.1:0"})}

	if got := NewRedisRepo(nil, conn); got != nil {
		t.Fatalf("expected nil with nil cfg, got %#v", got)
	}
	if got := NewRedisRepo(cfg, nil); got != nil {
		t.Fatalf("expected nil with nil conn, got %#v", got)
	}
	if got := NewRedisRepo(cfg, conn); got == nil {
		t.Fatal("expected repo with valid args, got nil")
	}
}

func TestAck(t *testing.T) {
	repo := newFailingRepo()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	t.Run("empty message id is noop", func(t *testing.T) {
		if err := repo.Ack(ctx, ""); err != nil {
			t.Fatalf("expected nil error for empty message id, got %v", err)
		}
	})

	t.Run("redis unavailable returns error", func(t *testing.T) {
		if err := repo.Ack(ctx, "1-0"); err == nil {
			t.Fatal("expected error when redis is unavailable")
		}
	})
}

func TestGetFromStream_RedisUnavailable(t *testing.T) {
	repo := newFailingRepo()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	if _, err := repo.GetFromStream(ctx, "worker-1"); err == nil {
		t.Fatal("expected error when redis is unavailable")
	}
}
