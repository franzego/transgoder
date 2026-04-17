package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/franzego/transgoder/internal/models"
	"github.com/franzego/transgoder/internal/sqlc"
	"github.com/gin-gonic/gin"
	"github.com/minio/minio-go/v7"
)

type repoMock struct {
	createJobFn          func(ctx context.Context, arg sqlc.CreateJobParams) (sqlc.Job, error)
	createPresignedURLFn func(ctx context.Context, jobID, presignedURL string, partNumber int32) (sqlc.PresignedUrl, error)
	deleteJobFn          func(ctx context.Context, id int32) error
	getJobByJobIDFn      func(ctx context.Context, jobID string) (sqlc.Job, error)
	createVideoMetaFn    func(ctx context.Context, arg sqlc.CreateVideoMetaParams) (sqlc.Videometum, error)
	transitionToFn       func(ctx context.Context, jobID string, from, to models.Status) error
	createJobCalls       int
	createPresignedCalls int
	deleteJobCalls       int
	transitionToCalls    int
}

func (m *repoMock) CreateJob(ctx context.Context, arg sqlc.CreateJobParams) (sqlc.Job, error) {
	m.createJobCalls++
	if m.createJobFn != nil {
		return m.createJobFn(ctx, arg)
	}
	return sqlc.Job{}, nil
}

func (m *repoMock) CreatePresignedURL(ctx context.Context, jobID, presignedURL string, partNumber int32) (sqlc.PresignedUrl, error) {
	m.createPresignedCalls++
	if m.createPresignedURLFn != nil {
		return m.createPresignedURLFn(ctx, jobID, presignedURL, partNumber)
	}
	return sqlc.PresignedUrl{}, nil
}

func (m *repoMock) DeleteJob(ctx context.Context, id int32) error {
	m.deleteJobCalls++
	if m.deleteJobFn != nil {
		return m.deleteJobFn(ctx, id)
	}
	return nil
}

func (m *repoMock) GetJobByJobID(ctx context.Context, jobID string) (sqlc.Job, error) {
	if m.getJobByJobIDFn != nil {
		return m.getJobByJobIDFn(ctx, jobID)
	}
	return sqlc.Job{}, errors.New("not implemented in this test")
}

func (m *repoMock) CreateVideoMeta(ctx context.Context, arg sqlc.CreateVideoMetaParams) (sqlc.Videometum, error) {
	if m.createVideoMetaFn != nil {
		return m.createVideoMetaFn(ctx, arg)
	}
	return sqlc.Videometum{}, nil
}

func (m *repoMock) TransitionTo(ctx context.Context, jobID string, from, to models.Status) error {
	m.transitionToCalls++
	if m.transitionToFn != nil {
		return m.transitionToFn(ctx, jobID, from, to)
	}
	return nil
}

type minioMock struct {
	uploadBucket     string
	newUploadFn      func(ctx context.Context, bucketName, objectName string) (string, error)
	presignPartURLFn func(ctx context.Context, bucketName, objectName, uploadID string, partNumber int, expires time.Duration) (string, error)
	completeUploadFn func(ctx context.Context, bucketName, objectName, uploadID string, parts []minio.CompletePart) error
	abortUploadFn    func(ctx context.Context, bucketName, objectName, uploadID string) error
	newUploadCalls   int
	presignPartCalls int
	abortUploadCalls int
	completeCalls    int
}

func (m *minioMock) UploadBucket() string {
	return m.uploadBucket
}

func (m *minioMock) NewMultipartUpload(ctx context.Context, bucketName, objectName string) (string, error) {
	m.newUploadCalls++
	if m.newUploadFn != nil {
		return m.newUploadFn(ctx, bucketName, objectName)
	}
	return "upload-id", nil
}

func (m *minioMock) PresignedUploadPartURL(ctx context.Context, bucketName, objectName, uploadID string, partNumber int, expires time.Duration) (string, error) {
	m.presignPartCalls++
	if m.presignPartURLFn != nil {
		return m.presignPartURLFn(ctx, bucketName, objectName, uploadID, partNumber, expires)
	}
	return "https://example.test/upload", nil
}

func (m *minioMock) CompleteMultipartUpload(ctx context.Context, bucketName, objectName, uploadID string, parts []minio.CompletePart) error {
	m.completeCalls++
	if m.completeUploadFn != nil {
		return m.completeUploadFn(ctx, bucketName, objectName, uploadID, parts)
	}
	return nil
}

func (m *minioMock) AbortMultipartUpload(ctx context.Context, bucketName, objectName, uploadID string) error {
	m.abortUploadCalls++
	if m.abortUploadFn != nil {
		return m.abortUploadFn(ctx, bucketName, objectName, uploadID)
	}
	return nil
}

