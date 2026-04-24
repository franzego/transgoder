package service

import (
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"

	"github.com/minio/minio-go/v7"

	"github.com/franzego/transcoder/grpc/connection"
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
}

func NewTranscoderService(logger *slog.Logger, wc *webserver.WebserverClient, minioClient *connection.MinioClient, downloadBucket, ffmpegPath, ffprobePath string) *TranscoderService {
	if logger == nil {
		ts := &TranscoderService{
			logger:         logger,
			wc:             wc,
			uploader:       minioClient,
			downloadBucket: downloadBucket,
			ffmpegPath:     ffmpegPath,
			ffprobePath:    ffprobePath,
		}
		return ts
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

	if err := s.transitionStatus(ctx, jobID, currentStatus, webserver.StatusDownloading); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to update job status to downloading: %v", err)
	}
	currentStatus = webserver.StatusDownloading

	// This is the presigned url for that particular jobid that will use to  download the videofile from minio to local tmp storage for transcoding
	// This should be carried out with a retry logic
	inputURL, err := s.wc.GetSourceURL(ctx, jobID)
	if err != nil {
		_ = s.transitionStatus(ctx, jobID, currentStatus, webserver.StatusFailed)
		return nil, status.Errorf(codes.Internal, "failed to fetch source url: %v", err)
	}
	// we validate that video source using ffprobe.
	if err := s.validateSourceVideo(ctx, inputURL); err != nil {
		_ = s.transitionStatus(ctx, jobID, currentStatus, webserver.StatusFailed)
		return nil, status.Errorf(codes.InvalidArgument, "source video validation failed: %v", err)
	}

	if err := s.transitionStatus(ctx, jobID, currentStatus, webserver.StatusProcessing); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to update job status to processing: %v", err)
	}
	currentStatus = webserver.StatusProcessing

	// the transcoding will happen here. We will use ffmpeg for transcoding and execute it as a subprocess.
	// The input will be the presigned url and the output will be stored in local tmp storage before uploading it back to minio.
	ffmpegArgs := buildFFmpegArgs(inputURL, localOutputPath, req)
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
	// we have to upload back to minio.
	// would have loved to have a different function for that.
	// But to keep the code simple and avoid too many abstractions, I am doing it in the same function for now.
	if err := s.uploadOutput(ctx, localOutputPath, objectKey, outputFormat); err != nil {
		_ = s.transitionStatus(ctx, jobID, currentStatus, webserver.StatusFailed)
		return nil, status.Errorf(codes.Internal, "failed to upload transcoded output to minio: %v", err)
	}

	if err := s.transitionStatus(ctx, jobID, currentStatus, webserver.StatusCompleted); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to update job status to completed: %v", err)
	}

	return &pb.TranscodeResponse{
		JobId:      jobID,
		Status:     "completed",
		Success:    true,
		OutputPath: objectKey, // this is the key/url that will be used to download the transcoded video from minio to the client.
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
