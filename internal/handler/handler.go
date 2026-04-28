package handler

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	transcoderpb "github.com/franzego/transcoder/grpc/server"
	"github.com/franzego/transcoder/internal/models"
	"github.com/franzego/transcoder/internal/presets"
	"github.com/franzego/transcoder/internal/service"
	"github.com/franzego/transcoder/internal/sqlc"
	"github.com/franzego/transcoder/pkg"
	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
	"github.com/minio/minio-go/v7"
)

type ServiceRepository interface {
	CreateJob(ctx context.Context, jobID string) (sqlc.Job, error)
	CreatePresignedURL(ctx context.Context, jobID, presignedUrl string, partNumber int32) (sqlc.PresignedUrl, error)
	GetJobByJobID(ctx context.Context, jobID string) (sqlc.Job, error)
	GetVideoMetaByJobID(ctx context.Context, jobID string) (sqlc.Videometum, error)
	CreateVideoMeta(ctx context.Context, arg models.VideoMedataReq) (sqlc.Videometum, error)
	DeleteJob(ctx context.Context, id int32) error
	TransitionTo(ctx context.Context, jobId string, from, to models.Status) error
}

// for minio
type MultipartService interface {
	UploadBucket() string
	DownloadBucket() string
	GetPresignedURL(ctx context.Context, bucketName, jobID string) (string, error)
	NewMultipartUpload(ctx context.Context, bucketName, objectName string) (string, error)
	PresignedUploadPartURL(ctx context.Context, bucketName, objectName, uploadID string, partNumber int, expires time.Duration) (string, error)
	CompleteMultipartUpload(ctx context.Context, bucketName, objectName, uploadID string, parts []minio.CompletePart) error
	AbortMultipartUpload(ctx context.Context, bucketName, objectName, uploadID string) error
}

// For redis queue
type Queuer interface {
	Enqueue(ctx context.Context, jobID string) error
	Dequeue(ctx context.Context, workerID string) (string, string, error)
}

type TranscoderClient interface {
	TranscodeVideo(ctx context.Context, req *transcoderpb.TranscodeRequest) (*transcoderpb.TranscodeResponse, error)
}

type Handler struct {
	minioService MultipartService
	service      ServiceRepository
	logger       *slog.Logger
	redisService Queuer
	grpcClient   TranscoderClient
	validator    *validator.Validate
}

func NewHandler(minioService MultipartService, service ServiceRepository, redisService Queuer, grpcClient TranscoderClient, logger *slog.Logger, validator *validator.Validate) *Handler {
	return &Handler{
		minioService: minioService,
		service:      service,
		logger:       logger,
		redisService: redisService,
		grpcClient:   grpcClient,
		validator:    validator,
	}
}

