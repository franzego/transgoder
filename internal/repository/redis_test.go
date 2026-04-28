package repository

import (
	"context"
	"testing"
	"time"

	"github.com/franzego/transcoder/internal/config"
	"github.com/franzego/transcoder/internal/connection"
	"github.com/redis/go-redis/v9"
)

func newFailingRedisRepo() *RedisRepo {
	cfg := &config.RedisConfig{
		StreamName: "jobs",
		GroupName:  "workers",
	}
	client := redis.NewClient(&redis.Options{
		Addr:        "127.0.0.1:0",
		DialTimeout: 20 * time.Millisecond,
		ReadTimeout: 20 * time.Millisecond,
	})

	return NewRedisRepo(cfg, &connection.RedisClient{Client: client})
}

func TestNewRedisRepo(t *testing.T) {
	cfg := &config.RedisConfig{StreamName: "jobs", GroupName: "workers"}
	client := &connection.RedisClient{Client: redis.NewClient(&redis.Options{Addr: "127.0.0.1:0"})}

	if got := NewRedisRepo(nil, client); got != nil {
		t.Fatalf("expected nil with nil cfg, got %#v", got)
	}
	if got := NewRedisRepo(cfg, nil); got != nil {
		t.Fatalf("expected nil with nil conn, got %#v", got)
	}
	if got := NewRedisRepo(cfg, client); got == nil {
		t.Fatal("expected repo with valid args, got nil")
	}
}

func TestRedisRepoMethods_ReturnErrorsWhenRedisUnavailable(t *testing.T) {
	repo := newFailingRedisRepo()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	t.Run("AddtoStream", func(t *testing.T) {
		err := repo.AddtoStream(ctx, "job-1")
		if err == nil {
			t.Fatal("expected error when redis is unavailable")
		}
	})

	t.Run("GetFromStream", func(t *testing.T) {
		_, _, err := repo.GetFromStream(ctx, "worker-1")
		if err == nil {
			t.Fatal("expected error when redis is unavailable")
		}
	})

	t.Run("ClaimStuckMessages", func(t *testing.T) {
		_, err := repo.ClaimStuckMessages(ctx, "worker-1", time.Second)
		if err == nil {
			t.Fatal("expected error when redis is unavailable")
		}
	})
}

