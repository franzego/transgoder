package worker

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/franzego/transcoder/grpc/repository"
	"github.com/franzego/transcoder/grpc/service"
)

type Workerpool struct {
	workers   int
	redisRepo redisQueue
	processor service.Transcoder
	consumer  string

	mu       sync.Mutex
	inFlight map[string]struct{}
}

type redisQueue interface {
	GetFromStream(ctx context.Context, workerID string) (repository.JobStuff, error)
	Ack(ctx context.Context, messageID string) error
}

func NewWorkerPool(workers int, redRepo redisQueue, processor service.Transcoder) *Workerpool {
	if redRepo == nil || processor == nil || workers <= 0 {
		return nil
	}
	hostname, err := os.Hostname()
	if err != nil || strings.TrimSpace(hostname) == "" {
		hostname = "unknown-host"
	}
	consumer := fmt.Sprintf("pool-dispatcher-%s-%d-%d", strings.ReplaceAll(hostname, " ", "_"), os.Getpid(), time.Now().UnixNano())

	return &Workerpool{
		workers:   workers,
		redisRepo: redRepo,
		processor: processor,
		consumer:  consumer,
		inFlight:  make(map[string]struct{}),
	}
}

// reserveJob attempts to reserve a job ID for processing. It returns true if the job ID was successfully reserved, or false if it is already in-flight or invalid.
func (wp *Workerpool) reserveJob(jobID string) bool {
	if jobID == "" {
		return false
	}
	wp.mu.Lock()
	defer wp.mu.Unlock()
	// a simple in-memory map is used to track in-flight job IDs.
	if _, exists := wp.inFlight[jobID]; exists {
		return false
	}
	wp.inFlight[jobID] = struct{}{}
	return true
}

func (wp *Workerpool) releaseJob(jobID string) {
	if jobID == "" {
		return
	}
	wp.mu.Lock()
	delete(wp.inFlight, jobID)
	wp.mu.Unlock()
}

// Dispacter takes jobs from the redis stream and puts them into the channel for consumption by the worker pool
// It does this with reverence to context. It takes into account, the cancellation of the parent context for whatever reason.
func (wp *Workerpool) Dispatcher(ctx context.Context, jobs chan<- repository.JobStuff) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		job, err := wp.redisRepo.GetFromStream(ctx, wp.consumer)
		if err != nil {
			slog.Error("An error has occured getting job from redis stream", "error", err, "job", job.JobID)
			continue
		}
		if job.JobID == "" {
			continue
		}
		// we try to reserve the job ID for processing. If it returns false, it means this job is already being processed by another worker,
		// so we skip it to avoid duplicate work.
		if !wp.reserveJob(job.JobID) {
			slog.Warn("Skipping duplicate in-flight job", "job_id", job.JobID, "message_id", job.StreamMessage)
			continue
		}
		select {
		case <-ctx.Done():
			// this is to release the reserved job ID if the context is cancelled before the job can be dispatched to a worker, preventing it from being stuck in-flight indefinitely.
			wp.releaseJob(job.JobID)
			return
		case jobs <- job:
		}
	}
}

func (wp *Workerpool) handleResults(ctx context.Context, results <-chan service.WorkerResult) {
	for {
		select {
		case <-ctx.Done():
			return
		case result, ok := <-results:
			if !ok {
				return
			}
			wp.releaseJob(result.Job.JobID)

			ackErr := wp.redisRepo.Ack(ctx, result.Job.StreamMessage)
			if ackErr != nil {
				slog.Error(
					"Worker finished job but failed to ack stream message",
					"worker_id", result.WorkerID,
					"job_id", result.Job.JobID,
					"message_id", result.Job.StreamMessage,
					"error", ackErr,
				)
				continue
			}

			if result.Err != nil {
				slog.Error(
					"Worker finished job with failure and acknowledged message",
					"worker_id", result.WorkerID,
					"job_id", result.Job.JobID,
					"message_id", result.Job.StreamMessage,
					"error", result.Err,
				)
				continue
			}
			select {
			case <-ctx.Done():
			default:
				slog.Info("Job completed and acknowledged", "worker_id", result.WorkerID, "job_id", result.Job.JobID)
			}
		}
	}
}

// Run is the central orchestrator that gets the message from redis, deploys the workers and gets the results
func (wp *Workerpool) Run(ctx context.Context, jobcount, resultCount int) {
	jobChan := make(chan repository.JobStuff, jobcount)
	resultChan := make(chan service.WorkerResult, resultCount)

	var jobwg sync.WaitGroup
	jobwg.Add(1)
	go func() {
		defer jobwg.Done()
		wp.Dispatcher(ctx, jobChan)
	}()

	var workWg sync.WaitGroup

	for i := 1; i <= wp.workers; i++ {
		workWg.Add(1)
		go service.Worker(ctx, i, jobChan, resultChan, wp.processor, &workWg)
	}
	go func() {
		jobwg.Wait()
		close(jobChan)
	}()
	go func() {
		workWg.Wait()
		close(resultChan)
	}()
	wp.handleResults(ctx, resultChan)
}
