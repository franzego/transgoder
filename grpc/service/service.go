package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"

	"github.com/franzego/transcoder/grpc/connection"
	"github.com/franzego/transcoder/grpc/retry"
	pb "github.com/franzego/transcoder/grpc/server"
	"github.com/franzego/transcoder/grpc/webserver"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type objectUploader interface {
	FPutObject(ctx context.Context, bucketName, objectName, filePath string, opts minio.PutObjectOptions) (minio.UploadInfo, error)
}

type TranscoderService struct {
	logger *slog.Logger
	pb.UnimplementedTranscoderServiceServer
	wc             *webserver.WebserverClient
	uploader       objectUploader
	downloadBucket string
	ffmpegPath     string
	ffprobePath    string
	retryer        *retry.Retry
}

const defaultTranscodeTimeout = 30 * time.Minute

func NewTranscoderService(logger *slog.Logger, wc *webserver.WebserverClient, minioClient *connection.MinioClient, downloadBucket, ffmpegPath, ffprobePath string) *TranscoderService {
	if logger == nil {
		logger = slog.Default()
	}
	if ffmpegPath == "" {
		ffmpegPath = "ffmpeg"
	}
	if ffprobePath == "" {
		ffprobePath = "ffprobe"
	}
	return &TranscoderService{
		logger:         logger,
		wc:             wc,
		uploader:       minioClient,
		downloadBucket: downloadBucket,
		ffmpegPath:     ffmpegPath,
		ffprobePath:    ffprobePath,
		retryer:        retry.NewRetry(),
	}
}

