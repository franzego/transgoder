package service

import (
	"context"
	"sync"

	"github.com/franzego/transcoder/grpc/repository"
	pb "github.com/franzego/transcoder/grpc/server"
)

type Transcoder interface {
	TranscodeVideo(ctx context.Context, req *pb.TranscodeRequest) (*pb.TranscodeResponse, error)
}

type WorkerResult struct {
	WorkerID int
	Job      repository.JobStuff
	Response *pb.TranscodeResponse
	Err      error
}

func Worker(
	ctx context.Context,
	id int,
	jobs <-chan repository.JobStuff,
	result chan<- WorkerResult,
	transcoder Transcoder,
	wg *sync.WaitGroup,
) {
	defer wg.Done()
	for {
		select {
		case <-ctx.Done():
			return
		case job, ok := <-jobs:
			if !ok {
				return
			}
			if job.JobID == "" {
				continue
			}
			req := &pb.TranscodeRequest{
				JobId: job.JobID,
			}
			res, err := transcoder.TranscodeVideo(ctx, req)
			select {
			case <-ctx.Done():
				return
			case result <- WorkerResult{
				WorkerID: id,
				Job:      job,
				Response: res,
				Err:      err,
			}:
			}
		}
	}

}
