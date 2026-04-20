package service

import (
	"context"
	"fmt"
	"time"

	"github.com/franzego/transcoder/grpc/connection"
	pb "github.com/franzego/transcoder/grpc/server"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type TranscoderService struct {
	pb.UnimplementedTranscoderServiceServer
	Redis *connection.RedisClient
}

func (s *TranscoderService) TranscodeVideo(ctx context.Context, req *pb.TranscodeRequest) (*pb.TranscodeResponse, error) {
	if req.GetJobId() == "" {
		return nil, status.Error(codes.InvalidArgument, "job_id is required")
	}

	start := time.Now()
	jobID := req.GetJobId()

	if err := s.setJobStatus(ctx, jobID, "processing"); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to set processing status in redis: %v", err)
	}

	// TODO: invoke ffmpeg transcoding here.

	if err := s.setJobStatus(ctx, jobID, "completed"); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to set completed status in redis: %v", err)
	}

	return &pb.TranscodeResponse{
		JobId:      jobID,
		Status:     "completed",
		Success:    true,
		OutputPath: req.GetOutputPath(),
		DurationMs: time.Since(start).Milliseconds(),
	}, nil
}

func (s *TranscoderService) setJobStatus(ctx context.Context, jobID, state string) error {
	key := fmt.Sprintf("job:%s:status", jobID)
	return s.Redis.Set(ctx, key, state, 24*time.Hour).Err()
}