func (s *TranscoderService) TranscodeVideo(ctx context.Context, req *pb.TranscodeRequest) (*pb.TranscodeResponse, error) {
	if req.GetJobId() == "" {
		return nil, status.Error(codes.InvalidArgument, "job_id is required")
	}
	if s.wc == nil {
		return nil, status.Error(codes.FailedPrecondition, "webserver client is required")
	}
	if s.uploader == nil {
		return nil, status.Error(codes.FailedPrecondition, "minio uploader is required")
	}
	if s.downloadBucket == "" {
		return nil, status.Error(codes.FailedPrecondition, "download bucket is required")
	}
	// we will get the transcode options here
	transcodeOptions, err := s.wc.GetTranscodeProfile(ctx, req.JobId)
	if err != nil {
		s.logger.Error("Failed fetching transcode profile", "job_id", req.JobId, "error", err)
		return nil, status.Errorf(codes.Internal, "failed to fetch transcode profile: %v", err)
	}
	outFormat := strings.ToLower(anyToString(transcodeOptions["format"]))
	request := &pb.TranscodeRequest{
		JobId:        req.GetJobId(),
		OutputFormat: outFormat,
		Options: &pb.VideoOptions{
			Codec:      anyToString(transcodeOptions["codec"]),
			Bitrate:    anyToInt32(transcodeOptions["bitrate"]),
			Framerate:  anyToInt32(transcodeOptions["framerate"]),
			Resolution: anyToString(transcodeOptions["resolution"]),
		},
	}
	start := time.Now()
	jobID := req.GetJobId()
	currentStatus := webserver.StatusQueued
	outputFormat := req.GetOutputFormat()
	if outputFormat == "" {
		outputFormat = "mp4"
	}
	objectKey := filepath.Base(req.GetOutputPath())
	if objectKey == "" || objectKey == "." || objectKey == "/" {
		objectKey = fmt.Sprintf("%s.%s", jobID, outputFormat)
	}
	localOutputPath := filepath.Join("/tmp", objectKey)

	s.logger.Info("Transcode job started", "job_id", jobID, "output_format", outputFormat, "object_key", objectKey)

	if err := s.transitionStatus(ctx, jobID, currentStatus, webserver.StatusDownloading); err != nil {
		s.logger.Error("Failed transitioning status", "job_id", jobID, "from", currentStatus, "to", webserver.StatusDownloading, "error", err, "failure_reason", "status_update_failed")
		return nil, status.Errorf(codes.Internal, "failed to update job status to downloading: %v", err)
	}
	currentStatus = webserver.StatusDownloading

	transcodeCtx, cancel := context.WithTimeout(ctx, defaultTranscodeTimeout)
	defer cancel()

	inputURL, err := s.wc.GetSourceURL(transcodeCtx, jobID)
	if err != nil {
		s.logger.Error("Failed fetching source URL", "job_id", jobID, "error", err, "failure_reason", "source_url_fetch_failed")
		_ = s.transitionStatus(ctx, jobID, currentStatus, webserver.StatusFailed)
		return nil, status.Errorf(codes.Internal, "failed to fetch source url: %v", err)
	}

	if err := s.validateSourceVideo(transcodeCtx, inputURL); err != nil {
		s.logger.Error("Source validation failed", "job_id", jobID, "error", err, "failure_reason", "source_validation_failed")
		_ = s.transitionStatus(ctx, jobID, currentStatus, webserver.StatusFailed)
		return nil, status.Errorf(codes.InvalidArgument, "source video validation failed: %v", err)
	}

	if err := s.transitionStatus(ctx, jobID, currentStatus, webserver.StatusProcessing); err != nil {
		s.logger.Error("Failed transitioning status", "job_id", jobID, "from", currentStatus, "to", webserver.StatusProcessing, "error", err, "failure_reason", "status_update_failed")
		return nil, status.Errorf(codes.Internal, "failed to update job status to processing: %v", err)
	}
	currentStatus = webserver.StatusProcessing

	ffmpegArgs := buildFFmpegArgs(inputURL, localOutputPath, request)
	if err := s.runFFmpegWithRetry(transcodeCtx, ffmpegArgs); err != nil {
		s.logger.Error("ffmpeg transcode failed", "job_id", jobID, "error", err, "failure_reason", "ffmpeg_failed")
		_ = s.transitionStatus(ctx, jobID, currentStatus, webserver.StatusFailed)
		return nil, status.Errorf(codes.Internal, "ffmpeg failed: %v", err)
	}

	if err := s.transitionStatus(ctx, jobID, currentStatus, webserver.StatusUploading); err != nil {
		s.logger.Error("Failed transitioning status", "job_id", jobID, "from", currentStatus, "to", webserver.StatusUploading, "error", err, "failure_reason", "status_update_failed")
		return nil, status.Errorf(codes.Internal, "failed to update job status to uploading: %v", err)
	}
	currentStatus = webserver.StatusUploading

	if err := s.retryer.DoWithCheck(ctx, "upload_output", func() error {
		return s.uploadOutput(ctx, localOutputPath, objectKey, outputFormat)
	}, func(err error) bool {
		return err != nil
	}); err != nil {
		s.logger.Error("MinIO upload failed", "job_id", jobID, "bucket", s.downloadBucket, "object_key", objectKey, "error", err, "failure_reason", "upload_failed")
		_ = s.transitionStatus(ctx, jobID, currentStatus, webserver.StatusFailed)
		return nil, status.Errorf(codes.Internal, "failed to upload transcoded output to minio: %v", err)
	}

	if err := s.transitionStatus(ctx, jobID, currentStatus, webserver.StatusCompleted); err != nil {
		s.logger.Error("Failed transitioning status", "job_id", jobID, "from", currentStatus, "to", webserver.StatusCompleted, "error", err, "failure_reason", "status_update_failed")
		return nil, status.Errorf(codes.Internal, "failed to update job status to completed: %v", err)
	}

	s.logger.Info("Transcode job completed", "job_id", jobID, "duration_ms", time.Since(start).Milliseconds(), "output_path", objectKey)
	return &pb.TranscodeResponse{
		JobId:      jobID,
		Status:     "completed",
		Success:    true,
		OutputPath: objectKey,
		DurationMs: time.Since(start).Milliseconds(),
	}, nil
}

