package service

import (
	"context"

	"github.com/franzego/transgoder/internal/repository"
	"github.com/franzego/transgoder/internal/sqlc"
)

type ServiceRepository interface {
	CreateJob(ctx context.Context, arg sqlc.CreateJobParams) (sqlc.Job, error)
	GetJobByJobID(ctx context.Context, jobID string) (sqlc.Job, error)
	UpdateJobStatus(ctx context.Context, arg sqlc.UpdateJobStatusParams) (sqlc.Job, error)
	CreateVideoMeta(ctx context.Context, arg sqlc.CreateVideoMetaParams) (sqlc.Videometum, error)
}

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
	if arg.ID == 0 {
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
	if arg.JobID == 0 {
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
