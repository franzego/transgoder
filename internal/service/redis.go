package service

import (
	"context"

	"github.com/franzego/transcoder/internal/repository"
)

type RedisService struct {
	r *repository.RedisRepo
}

func NewRedisService(r *repository.RedisRepo) *RedisService {
	if r == nil {
		return nil
	}
	return &RedisService{
		r: r,
	}
}

func (re *RedisService) Enqueue(ctx context.Context, jobID string) error {
	return re.r.AddtoStream(ctx, jobID)
}

func (re *RedisService) Dequeue(ctx context.Context, workerID string) (string, string, error) {
	return re.r.GetFromStream(ctx, workerID)
}
