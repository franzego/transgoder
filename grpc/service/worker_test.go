package service

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/franzego/transcoder/grpc/repository"
	pb "github.com/franzego/transcoder/grpc/server"
)

type fakeTranscoder struct {
	mu    sync.Mutex
	calls int
	err   error
}

func (f *fakeTranscoder) TranscodeVideo(ctx context.Context, req *pb.TranscodeRequest) (*pb.TranscodeResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return &pb.TranscodeResponse{
		JobId:   req.GetJobId(),
		Status:  "completed",
		Success: true,
	}, nil
}

func (f *fakeTranscoder) CallCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func TestWorkerProcessesJobAndSendsResult(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	jobs := make(chan repository.JobStuff, 1)
	results := make(chan WorkerResult, 1)
	transcoder := &fakeTranscoder{}

	var wg sync.WaitGroup
	wg.Add(1)
	go Worker(ctx, 1, jobs, results, transcoder, &wg)

	jobs <- repository.JobStuff{JobID: "job-123", StreamMessage: "1-0"}
	close(jobs)
	wg.Wait()
	close(results)

	got, ok := <-results
	if !ok {
		t.Fatalf("expected a worker result but channel was empty")
	}
	if got.Err != nil {
		t.Fatalf("expected nil error, got %v", got.Err)
	}
	if got.Job.JobID != "job-123" {
		t.Fatalf("expected job id job-123, got %q", got.Job.JobID)
	}
	if transcoder.CallCount() != 1 {
		t.Fatalf("expected 1 transcode call, got %d", transcoder.CallCount())
	}
}

func TestWorkerSkipsEmptyJobID(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	jobs := make(chan repository.JobStuff, 1)
	results := make(chan WorkerResult, 1)
	transcoder := &fakeTranscoder{}

	var wg sync.WaitGroup
	wg.Add(1)
	go Worker(ctx, 1, jobs, results, transcoder, &wg)

	jobs <- repository.JobStuff{JobID: "", StreamMessage: "1-0"}
	close(jobs)
	wg.Wait()
	close(results)

	if transcoder.CallCount() != 0 {
		t.Fatalf("expected no transcode calls for empty job id, got %d", transcoder.CallCount())
	}
	if _, ok := <-results; ok {
		t.Fatalf("expected no worker results for empty job id")
	}
}

func TestWorkerPropagatesTranscodeError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	jobs := make(chan repository.JobStuff, 1)
	results := make(chan WorkerResult, 1)
	transcoder := &fakeTranscoder{err: errors.New("transcode failed")}

	var wg sync.WaitGroup
	wg.Add(1)
	go Worker(ctx, 1, jobs, results, transcoder, &wg)

	jobs <- repository.JobStuff{JobID: "job-err", StreamMessage: "2-0"}
	close(jobs)
	wg.Wait()
	close(results)

	got, ok := <-results
	if !ok {
		t.Fatalf("expected a worker result but channel was empty")
	}
	if got.Err == nil {
		t.Fatalf("expected transcode error, got nil")
	}
	if transcoder.CallCount() != 1 {
		t.Fatalf("expected 1 transcode call, got %d", transcoder.CallCount())
	}
}
