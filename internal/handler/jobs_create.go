package handler

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/franzego/transcoder/internal/models"
	"github.com/franzego/transcoder/pkg"
	"github.com/gin-gonic/gin"
)

// CreateJob godoc
// @Summary Create and enqueue a transcode job
// @Description Creates a job with preset-based settings and enqueues it for processing.
// @Tags jobs
// @Accept json
// @Produce json
// @Param request body models.CreateJobRequest true "Create job request"
// @Success 201 {object} models.ApiMessage
// @Failure 400 {object} models.ApiMessage
// @Failure 500 {object} models.ApiMessage
// @Router /jobs [post]
func (h *Handler) CreateJob(c *gin.Context) {
	var req models.CreateJobRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ApiMessage{Success: false, Message: "Invalid request payload", Code: http.StatusBadRequest, ErrorCode: models.ErrorCodeValidation, Error: err.Error()})
		return
	}

	jobID := strings.TrimSpace(req.JobID)
	if jobID == "" {
		jobID = pkg.GenerateID()
	}
	job, err := h.service.CreateJob(c.Request.Context(), jobID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ApiMessage{Success: false, Message: "failed to create job", Code: http.StatusInternalServerError, ErrorCode: models.ErrorCodeInternal, Error: err.Error()})
		return
	}

	metaData, err := h.buildVideoMetadata(jobID, req.VideoName, req.Description, req.PresetID, req.Overrides, "", "", "", nil, req.Duration)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ApiMessage{Success: false, Message: "Invalid preset/overrides", Code: http.StatusBadRequest, ErrorCode: models.ErrorCodePresetOverride, Error: err.Error()})
		return
	}
	if _, err = h.service.CreateVideoMeta(c.Request.Context(), metaData); err != nil {
		c.JSON(http.StatusInternalServerError, models.ApiMessage{Success: false, Message: "failed to store video metadata", Code: http.StatusInternalServerError, ErrorCode: models.ErrorCodeInternal, Error: err.Error()})
		return
	}
	if err := h.redisService.Enqueue(c.Request.Context(), jobID); err != nil {
		c.JSON(http.StatusInternalServerError, models.ApiMessage{Success: false, Message: "failed to enqueue job", Code: http.StatusInternalServerError, ErrorCode: models.ErrorCodeDependency, Error: err.Error()})
		return
	}
	if err := h.service.TransitionTo(c.Request.Context(), jobID, models.StatusPending, models.StatusQueued); err != nil {
		c.JSON(http.StatusInternalServerError, models.ApiMessage{Success: false, Message: "failed to transition job status", Code: http.StatusInternalServerError, ErrorCode: models.ErrorCodeInvalidState, Error: err.Error()})
		return
	}

	c.JSON(http.StatusCreated, models.ApiMessage{
		Success: true,
		Message: "Job created and queued",
		Code:    http.StatusCreated,
		Metadata: map[string]any{
			"job_id":          jobID,
			"job_db_id":       job.ID,
			"preset_id":       req.PresetID,
			"expected_source": fmt.Sprintf("%s/%s", h.minioService.UploadBucket(), jobID),
		},
	})
}
