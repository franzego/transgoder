// Package handler contains the HTTP handlers for the transcoder service.
package handler

import (
	"fmt"
	"net/http"

	"github.com/franzego/transgoder/internal/models"
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
		c.JSON(http.StatusInternalServerError, models.ApiMessage{
			Message: fmt.Sprintf("Failed to transition status: %s -> %s", req.From, req.To),
			Success: false,
			Code:    http.StatusInternalServerError,
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
