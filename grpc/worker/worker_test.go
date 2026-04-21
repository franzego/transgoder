package worker

import (
	"context"
	"testing"

	"github.com/franzego/transcoder/grpc/repository"
	pb "github.com/franzego/transcoder/grpc/server"
)

type fakeProcessor struct{}

func (fakeProcessor) TranscodeVideo(context.Context, *pb.TranscodeRequest) (*pb.TranscodeResponse, error) {
	return &pb.TranscodeResponse{Success: true}, nil
}

func TestNewWorkerPool(t *testing.T) {
	repo := &repository.RedisRepo{}
	processor := fakeProcessor{}

	if got := NewWorkerPool(0, repo, processor); got != nil {
		t.Fatalf("expected nil for zero workers, got %#v", got)
	}
	if got := NewWorkerPool(2, nil, processor); got != nil {
		t.Fatalf("expected nil for nil repo, got %#v", got)
	}
	if got := NewWorkerPool(2, repo, nil); got != nil {
		t.Fatalf("expected nil for nil processor, got %#v", got)
	}

	got := NewWorkerPool(3, repo, processor)
	if got == nil {
		t.Fatal("expected non-nil worker pool")
	}
	if got.workers != 3 {
		t.Fatalf("expected workers=3, got %d", got.workers)
	}
	if got.consumer == "" {
		t.Fatal("expected non-empty consumer name")
	}
}
