package service

import (
	"context"
	"fmt"

	"github.com/franzego/transgoder/internal/repository"
	"github.com/franzego/transgoder/internal/sqlc"
)

type RepoService struct {
	repo *repository.Repo
}

func NewRepoService(repo *repository.Repo) *RepoService {
	return &RepoService{
		repo: repo,
	}
}

func (r *RepoService) CreateJob(ctx context.Context, arg sqlc.CreateJobParams) (sqlc.Job, error) {
	if arg.JobID == "" {
		return sqlc.Job{}, ErrInvalidJobID
	}
	job, err := r.repo.Q.CreateJob(ctx, arg)
	if err != nil {
		return sqlc.Job{}, &ServiceError{
			Err:     err,
			Code:    500,
			Message: "failed to create job",
		}
	}
	return job, nil
}

func (r *RepoService) GetJobByJobID(ctx context.Context, jobID string) (sqlc.Job, error) {
	if jobID == "" {
		return sqlc.Job{}, ErrInvalidJobID
	}
	job, err := r.repo.Q.GetJobByJobID(ctx, jobID)
	if err != nil {
		return sqlc.Job{}, &ServiceError{
			Err:     err,
			Code:    500,
			Message: "failed to get job",
		}
	}
	return job, nil
}

func (r *RepoService) UpdateJobStatus(ctx context.Context, arg sqlc.UpdateJobStatusParams) (sqlc.Job, error) {
	if arg.JobID == "" {
		return sqlc.Job{}, ErrInvalidJobID
	}
	job, err := r.repo.Q.UpdateJobStatus(ctx, arg)
	if err != nil {
		return sqlc.Job{}, &ServiceError{
			Err:     err,
			Code:    500,
			Message: "failed to update job status",
		}
	}
	return job, nil
}

func (r *RepoService) CreateVideoMeta(ctx context.Context, arg sqlc.CreateVideoMetaParams) (sqlc.Videometum, error) {
	if arg.JobID == "" {
		return sqlc.Videometum{}, ErrInvalidJobID
	}
	item, err := r.repo.Q.CreateVideoMeta(ctx, arg)
	if err != nil {
		return sqlc.Videometum{}, &ServiceError{
			Err:     err,
			Code:    500,
			Message: "failed to create video metadata",
		}
	}
	return item, nil
}

func (r *RepoService) CreatePresignedURL(ctx context.Context, jobID, presignedUrl string, partNumber int32) (sqlc.PresignedUrl, error) {
	if jobID == "" {
		return sqlc.PresignedUrl{}, ErrInvalidJobID
	}

	// Get the job by jobID to get the internal ID
	job, err := r.repo.Q.GetJobByJobID(ctx, jobID)
	if err != nil {
		return sqlc.PresignedUrl{}, &ServiceError{
			Err:     err,
			Code:    500,
			Message: "failed to get job",
		}
	}

	arg := sqlc.CreatePresignedURLParams{
		JobID:        job.JobID,
		PresignedUrl: presignedUrl,
		PartNumber:   partNumber,
	}
	item, err := r.repo.Q.CreatePresignedURL(ctx, arg)
	if err != nil {
		return sqlc.PresignedUrl{}, &ServiceError{
			Err:     err,
			Code:    500,
			Message: "failed to create presigned URL",
		}
	}
	return item, nil
}

func (r *RepoService) GetPresignedURLsByJobID(ctx context.Context, jobID string) ([]sqlc.PresignedUrl, error) {
	if jobID == "" {
		return nil, ErrEmptyJobID
	}

	// Get the job by jobID to get the internal ID
	job, err := r.repo.Q.GetJobByJobID(ctx, jobID)
	if err != nil {
		return nil, &ServiceError{
			Err:     err,
			Code:    500,
			Message: "failed to get job",
		}
	}

	urls, err := r.repo.Q.GetPresignedURLsByJobID(ctx, job.JobID)
	if err != nil {
		return nil, &ServiceError{
			Err:     err,
			Code:    500,
			Message: fmt.Sprintf("failed to get presigned urls for job ID: %s", jobID),
		}
	}
	return urls, nil
}

func (r *RepoService) DeleteJob(ctx context.Context, id int32) error {
	if id == 0 {
		return ErrInvalidJobID
	}

	if err := r.repo.Q.DeleteJob(ctx, id); err != nil {
		return &ServiceError{
			Err:     err,
			Code:    500,
			Message: "failed to delete job",
		}
	}
	return nil
}