type redisMock struct {
	enqueueFn    func(ctx context.Context, jobID string) error
	enqueueCalls int
}

func (m *redisMock) Enqueue(ctx context.Context, jobID string) error {
	m.enqueueCalls++
	if m.enqueueFn != nil {
		return m.enqueueFn(ctx, jobID)
	}
	return nil
}
func (m *redisMock) Dequeue(context.Context, string) (string, string, error) {
	return "", "", nil
}

func newTestHandler(repo ServiceRepository, minio MultipartService, redis Queuer) *Handler {
	gin.SetMode(gin.TestMode)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewHandler(minio, repo, redis, logger)
}

func performInitiate(t *testing.T, h *Handler, payload map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("failed to marshal payload: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/upload/initiate", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	h.InitiateMultipartUploadHandler(c)
	return w
}

func performComplete(t *testing.T, h *Handler, payload map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("failed to marshal payload: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/upload/complete", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	h.CompleteMultipartUploadHandler(c)
	return w
}

func TestInitiateMultipartUploadHandler_NewMultipartUploadFailureCleansUpJob(t *testing.T) {
	repo := &repoMock{
		createJobFn: func(_ context.Context, arg sqlc.CreateJobParams) (sqlc.Job, error) {
			return sqlc.Job{ID: 77, JobID: arg.JobID}, nil
		},
	}
	minioSvc := &minioMock{
		uploadBucket: "uploads",
		newUploadFn: func(context.Context, string, string) (string, error) {
			return "", errors.New("minio unavailable")
		},
	}
	h := newTestHandler(repo, minioSvc, &redisMock{})
	w := performInitiate(t, h, map[string]any{
		"file_name": "video.mp4",
		"file_size": 6 * 1024 * 1024,
	})

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d, body: %s", w.Code, w.Body.String())
	}
	if repo.deleteJobCalls != 1 {
		t.Fatalf("expected DeleteJob called once, got %d", repo.deleteJobCalls)
	}
	if minioSvc.abortUploadCalls != 0 {
		t.Fatalf("expected AbortMultipartUpload not called, got %d", minioSvc.abortUploadCalls)
	}
}

func TestInitiateMultipartUploadHandler_PresignFailureAbortsAndDeletes(t *testing.T) {
	repo := &repoMock{
		createJobFn: func(_ context.Context, arg sqlc.CreateJobParams) (sqlc.Job, error) {
			return sqlc.Job{ID: 42, JobID: arg.JobID}, nil
		},
	}
	minioSvc := &minioMock{
		uploadBucket: "uploads",
		newUploadFn: func(context.Context, string, string) (string, error) {
			return "upload-1", nil
		},
		presignPartURLFn: func(_ context.Context, _ string, _ string, _ string, partNumber int, _ time.Duration) (string, error) {
			if partNumber == 2 {
				return "", errors.New("presign failed")
			}
			return "https://example.test/upload/part", nil
		},
	}
	h := newTestHandler(repo, minioSvc, &redisMock{})
	w := performInitiate(t, h, map[string]any{
		"file_name": "video.mp4",
		"file_size": 11 * 1024 * 1024,
		"part_size": 5 * 1024 * 1024,
	})

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d, body: %s", w.Code, w.Body.String())
	}
	if minioSvc.abortUploadCalls != 1 {
		t.Fatalf("expected AbortMultipartUpload called once, got %d", minioSvc.abortUploadCalls)
	}
	if repo.deleteJobCalls != 1 {
		t.Fatalf("expected DeleteJob called once, got %d", repo.deleteJobCalls)
	}
	if repo.createPresignedCalls != 1 {
		t.Fatalf("expected first part only persisted, got %d calls", repo.createPresignedCalls)
	}
}

func TestInitiateMultipartUploadHandler_CreatePresignedURLFailureAbortsAndDeletes(t *testing.T) {
	repo := &repoMock{
		createJobFn: func(_ context.Context, arg sqlc.CreateJobParams) (sqlc.Job, error) {
			return sqlc.Job{ID: 10, JobID: arg.JobID}, nil
		},
		createPresignedURLFn: func(context.Context, string, string, int32) (sqlc.PresignedUrl, error) {
			return sqlc.PresignedUrl{}, errors.New("db insert failed")
		},
	}
	minioSvc := &minioMock{
		uploadBucket: "uploads",
		newUploadFn: func(context.Context, string, string) (string, error) {
			return "upload-2", nil
		},
	}
	h := newTestHandler(repo, minioSvc, &redisMock{})
	w := performInitiate(t, h, map[string]any{
		"file_name": "video.mp4",
		"file_size": 6 * 1024 * 1024,
		"part_size": 5 * 1024 * 1024,
	})

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d, body: %s", w.Code, w.Body.String())
	}
	if minioSvc.abortUploadCalls != 1 {
		t.Fatalf("expected AbortMultipartUpload called once, got %d", minioSvc.abortUploadCalls)
	}
	if repo.deleteJobCalls != 1 {
		t.Fatalf("expected DeleteJob called once, got %d", repo.deleteJobCalls)
	}
}