func (s *TranscoderService) runFFmpegWithRetry(ctx context.Context, ffmpegArgs []string) error {
	// Retry only process start/transport faults; do not retry deterministic ffmpeg exit failures.
	return s.retryer.DoWithCheck(ctx, "ffmpeg_transcode", func() error {
		cmd := exec.CommandContext(ctx, s.ffmpegPath, ffmpegArgs...)
		output, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("ffmpeg error: %w, output=%s", err, string(output))
		}
		return nil
	}, func(err error) bool {
		if err == nil {
			return false
		}
		var exitErr *exec.ExitError
		return !errors.As(err, &exitErr)
	})
}

func (s *TranscoderService) transitionStatus(ctx context.Context, jobID string, from, to webserver.Status) error {
	_, err := s.wc.UpdateJobStatus(ctx, webserver.JobStatusReq{JobID: jobID, From: from, To: to})
	return err
}

func buildFFmpegArgs(inputPath, outputPath string, req *pb.TranscodeRequest) []string {
	args := []string{"-y", "-i", inputPath}
	if opts := req.GetOptions(); opts != nil {
		if opts.GetCodec() != "" {
			switch opts.GetCodec() {
			case "h264":
				args = append(args, "-c:v", "libx264", "-preset", "veryfast")
			case "h265":
				args = append(args, "-c:v", "libx265", "-preset", "veryfast")
			case "vp9":
				args = append(args, "-c:v", "libvpx-vp9", "-b:v", "0", "-crf", "30")
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
			res := normalizeFFmpegResolution(opts.GetResolution())
			filter := fmt.Sprintf("scale=%s:flags=bicubic", res)
			args = append(args, "-vf", filter)
		}
	}
	if req.GetOutputFormat() != "" {
		args = append(args, "-f", req.GetOutputFormat())
	}
	maxThreadsPerJob := runtime.NumCPU() / 4
	if maxThreadsPerJob < 1 {
		maxThreadsPerJob = 1
	}
	args = append(args, "-threads", strconv.Itoa(maxThreadsPerJob))
	args = append(args, outputPath)
	return args
}

func normalizeFFmpegResolution(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "480", "480p":
		return "854x480"
	case "720", "720p":
		return "1280x720"
	case "1080", "1080p":
		return "1920x1080"
	default:
		return raw
	}
}

func (s *TranscoderService) uploadOutput(ctx context.Context, localPath, objectKey, outputFormat string) error {
	contentType := "application/octet-stream"
	if outputFormat != "" {
		contentType = "video/" + outputFormat
	}
	_, err := s.uploader.FPutObject(
		ctx,
		s.downloadBucket,
		objectKey,
		localPath,
		minio.PutObjectOptions{ContentType: contentType},
	)
	return err
}

func (s *TranscoderService) validateSourceVideo(ctx context.Context, inputURL string) error {
	cmd := exec.CommandContext(
		ctx,
		s.ffprobePath,
		"-v", "error",
		"-select_streams", "v:0",
		"-show_entries", "stream=codec_type",
		"-of", "default=nokey=1:noprint_wrappers=1",
		inputURL,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffprobe failed: %w, output=%s", err, string(out))
	}
	if len(out) == 0 {
		return fmt.Errorf("ffprobe produced empty output")
	}
	return nil
}

// Direct type assertions from untyped maps can panic if the data is not in the expected format.
func anyToInt32(v any) int32 {
	if v == nil {
		return 0
	}
	switch t := v.(type) {
	case int32:
		return t
	case int64:
		return int32(t)
	case int:
		return int32(t)
	case float64:
		return int32(t)
	case json.Number:
		if i64, err := t.Int64(); err == nil {
			return int32(i64)
		}
		if f64, err := t.Float64(); err == nil {
			return int32(f64)
		}
		return 0
	default:
		// fallback: try to parse from string
		s := fmt.Sprint(t)
		if i64, err := strconv.ParseInt(s, 10, 32); err == nil {
			return int32(i64)
		}
		if f64, err := strconv.ParseFloat(s, 64); err == nil {
			return int32(f64)
		}
		return 0
	}
}

func anyToString(v any) string {
	if v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return t
	case []byte:
		return string(t)
	case json.Number:
		return t.String()
	default:
		return fmt.Sprint(t)
	}

}