// InitiateMultipartUploadHandler godoc
// @Summary Initiate a multipart upload
// @Description Start a new multipart upload for a video file
// @Tags uploads
// @Accept json
// @Produce json
// @Param request body models.MultipartInitiateRequest true "Upload initiation request"
// @Success 200 {object} models.ApiMessage "Upload initiated successfully"
// @Failure 400 {object} models.ApiMessage "Invalid request payload"
// @Failure 500 {object} models.ApiMessage "Internal server error"
// @Router /upload/initiate [post]
func (h *Handler) InitiateMultipartUploadHandler(c *gin.Context) {
	var req models.MultipartInitiateRequest
	var srvError *service.ServiceError
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ApiMessage{
			Success:   false,
			Message:   "Invalid request payload",
			Code:      http.StatusBadRequest,
			ErrorCode: models.ErrorCodeValidation,
			Error:     err.Error(),
		})
		return
	}
	if req.FileSize <= 0 {
		c.JSON(http.StatusBadRequest, models.ApiMessage{Success: false, Message: "file_size must be greater than zero", Code: http.StatusBadRequest, ErrorCode: models.ErrorCodeValidation})
		return
	}

	const (
		minPartSize     = int64(5 * 1024 * 1024)
		defaultPartSize = int64(64 * 1024 * 1024)
		maxParts        = int64(10000)
		maxFileSize     = int64(5 * 1024 * 1024 * 1024 * 1024)
	)
	partSize := req.PartSize
	if partSize == 0 {
		partSize = defaultPartSize
	}
	if partSize < minPartSize {
		c.JSON(http.StatusBadRequest, models.ApiMessage{Success: false, Message: "part_size must be at least 5MB", Code: http.StatusBadRequest, ErrorCode: models.ErrorCodeValidation})
		return
	}
	if req.FileSize > maxFileSize {
		c.JSON(http.StatusBadRequest, models.ApiMessage{Success: false, Message: "file_size exceeds 5TB limit", Code: http.StatusBadRequest, ErrorCode: models.ErrorCodeValidation})
		return
	}
	totalParts := (req.FileSize + partSize - 1) / partSize
	if totalParts > maxParts {
		c.JSON(http.StatusBadRequest, models.ApiMessage{Success: false, Message: "file_size/part_size creates too many parts (max 10000)", Code: http.StatusBadRequest, ErrorCode: models.ErrorCodeValidation})
		return
	}

	jobID := pkg.GenerateID()
	job, err := h.service.CreateJob(c.Request.Context(), jobID)
	if err != nil {
		if errors.As(err, &srvError) {
			h.logger.Error("Failed to create job", "job_id", jobID, "error", &srvError)
			c.JSON(http.StatusInternalServerError, models.ApiMessage{Success: false, Message: "Failed to create job", Code: http.StatusInternalServerError, ErrorCode: models.ErrorCodeInternal, Error: err.Error()})
			return
		}
		h.logger.Error("Unexpected error when creating job", "job_id", jobID, "error", err)
		c.JSON(http.StatusInternalServerError, models.ApiMessage{Success: false, Message: "An unexpected error has occurred while creating the job. Please contact support.", Code: http.StatusInternalServerError, ErrorCode: models.ErrorCodeInternal, Error: err.Error()})
		return
	}

	objectName := jobID
	uploadID, err := h.minioService.NewMultipartUpload(c.Request.Context(), h.minioService.UploadBucket(), objectName)
	if err != nil {
		if cleanupErr := h.service.DeleteJob(c.Request.Context(), job.ID); cleanupErr != nil {
			h.logger.Error("Failed to cleanup job after upload init error", "job_id", jobID, "error", cleanupErr)
		}
		h.logger.Error("Failed to initiate multipart upload", "job_id", jobID, "error", err)
		c.JSON(http.StatusInternalServerError, models.ApiMessage{Success: false, Message: "Failed to initiate multipart upload", Code: http.StatusInternalServerError, ErrorCode: models.ErrorCodeDependency, Error: err.Error()})
		return
	}

	cleanupFailedInitiation := func(cause error) {
		abortErr := h.minioService.AbortMultipartUpload(c.Request.Context(), h.minioService.UploadBucket(), objectName, uploadID)
		if abortErr != nil {
			h.logger.Error("Failed to abort multipart upload during cleanup", "job_id", jobID, "upload_id", uploadID, "error", abortErr)
		}
		deleteErr := h.service.DeleteJob(c.Request.Context(), job.ID)
		if deleteErr != nil {
			h.logger.Error("Failed to delete job during cleanup", "job_id", jobID, "error", deleteErr)
		}
		h.logger.Error("Initiation cleanup completed after failure", "job_id", jobID, "upload_id", uploadID, "cause", cause)
	}

	urls := make([]map[string]any, 0, int(totalParts))
	for partNumber := int64(1); partNumber <= totalParts; partNumber++ {
		url, err := h.minioService.PresignedUploadPartURL(c.Request.Context(), h.minioService.UploadBucket(), objectName, uploadID, int(partNumber), 60*time.Minute)
		if err != nil {
			cleanupFailedInitiation(err)
			h.logger.Error("Failed to create presigned part URL", "job_id", jobID, "part", partNumber, "error", err)
			c.JSON(http.StatusInternalServerError, models.ApiMessage{Success: false, Message: "Failed to create presigned part URL", Code: http.StatusInternalServerError, ErrorCode: models.ErrorCodeDependency, Error: err.Error()})
			return
		}

		_, err = h.service.CreatePresignedURL(c.Request.Context(), jobID, url, int32(partNumber))
		if err != nil {
			cleanupFailedInitiation(err)
			h.logger.Error("Failed to store presigned URL", "job_id", jobID, "part", partNumber, "error", err)
			c.JSON(http.StatusInternalServerError, models.ApiMessage{Success: false, Message: "Failed to store presigned URL", Code: http.StatusInternalServerError, ErrorCode: models.ErrorCodeInternal, Error: err.Error()})
			return
		}

		urls = append(urls, map[string]any{"part_number": int(partNumber), "url": url})
	}

	c.JSON(http.StatusOK, models.ApiMessage{
		Success: true,
		Message: "Multipart upload initiated",
		Code:    http.StatusOK,
		Metadata: map[string]any{
			"job_id":     jobID,
			"upload_id":  uploadID,
			"object_key": objectName,
			"part_size":  partSize,
			"parts":      urls,
		},
	})
}

