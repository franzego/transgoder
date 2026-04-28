// Package handler contains the HTTP handlers for the transcoder service.
package handler

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	transcoderpb "github.com/franzego/transcoder/grpc/server"
	"github.com/franzego/transcoder/internal/models"
	"github.com/franzego/transcoder/internal/service"
	"github.com/gin-gonic/gin"
)

// UpdateStatus godoc
// @Summary Update job status
// @Description Transition a job from one status to another
// @Tags status
// @Accept json
// @Produce json
// @Param id path string true "Job ID"
// @Param request body models.UpdateStatusRequest true "Status transition payload"
// @Success 200 {object} models.ApiMessage "Status updated successfully"
// @Failure 400 {object} models.ApiMessage "Invalid request payload"
// @Failure 500 {object} models.ApiMessage "Internal server error"
// @Router /status/{id}/update [post]
func (h *Handler) UpdateStatus(c *gin.Context) {
	var req models.UpdateStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.Error("failed to parse request", "error", err)
		c.JSON(http.StatusBadRequest, models.ApiMessage{
			Message: "Failed to parse request",
			Success: false,
			Code:    http.StatusBadRequest,
		})
		return
	}
	if err := h.validator.Struct(&req); err != nil {
		h.logger.Error("failed to validate request", "error", err)
		c.JSON(http.StatusBadRequest, models.ApiMessage{
			Message: "Failed to validate request",
			Success: false,
			Code:    http.StatusBadRequest,
		})
		return
	}
	if err := h.service.TransitionTo(c, req.JobID, req.From, req.To); err != nil {
		h.logger.Error("failed to transition status", "error", err)
		statusCode := http.StatusInternalServerError
		errorCode := models.ErrorCodeInternal
		if serr := new(service.ServiceError); errors.As(err, &serr) {
			if serr.Code >= 400 {
				statusCode = serr.Code
			}
			if statusCode == http.StatusBadRequest || statusCode == http.StatusConflict {
				errorCode = models.ErrorCodeInvalidState
			}
		}
		c.JSON(statusCode, models.ApiMessage{
			Message:   fmt.Sprintf("Failed to transition status: %s -> %s", req.From, req.To),
			Success:   false,
			Code:      statusCode,
			ErrorCode: errorCode,
			Error:     err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, models.ApiMessage{
		Message: fmt.Sprintf("Status updated successfully: %s -> %s", req.From, req.To),
		Success: true,
		Code:    http.StatusOK,
	})
}

// GetJobStatus godoc
// @Summary Get job status
// @Description Retrieve the current status for a job
// @Tags status
// @Produce json
// @Param id path string true "Job ID"
// @Success 200 {object} models.ApiMessage "Job status retrieved successfully"
// @Failure 500 {object} models.ApiMessage "Internal server error"
// @Router /status/{id}/update [get]
func (h *Handler) GetJobStatus(c *gin.Context) {
	jobID := c.Param("id")
	job, err := h.service.GetJobByJobID(c, jobID)
	if err != nil {
		h.logger.Error("failed to get job status", "error", err, "job_id", jobID)
		c.JSON(http.StatusInternalServerError, models.ApiMessage{
			Message: "Failed to get job status",
			Success: false,
			Code:    http.StatusInternalServerError,
		})
		return
	}
	c.JSON(http.StatusOK, models.ApiMessage{
		Message:  "Job status retrieved successfully",
		Success:  true,
		Code:     http.StatusOK,
		Metadata: models.JobStatusResponse{Status: job.Status},
	})
}

// GetSourceVideoURL godoc
// @Summary Get source video URL
// @Description Retrieve a presigned GET URL for a job's uploaded source video
// @Tags jobs
// @Produce json
// @Param id path string true "Job ID"
// @Success 200 {object} models.ApiMessage "Source URL retrieved successfully"
// @Failure 500 {object} models.ApiMessage "Internal server error"
// @Router /jobs/{id}/source-url [get]
func (h *Handler) GetSourceVideoURL(c *gin.Context) {
	jobID := c.Param("id")
	if _, err := h.service.GetJobByJobID(c, jobID); err != nil {
		h.logger.Error("failed to get job", "error", err, "job_id", jobID)
		c.JSON(http.StatusInternalServerError, models.ApiMessage{
			Message: "Failed to get job",
			Success: false,
			Code:    http.StatusInternalServerError,
		})
		return
	}

	sourceURL, err := h.minioService.GetPresignedURL(c.Request.Context(), h.minioService.UploadBucket(), jobID)
	if err != nil {
		h.logger.Error("failed to generate source presigned url", "error", err, "job_id", jobID)
		c.JSON(http.StatusInternalServerError, models.ApiMessage{
			Message: "Failed to generate source URL",
			Success: false,
			Code:    http.StatusInternalServerError,
		})
		return
	}

	c.JSON(http.StatusOK, models.ApiMessage{
		Message: "Source URL retrieved successfully",
		Success: true,
		Code:    http.StatusOK,
		Metadata: map[string]any{
			"job_id":     jobID,
			"source_url": sourceURL,
		},
	})
}

// GetOutputVideoURL godoc
// @Summary Get output video URL
// @Description Retrieve a presigned GET URL for a job's transcoded output video
// @Tags jobs
// @Produce json
// @Param id path string true "Job ID"
// @Success 200 {object} models.ApiMessage "Output URL retrieved successfully"
// @Failure 409 {object} models.ApiMessage "Job is not ready for download"
// @Failure 500 {object} models.ApiMessage "Internal server error"
// @Router /jobs/{id}/output-url [get]
func (h *Handler) GetOutputVideoURL(c *gin.Context) {
	jobID := c.Param("id")
	job, err := h.service.GetJobByJobID(c, jobID)
	if err != nil {
		h.logger.Error("failed to get job", "error", err, "job_id", jobID)
		c.JSON(http.StatusInternalServerError, models.ApiMessage{
			Message: "Failed to get job",
			Success: false,
			Code:    http.StatusInternalServerError,
		})
		return
	}
	if strings.ToLower(job.Status) != string(models.StatusCompleted) {
		c.JSON(http.StatusConflict, models.ApiMessage{
			Message: "Job is not ready for download",
			Success: false,
			Code:    http.StatusConflict,
		})
		return
	}

	objectKey, outputURL, err := h.resolveOutputURL(c, jobID)
	if err != nil {
		h.logger.Error("failed to generate output presigned url", "error", err, "job_id", jobID)
		c.JSON(http.StatusInternalServerError, models.ApiMessage{
			Message: "Failed to generate output URL",
			Success: false,
			Code:    http.StatusInternalServerError,
		})
		return
	}

	c.JSON(http.StatusOK, models.ApiMessage{
		Message: "Output URL retrieved successfully",
		Success: true,
		Code:    http.StatusOK,
		Metadata: map[string]any{
			"job_id":     jobID,
			"output_url": outputURL,
			"object_key": objectKey,
		},
	})
}

// DownloadOutputVideo godoc
// @Summary Download output video
// @Description Trigger transcode if needed and stream the transcoded video to the client
// @Tags jobs
// @Produce application/octet-stream
// @Param id path string true "Job ID"
// @Success 200 {file} file "Video stream"
// @Failure 500 {object} models.ApiMessage "Internal server error"
// @Router /jobs/{id}/download [get]
func (h *Handler) DownloadOutputVideo(c *gin.Context) {
	jobID := c.Param("id")
	job, err := h.service.GetJobByJobID(c.Request.Context(), jobID)
	if err != nil {
		h.logger.Error("failed to get job", "error", err, "job_id", jobID)
		c.JSON(http.StatusInternalServerError, models.ApiMessage{
			Message: "Failed to get job",
			Success: false,
			Code:    http.StatusInternalServerError,
		})
		return
	}

	if strings.ToLower(job.Status) != string(models.StatusCompleted) {
		if h.grpcClient == nil {
			c.JSON(http.StatusInternalServerError, models.ApiMessage{
				Message: "gRPC transcoder client not configured",
				Success: false,
				Code:    http.StatusInternalServerError,
			})
			return
		}
		meta, err := h.service.GetVideoMetaByJobID(c.Request.Context(), jobID)
		if err != nil {
			h.logger.Error("failed to get video metadata for transcode", "error", err, "job_id", jobID)
			c.JSON(http.StatusInternalServerError, models.ApiMessage{
				Message: "Failed to get video metadata",
				Success: false,
				Code:    http.StatusInternalServerError,
			})
			return
		}
		req := &transcoderpb.TranscodeRequest{
			JobId:        jobID,
			OutputFormat: strings.ToLower(meta.Format.String),
			Options: &transcoderpb.VideoOptions{
				Codec: meta.Codec,
			},
		}
		if meta.Bitrate.Valid {
			req.Options.Bitrate = meta.Bitrate.Int32
		}
		if meta.Framerate.Valid {
			req.Options.Framerate = meta.Framerate.Int32
		}
		if meta.Resolution.Valid {
			req.Options.Resolution = meta.Resolution.String
		}
		resp, err := h.grpcClient.TranscodeVideo(c.Request.Context(), req)
		if err != nil || resp == nil || !resp.Success {
			h.logger.Error("transcode rpc failed", "error", err, "job_id", jobID)
			c.JSON(http.StatusInternalServerError, models.ApiMessage{
				Message: "Failed to transcode video",
				Success: false,
				Code:    http.StatusInternalServerError,
			})
			return
		}
	}

	objectKey, outputURL, err := h.resolveOutputURL(c, jobID)
	if err != nil {
		h.logger.Error("failed to resolve output url", "error", err, "job_id", jobID)
		c.JSON(http.StatusInternalServerError, models.ApiMessage{
			Message: "Failed to get output URL",
			Success: false,
			Code:    http.StatusInternalServerError,
		})
		return
	}

	req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodGet, outputURL, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ApiMessage{
			Message: "Failed to create download request",
			Success: false,
			Code:    http.StatusInternalServerError,
		})
		return
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ApiMessage{
			Message: "Failed to download output video",
			Success: false,
			Code:    http.StatusInternalServerError,
		})
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		c.JSON(http.StatusInternalServerError, models.ApiMessage{
			Message: "Output download failed",
			Success: false,
			Code:    http.StatusInternalServerError,
		})
		return
	}

	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	c.Header("Content-Type", contentType)
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", objectKey))
	c.Status(http.StatusOK)
	if _, err := io.Copy(c.Writer, resp.Body); err != nil {
		h.logger.Error("failed to stream output video", "error", err, "job_id", jobID)
	}
}

