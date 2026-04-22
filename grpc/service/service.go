package service

import (
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"

	pb "github.com/franzego/transcoder/grpc/server"
	"github.com/franzego/transcoder/grpc/webserver"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type TranscoderService struct {
	logger *slog.Logger
	pb.UnimplementedTranscoderServiceServer
	wc         *webserver.WebserverClient
	ffmpegPath string
}

func NewTranscoderService(logger *slog.Logger, wc *webserver.WebserverClient, ffmpegPath string) *TranscoderService {
	if logger == nil {
		ts := &TranscoderService{
			logger:     logger,
			wc:         wc,
			ffmpegPath: ffmpegPath,
		}
		return ts
	}
	if ffmpegPath == "" {
		ffmpegPath = "ffmpeg"
	}
	return &TranscoderService{
		logger:     logger,
		wc:         wc,
		ffmpegPath: ffmpegPath,
	}

}
func (s *TranscoderService) TranscodeVideo(ctx context.Context, req *pb.TranscodeRequest) (*pb.TranscodeResponse, error) {
	if req.GetJobId() == "" {
		return nil, status.Error(codes.InvalidArgument, "job_id is required")
	}
	if s.wc == nil {
		return nil, status.Error(codes.FailedPrecondition, "webserver client is required")
	}

	start := time.Now()
	jobID := req.GetJobId()
	currentStatus := webserver.StatusQueued
	outputPath := req.GetOutputPath()
	if outputPath == "" {
		outputPath = filepath.Join("/tmp", jobID+".mp4")
	}

	if err := s.transitionStatus(ctx, jobID, currentStatus, webserver.StatusDownloading); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to update job status to downloading: %v", err)
	}
	currentStatus = webserver.StatusDownloading

	inputURL, err := s.wc.GetSourceURL(ctx, jobID)
	if err != nil {
		_ = s.transitionStatus(ctx, jobID, currentStatus, webserver.StatusFailed)
		return nil, status.Errorf(codes.Internal, "failed to fetch source url: %v", err)
	}

	if err := s.transitionStatus(ctx, jobID, currentStatus, webserver.StatusProcessing); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to update job status to processing: %v", err)
	}
	currentStatus = webserver.StatusProcessing

	ffmpegArgs := buildFFmpegArgs(inputURL, outputPath, req)
	cmd := exec.CommandContext(ctx, s.ffmpegPath, ffmpegArgs...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		_ = s.transitionStatus(ctx, jobID, currentStatus, webserver.StatusFailed)
		return nil, status.Errorf(codes.Internal, "ffmpeg failed: %v, output=%s", err, string(output))
	}

	if err := s.transitionStatus(ctx, jobID, currentStatus, webserver.StatusUploading); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to update job status to uploading: %v", err)
	}
	currentStatus = webserver.StatusUploading

	if err := s.transitionStatus(ctx, jobID, currentStatus, webserver.StatusCompleted); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to update job status to completed: %v", err)
	}

	return &pb.TranscodeResponse{
		JobId:      jobID,
		Status:     "completed",
		Success:    true,
		OutputPath: outputPath,
		DurationMs: time.Since(start).Milliseconds(),
	}, nil
}

func (s *TranscoderService) transitionStatus(ctx context.Context, jobID string, from, to webserver.Status) error {
	_, err := s.wc.UpdateJobStatus(ctx, webserver.JobStatusReq{
		JobID: jobID,
		From:  from,
		To:    to,
	})
	return err
}

func buildFFmpegArgs(inputPath, outputPath string, req *pb.TranscodeRequest) []string {
	args := []string{
		"-y",
		"-i", inputPath,
	}
	if opts := req.GetOptions(); opts != nil {
		if opts.GetCodec() != "" {
			switch opts.GetCodec() {
			case "h264":
				args = append(args, "-c:v", "libx264")
			case "h265":
				args = append(args, "-c:v", "libx265")
			case "vp9":
				args = append(args, "-c:v", "libvpx-vp9")
			default:
				args = append(args, "-c:v", opts.GetCodec())
			}
		}
		if opts.GetBitrate() > 0 {
			args = append(args, "-b:v", fmt.Sprintf("%dk", opts.GetBitrate()))
		}
		if opts.GetFramerate() > 0 {
			args = append(args, "-r", strconv.Itoa(int(opts.GetFramerate())))
		}
		if opts.GetResolution() != "" {
			args = append(args, "-s", opts.GetResolution())
		}
	}
	if req.GetOutputFormat() != "" {
		args = append(args, "-f", req.GetOutputFormat())
	}
	args = append(args, outputPath)
	return args
}
