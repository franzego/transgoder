package service

import (
	"context"
	"net/http"
	"slices"

	"github.com/franzego/transgoder/internal/models"
	"github.com/franzego/transgoder/internal/sqlc"
)

var validTransitions = map[models.Status][]models.Status{
	models.StatusPending:     {models.StatusQueued, models.StatusCancelled, models.StatusFailed},
	models.StatusQueued:      {models.StatusDownloading, models.StatusCancelled, models.StatusFailed},
	models.StatusDownloading: {models.StatusProcessing, models.StatusCancelled, models.StatusFailed},
	models.StatusProcessing:  {models.StatusUploading, models.StatusCancelled, models.StatusFailed},
	models.StatusUploading:   {models.StatusCompleted, models.StatusCancelled, models.StatusFailed},
	models.StatusCompleted:   {},
	models.StatusFailed:      {},
	models.StatusCancelled:   {},
}

func canTransition(from, to models.Status) bool {
	transitions, ok := validTransitions[from]
	if !ok {
		return false
	}
	if slices.Contains(transitions, to) {
		return true
	}
	return false
}

// TransitionTo attempts to transition a job from current status to new status, returning an error if the transition is invalid.
func (r *RepoService) TransitionTo(ctx context.Context, jobId string, from, to models.Status) error {
	if !canTransition(from, to) {
		return &ServiceError{
			Err:     ErrInvalidTransition,
			Code:    http.StatusBadRequest,
			Message: "Invalid transition was attempted",
		}
	}
	_, err := r.repo.Q.UpdateJobStatus(ctx, sqlc.UpdateJobStatusParams{
		JobID:  jobId,
		Status: string(to),
	})
	if err != nil {
		return &ServiceError{
			Err:     err,
			Code:    http.StatusInternalServerError,
			Message: "Failed to update job status",
		}
	}
	return nil
}
