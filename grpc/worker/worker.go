package worker

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/franzego/transcoder/grpc/repository"
	"github.com/franzego/transcoder/grpc/service"
)

type Workerpool struct {
	workers   int
	redisRepo *repository.RedisRepo
	processor service.Transcoder
	consumer  string
}

func NewWorkerPool(workers int, redRepo *repository.RedisRepo, processor service.Transcoder) *Workerpool {
	if redRepo == nil || processor == nil || workers <= 0 {
		return nil
	}
	return &Workerpool{
		workers:   workers,
		redisRepo: redRepo,
		processor: processor,
		consumer:  fmt.Sprintf("pool-dispatcher-%d", workers),
	}
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
		select {
		case <-ctx.Done():
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
			if result.Err != nil {
				slog.Error("Worker failed to transcode job", "worker_id", result.WorkerID, "job_id", result.Job.JobID, "error", result.Err)
				continue
			}
			err := wp.redisRepo.Ack(ctx, result.Job.StreamMessage)
			if err != nil {
				slog.Error("Worker completed job but failed to ack stream message", "worker_id", result.WorkerID, "job_id", result.Job.JobID, "message_id", result.Job.StreamMessage, "error", err)
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