func (h *Handler) resolveOutputURL(c *gin.Context, jobID string) (string, string, error) {
	format := "mp4"
	if meta, err := h.service.GetVideoMetaByJobID(c, jobID); err == nil && meta.Format.Valid && meta.Format.String != "" {
		format = strings.ToLower(meta.Format.String)
	}
	objectKey := fmt.Sprintf("%s.%s", jobID, format)
	outputURL, err := h.minioService.GetPresignedURL(c.Request.Context(), h.minioService.DownloadBucket(), objectKey)
	if err != nil {
		return "", "", err
	}
	return objectKey, outputURL, nil
}

// GetTranscodeProfile godoc
// @Summary Get transcode profile
// @Description Retrieve resolved transcode parameters for a job (used by workers)
// @Tags jobs
// @Produce json
// @Param id path string true "Job ID"
// @Success 200 {object} models.ApiMessage
// @Failure 500 {object} models.ApiMessage
// @Router /jobs/{id}/transcode-profile [get]
func (h *Handler) GetTranscodeProfile(c *gin.Context) {
	jobID := c.Param("id")
	meta, err := h.service.GetVideoMetaByJobID(c.Request.Context(), jobID)
	if err != nil {
		h.logger.Error("failed to get video metadata", "error", err, "job_id", jobID)
		c.JSON(http.StatusInternalServerError, models.ApiMessage{
			Message:   "Failed to get video metadata",
			Success:   false,
			Code:      http.StatusInternalServerError,
			ErrorCode: models.ErrorCodeInternal,
		})
		return
	}

	payload := map[string]any{
		"job_id":     jobID,
		"format":     strings.ToLower(meta.Format.String),
		"codec":      meta.Codec,
		"bitrate":    int32(0),
		"resolution": meta.Resolution.String,
		"framerate":  meta.Framerate.Int32,
	}
	if meta.Resolution.Valid {
		payload["resolution"] = meta.Resolution.String
	}
	if meta.Bitrate.Valid {
		payload["bitrate"] = meta.Bitrate.Int32
	}
	if meta.Framerate.Valid {
		payload["framerate"] = meta.Framerate.Int32
	}

	c.JSON(http.StatusOK, models.ApiMessage{
		Message:  "Transcode profile fetched",
		Success:  true,
		Code:     http.StatusOK,
		Metadata: payload,
	})
}