// CompleteMultipartUploadHandler godoc
// @Summary Complete a multipart upload
// @Description Finish uploading video parts and create metadata
// @Tags uploads
// @Accept json
// @Produce json
// @Param request body models.MultipartCompleteRequest true "Upload completion request"
// @Success 200 {object} models.ApiMessage "Upload completed successfully"
// @Failure 400 {object} models.ApiMessage "Invalid request payload"
// @Failure 500 {object} models.ApiMessage "Internal server error"
// @Router /upload/complete [post]
func (h *Handler) CompleteMultipartUploadHandler(c *gin.Context) {
	var req models.MultipartCompleteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ApiMessage{Success: false, Message: "Invalid request payload", Code: http.StatusBadRequest, ErrorCode: models.ErrorCodeValidation, Error: err.Error()})
		return
	}
	if len(req.Parts) == 0 {
		c.JSON(http.StatusBadRequest, models.ApiMessage{Success: false, Message: "parts cannot be empty", Code: http.StatusBadRequest, ErrorCode: models.ErrorCodeValidation})
		return
	}

	job, err := h.service.GetJobByJobID(c.Request.Context(), req.JobID)
	if err != nil {
		h.logger.Error("Failed to get job", "job_id", req.JobID, "error", err)
		c.JSON(http.StatusInternalServerError, models.ApiMessage{Success: false, Message: "Failed to get job", Code: http.StatusInternalServerError, ErrorCode: models.ErrorCodeInternal, Error: err.Error()})
		return
	}

	parts := make([]minio.CompletePart, 0, len(req.Parts))
	for _, part := range req.Parts {
		parts = append(parts, minio.CompletePart{PartNumber: part.PartNumber, ETag: part.ETag})
	}

	if err := h.minioService.CompleteMultipartUpload(c.Request.Context(), h.minioService.UploadBucket(), req.JobID, req.UploadID, parts); err != nil {
		h.logger.Error("Failed to complete multipart upload", "job_id", req.JobID, "error", err)
		c.JSON(http.StatusInternalServerError, models.ApiMessage{Success: false, Message: "Failed to complete multipart upload", Code: http.StatusInternalServerError, ErrorCode: models.ErrorCodeDependency, Error: err.Error()})
		return
	}

	metaData, err := h.buildVideoMetadata(job.JobID, req.VideoName, req.Description, req.PresetID, req.Overrides, req.Format, req.Resolution, req.Codec, req.Framerate, req.Duration)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ApiMessage{Success: false, Message: "Invalid preset/overrides", Code: http.StatusBadRequest, ErrorCode: models.ErrorCodePresetOverride, Error: err.Error()})
		return
	}

	if _, err = h.service.CreateVideoMeta(c.Request.Context(), metaData); err != nil {
		h.logger.Error("Failed to create video metadata", "job_id", req.JobID, "error", err)
		c.JSON(http.StatusInternalServerError, models.ApiMessage{Success: false, Message: "Failed to create video metadata", Code: http.StatusInternalServerError, ErrorCode: models.ErrorCodeInternal, Error: err.Error()})
		return
	}

	if err := h.redisService.Enqueue(c.Request.Context(), req.JobID); err != nil {
		h.logger.Error("Failed to enqueue job in redis", "job_id", req.JobID, "error", err)
		c.JSON(http.StatusInternalServerError, models.ApiMessage{Success: false, Message: "An error has occured while queuing the job for transcoding. Please contact support.", Code: http.StatusInternalServerError, ErrorCode: models.ErrorCodeDependency, Error: err.Error()})
		return
	}
	if err := h.service.TransitionTo(c.Request.Context(), job.JobID, models.StatusPending, models.StatusQueued); err != nil {
		h.logger.Error("Failed to update job status", "job_id", req.JobID, "error", err)
		c.JSON(http.StatusInternalServerError, models.ApiMessage{Success: false, Message: "Failed to update job status", Code: http.StatusInternalServerError, ErrorCode: models.ErrorCodeInvalidState, Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, models.ApiMessage{
		Success: true,
		Message: "Multipart upload completed and metadata stored",
		Code:    http.StatusOK,
		Metadata: map[string]any{
			"video_id": req.JobID,
			"filename": req.VideoName,
			"status":   "Currently queued for transcoding. It may take a few minutes.",
		},
	})
}

func (h *Handler) buildVideoMetadata(jobID, videoName, description, presetID string, overrides models.PresetOverrides, format, resolution, codec string, framerate, duration *int32) (models.VideoMedataReq, error) {
	if strings.TrimSpace(presetID) != "" {
		resolved, err := presets.Resolve(presetID, presets.Overrides{
			Codec:      overrides.Codec,
			Resolution: overrides.Resolution,
			Bitrate:    overrides.Bitrate,
			Framerate:  overrides.Framerate,
			Format:     overrides.Format,
		})
		if err != nil {
			return models.VideoMedataReq{}, err
		}
		return models.VideoMedataReq{
			JobID:       jobID,
			VideoName:   pkg.TextOrNull(videoName),
			Description: pkg.TextOrNull(description),
			Format:      pkg.TextOrNull(resolved.Format),
			Bitrate:     pkg.IntOrNull(intPtr(resolved.Options.Bitrate)),
			Resolution:  pkg.TextOrNull(resolved.Options.Resolution),
			Codec:       resolved.Options.Codec,
			Framerate:   pkg.IntOrNull(intPtr(resolved.Options.Framerate)),
			Duration:    pkg.IntOrNull(duration),
		}, nil
	}

	codec = strings.TrimSpace(codec)
	if codec == "" {
		codec = "h264"
	}
	res, err := normalizeResolutionPreset(resolution)
	if err != nil {
		return models.VideoMedataReq{}, err
	}
	return models.VideoMedataReq{
		JobID:       jobID,
		VideoName:   pkg.TextOrNull(videoName),
		Description: pkg.TextOrNull(description),
		Format:      pkg.TextOrNull(format),
		Bitrate:     pkg.IntOrNull(overrides.Bitrate),
		Resolution:  pkg.TextOrNull(res),
		Codec:       codec,
		Framerate:   pkg.IntOrNull(framerate),
		Duration:    pkg.IntOrNull(duration),
	}, nil
}

func intPtr(v int32) *int32 {
	vv := v
	return &vv
}

func normalizeResolutionPreset(raw string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "1080", "1080p", "1920x1080":
		return "1080", nil
	case "720", "720p", "1280x720":
		return "720", nil
	case "480", "480p", "854x480":
		return "480", nil
	default:
		return "", fmt.Errorf("resolution must be one of 480, 720, 1080")
	}
}
