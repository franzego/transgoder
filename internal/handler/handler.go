package handler

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/franzego/transgoder/internal/models"
	"github.com/franzego/transgoder/internal/service"
	"github.com/franzego/transgoder/internal/sqlc"
	"github.com/franzego/transgoder/pkg"
	"github.com/gin-gonic/gin"
	"github.com/minio/minio-go/v7"
)

type Handler struct {
	minioService *service.MinioService
	service      service.ServiceRepository
	logger       *slog.Logger
	// we will add the services here later
}

func NewHandler(minioService *service.MinioService, service service.ServiceRepository, logger *slog.Logger) *Handler {
	return &Handler{
		minioService: minioService,
		service:      service,
		logger:       logger,
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
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ApiMessage{
			Success: false,
			Message: "Invalid request payload",
			Code:    400,
			Error:   err.Error(),
		})
		return
	}
	if req.FileSize <= 0 {
		c.JSON(http.StatusBadRequest, models.ApiMessage{
			Success: false,
			Message: "file_size must be greater than zero",
			Code:    400,
		})
		return
	}

	const (
		minPartSize     = int64(5 * 1024 * 1024)
		defaultPartSize = int64(64 * 1024 * 1024)
	)
	partSize := req.PartSize
	if partSize == 0 {
		partSize = defaultPartSize
	}
	if partSize < minPartSize {
		c.JSON(http.StatusBadRequest, models.ApiMessage{
			Success: false,
			Message: "part_size must be at least 5MB",
			Code:    400,
		})
		return
	}

	jobID := pkg.GenerateID()
	_, err := h.service.CreateJob(c.Request.Context(), sqlc.CreateJobParams{JobID: jobID})
	if err != nil {
		h.logger.Error("Failed to create job", "job_id", jobID, "error", err)
		c.JSON(http.StatusInternalServerError, models.ApiMessage{
			Success: false,
			Message: "Failed to create job",
			Code:    500,
			Error:   err.Error(),
		})
		return
	}

	objectName := jobID
	uploadID, err := h.minioService.NewMultipartUpload(c.Request.Context(), h.minioService.Cfg.UploadBucket, objectName)
	if err != nil {
		h.logger.Error("Failed to initiate multipart upload", "job_id", jobID, "error", err)
		c.JSON(http.StatusInternalServerError, models.ApiMessage{
			Success: false,
			Message: "Failed to initiate multipart upload",
			Code:    500,
			Error:   err.Error(),
		})
		return
	}

	totalParts := int((req.FileSize + partSize - 1) / partSize)
	urls := make([]map[string]any, 0, totalParts)

	for partNumber := 1; partNumber <= totalParts; partNumber++ {
		url, err := h.minioService.PresignedUploadPartURL(
			c.Request.Context(),
			h.minioService.Cfg.UploadBucket,
			objectName,
			uploadID,
			partNumber,
			60*time.Minute,
		)
		if err != nil {
			h.logger.Error("Failed to create presigned part URL", "job_id", jobID, "part", partNumber, "error", err)
			c.JSON(http.StatusInternalServerError, models.ApiMessage{
				Success: false,
				Message: "Failed to create presigned part URL",
				Code:    500,
				Error:   err.Error(),
			})
			return
		}

		// Store presigned URL in database
		_, err = h.service.CreatePresignedURL(c.Request.Context(), jobID, url, int32(partNumber))
		if err != nil {
			h.logger.Error("Failed to store presigned URL", "job_id", jobID, "part", partNumber, "error", err)
			c.JSON(http.StatusInternalServerError, models.ApiMessage{
				Success: false,
				Message: "Failed to store presigned URL",
				Code:    500,
				Error:   err.Error(),
			})
			return
		}

		urls = append(urls, map[string]any{
			"part_number": partNumber,
			"url":         url,
		})
	}

	c.JSON(http.StatusOK, models.ApiMessage{
		Success: true,
		Message: "Multipart upload initiated",
		Code:    200,
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
		c.JSON(http.StatusBadRequest, models.ApiMessage{
			Success: false,
			Message: "Invalid request payload",
			Code:    400,
			Error:   err.Error(),
		})
		return
	}
	if len(req.Parts) == 0 {
		c.JSON(http.StatusBadRequest, models.ApiMessage{
			Success: false,
			Message: "parts cannot be empty",
			Code:    400,
		})
		return
	}

	job, err := h.service.GetJobByJobID(c.Request.Context(), req.JobID)
	if err != nil {
		h.logger.Error("Failed to get job", "job_id", req.JobID, "error", err)
		c.JSON(http.StatusInternalServerError, models.ApiMessage{
			Success: false,
			Message: "Failed to get job",
			Code:    500,
			Error:   err.Error(),
		})
		return
	}

	parts := make([]minio.CompletePart, 0, len(req.Parts))
	for _, part := range req.Parts {
		parts = append(parts, minio.CompletePart{
			PartNumber: part.PartNumber,
			ETag:       part.ETag,
		})
	}

	objectName := req.JobID
	if err := h.minioService.CompleteMultipartUpload(
		c.Request.Context(),
		h.minioService.Cfg.UploadBucket,
		objectName,
		req.UploadID,
		parts,
	); err != nil {
		h.logger.Error("Failed to complete multipart upload", "job_id", req.JobID, "error", err)
		c.JSON(http.StatusInternalServerError, models.ApiMessage{
			Success: false,
			Message: "Failed to complete multipart upload",
			Code:    500,
			Error:   err.Error(),
		})
		return
	}

	_, err = h.service.CreateVideoMeta(c.Request.Context(), sqlc.CreateVideoMetaParams{
		JobID:       job.JobID,
		VideoName:   pkg.TextOrNull(req.VideoName),
		Description: pkg.TextOrNull(req.Description),
		Format:      pkg.TextOrNull(req.Format),
		Bitrate:     pkg.IntOrNull(req.Bitrate),
		Resolution:  pkg.TextOrNull(req.Resolution),
		Duration:    pkg.IntOrNull(req.Duration),
	})
	if err != nil {
		h.logger.Error("Failed to create video metadata", "job_id", req.JobID, "error", err)
		c.JSON(http.StatusInternalServerError, models.ApiMessage{
			Success: false,
			Message: "Failed to create video metadata",
			Code:    500,
			Error:   err.Error(),
		})
		return
	}

	_, err = h.service.UpdateJobStatus(c.Request.Context(), sqlc.UpdateJobStatusParams{
		ID:     job.ID,
		Status: string(models.StatusQueued),
	})
	if err != nil {
		h.logger.Error("Failed to update job status", "job_id", req.JobID, "error", err)
		c.JSON(http.StatusInternalServerError, models.ApiMessage{
			Success: false,
			Message: "Failed to update job status",
			Code:    500,
			Error:   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, models.ApiMessage{
		Success: true,
		Message: "Multipart upload completed and metadata stored",
		Code:    200,
		Metadata: map[string]any{
			"video_id": req.JobID,
			// "video_id": meta.ID,
			"status": "Currently queued for transcoding. It may take a few minutes.",
		},
	})
}
