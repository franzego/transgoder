package service

import (
	"context"
	"testing"
	"time"

	"github.com/franzego/transgoder/internal/config"
	"github.com/franzego/transgoder/internal/connection"
	"github.com/franzego/transgoder/internal/repository"
	"github.com/redis/go-redis/v9"
)

func newFailingRedisService() *RedisService {
	cfg := &config.RedisConfig{
		StreamName: "jobs",
		GroupName:  "workers",
	}
	client := redis.NewClient(&redis.Options{
		Addr:        "127.0.0.1:0",
		DialTimeout: 20 * time.Millisecond,
		ReadTimeout: 20 * time.Millisecond,
	})
	repo := repository.NewRedisRepo(cfg, &connection.RedisClient{Client: client})
	return NewRedisService(repo)
}

func TestNewRedisService(t *testing.T) {
	if got := NewRedisService(nil); got != nil {
		t.Fatalf("expected nil service with nil repo, got %#v", got)
	}
}

func TestRedisService_DelegatesToRepository(t *testing.T) {
	svc := newFailingRedisService()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	t.Run("Enqueue", func(t *testing.T) {
		err := svc.Enqueue(ctx, "job-1")
		if err == nil {
			t.Fatal("expected enqueue error when redis is unavailable")
		}
	})

	t.Run("Dequeue", func(t *testing.T) {
		_, _, err := svc.Dequeue(ctx, "worker-1")
		if err == nil {
			t.Fatal("expected dequeue error when redis is unavailable")
		}
	})
}

