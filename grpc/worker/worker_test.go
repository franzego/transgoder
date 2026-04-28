package worker

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/franzego/transcoder/grpc/repository"
	pb "github.com/franzego/transcoder/grpc/server"
	"github.com/franzego/transcoder/grpc/service"
)

type fakeProcessor struct{}

type fakeRedisQueue struct {
	ackErr  error
	acks    []string
	acksMu  sync.Mutex
	getFrom func(context.Context, string) (repository.JobStuff, error)
}

func (f *fakeRedisQueue) GetFromStream(ctx context.Context, workerID string) (repository.JobStuff, error) {
	if f.getFrom != nil {
		return f.getFrom(ctx, workerID)
	}
	return repository.JobStuff{}, nil
}

func (f *fakeRedisQueue) Ack(_ context.Context, messageID string) error {
	f.acksMu.Lock()
	f.acks = append(f.acks, messageID)
	f.acksMu.Unlock()
	return f.ackErr
}

func (fakeProcessor) TranscodeVideo(context.Context, *pb.TranscodeRequest) (*pb.TranscodeResponse, error) {
	return &pb.TranscodeResponse{Success: true}, nil
}

func TestNewWorkerPool(t *testing.T) {
	repo := &fakeRedisQueue{}
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

func TestReserveAndReleaseJob(t *testing.T) {
	wp := NewWorkerPool(1, &fakeRedisQueue{}, fakeProcessor{})
	if wp == nil {
		t.Fatal("expected non-nil worker pool")
	}

	if ok := wp.reserveJob(""); ok {
		t.Fatal("expected empty job id reserve to fail")
	}
	if ok := wp.reserveJob("JB-1"); !ok {
		t.Fatal("expected first reserve to succeed")
	}
	if ok := wp.reserveJob("JB-1"); ok {
		t.Fatal("expected duplicate reserve to fail")
	}

	wp.releaseJob("JB-1")
	if ok := wp.reserveJob("JB-1"); !ok {
		t.Fatal("expected reserve after release to succeed")
	}
}

func TestHandleResults_AcksSuccessAndFailure(t *testing.T) {
	repo := &fakeRedisQueue{}
	wp := NewWorkerPool(1, repo, fakeProcessor{})
	if wp == nil {
		t.Fatal("expected non-nil worker pool")
	}

	_ = wp.reserveJob("JB-SUCCESS")
	_ = wp.reserveJob("JB-FAIL")

	results := make(chan service.WorkerResult, 2)
	results <- service.WorkerResult{WorkerID: 1, Job: repository.JobStuff{JobID: "JB-SUCCESS", StreamMessage: "1-0"}}
	results <- service.WorkerResult{WorkerID: 2, Job: repository.JobStuff{JobID: "JB-FAIL", StreamMessage: "2-0"}, Err: errors.New("transcode failed")}
	close(results)

	wp.handleResults(context.Background(), results)

	if len(repo.acks) != 2 {
		t.Fatalf("expected 2 acks, got %d", len(repo.acks))
	}
	acked := map[string]bool{}
	for _, id := range repo.acks {
		acked[id] = true
	}
	if !acked["1-0"] || !acked["2-0"] {
		t.Fatalf("expected acks for 1-0 and 2-0, got %+v", repo.acks)
	}

	if wp.reserveJob("JB-SUCCESS") == false {
		t.Fatal("expected success job to be released from in-flight")
	}
	if wp.reserveJob("JB-FAIL") == false {
		t.Fatal("expected failed job to be released from in-flight")
	}
}