func TestInitiateMultipartUploadHandler_RejectsTooManyParts(t *testing.T) {
	repo := &repoMock{}
	minioSvc := &minioMock{uploadBucket: "uploads"}
	h := newTestHandler(repo, minioSvc, &redisMock{})

	tooMany := int64(10001 * 5 * 1024 * 1024)
	w := performInitiate(t, h, map[string]any{
		"file_name": "video.mp4",
		"file_size": tooMany,
		"part_size": 5 * 1024 * 1024,
	})

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d, body: %s", w.Code, w.Body.String())
	}
	if repo.createJobCalls != 0 {
		t.Fatalf("expected CreateJob not called for invalid size, got %d", repo.createJobCalls)
	}
	if minioSvc.newUploadCalls != 0 {
		t.Fatalf("expected NewMultipartUpload not called for invalid size, got %d", minioSvc.newUploadCalls)
	}
}

func TestCompleteMultipartUploadHandler_TransitionPendingToQueued(t *testing.T) {
	repo := &repoMock{
		getJobByJobIDFn: func(_ context.Context, jobID string) (sqlc.Job, error) {
			return sqlc.Job{JobID: "JB-123"}, nil
		},
		createVideoMetaFn: func(_ context.Context, arg sqlc.CreateVideoMetaParams) (sqlc.Videometum, error) {
			if arg.JobID != "JB-123" {
				t.Fatalf("expected video meta for JB-123, got %s", arg.JobID)
			}
			if !arg.Format.Valid || arg.Format.String != "mp4" {
				t.Fatalf("expected required format mp4, got %+v", arg.Format)
			}
			if arg.Codec != "h264" {
				t.Fatalf("expected default codec h264, got %+v", arg.Codec)
			}
			return sqlc.Videometum{}, nil
		},
		transitionToFn: func(_ context.Context, jobID string, from, to models.Status) error {
			if jobID != "JB-123" {
				t.Fatalf("expected transition for JB-123, got %s", jobID)
			}
			if from != models.StatusPending || to != models.StatusQueued {
				t.Fatalf("unexpected transition: %s -> %s", from, to)
			}
			return nil
		},
	}
	minioSvc := &minioMock{
		uploadBucket: "uploads",
	}
	redisSvc := &redisMock{}
	h := newTestHandler(repo, minioSvc, redisSvc)

	w := performComplete(t, h, map[string]any{
		"job_id":    "JB-123",
		"upload_id": "upload-1",
		"parts": []map[string]any{
			{"part_number": 1, "etag": "etag-1"},
		},
		"video_name":  "my_video.mp4",
		"description": "test",
		"format":      "mp4",
	})

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d, body: %s", w.Code, w.Body.String())
	}
	if repo.transitionToCalls != 1 {
		t.Fatalf("expected TransitionTo called once, got %d", repo.transitionToCalls)
	}
	if redisSvc.enqueueCalls != 1 {
		t.Fatalf("expected Enqueue called once, got %d", redisSvc.enqueueCalls)
	}
	if !bytes.Contains(w.Body.Bytes(), []byte(`"filename":"my_video.mp4"`)) {
		t.Fatalf("expected filename in response body, got %s", w.Body.String())
	}
}

func TestCompleteMultipartUploadHandler_TransitionFailureReturns500(t *testing.T) {
	repo := &repoMock{
		getJobByJobIDFn: func(_ context.Context, jobID string) (sqlc.Job, error) {
			return sqlc.Job{JobID: "JB-321"}, nil
		},
		createVideoMetaFn: func(_ context.Context, _ sqlc.CreateVideoMetaParams) (sqlc.Videometum, error) {
			return sqlc.Videometum{}, nil
		},
		transitionToFn: func(_ context.Context, _ string, _ models.Status, _ models.Status) error {
			return errors.New("invalid status transition")
		},
	}
	minioSvc := &minioMock{
		uploadBucket: "uploads",
	}
	redisSvc := &redisMock{}
	h := newTestHandler(repo, minioSvc, redisSvc)

	w := performComplete(t, h, map[string]any{
		"job_id":    "JB-321",
		"upload_id": "upload-1",
		"parts": []map[string]any{
			{"part_number": 1, "etag": "etag-1"},
		},
		"video_name": "my_video.mp4",
		"format":     "mp4",
	})

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d, body: %s", w.Code, w.Body.String())
	}
	if !bytes.Contains(w.Body.Bytes(), []byte(`"Failed to update job status"`)) {
		t.Fatalf("expected transition failure response, got %s", w.Body.String())
	}
}
