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

	"github.com/franzego/transgoder/internal/sqlc"
	"github.com/gin-gonic/gin"
	"github.com/minio/minio-go/v7"
)

type repoMock struct {
	createJobFn          func(ctx context.Context, arg sqlc.CreateJobParams) (sqlc.Job, error)
	createPresignedURLFn func(ctx context.Context, jobID, presignedURL string, partNumber int32) (sqlc.PresignedUrl, error)
	deleteJobFn          func(ctx context.Context, id int32) error
	createJobCalls       int
	createPresignedCalls int
	deleteJobCalls       int
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

func (m *repoMock) GetJobByJobID(context.Context, string) (sqlc.Job, error) {
	return sqlc.Job{}, errors.New("not implemented in this test")
}

func (m *repoMock) UpdateJobStatus(context.Context, sqlc.UpdateJobStatusParams) (sqlc.Job, error) {
	return sqlc.Job{}, errors.New("not implemented in this test")
}

func (m *repoMock) CreateVideoMeta(context.Context, sqlc.CreateVideoMetaParams) (sqlc.Videometum, error) {
	return sqlc.Videometum{}, errors.New("not implemented in this test")
}

type minioMock struct {
	uploadBucket     string
	newUploadFn      func(ctx context.Context, bucketName, objectName string) (string, error)
	presignPartURLFn func(ctx context.Context, bucketName, objectName, uploadID string, partNumber int, expires time.Duration) (string, error)
	abortUploadFn    func(ctx context.Context, bucketName, objectName, uploadID string) error
	newUploadCalls   int
	presignPartCalls int
	abortUploadCalls int
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

func (m *minioMock) CompleteMultipartUpload(context.Context, string, string, string, []minio.CompletePart) error {
	return errors.New("not implemented in this test")
}

func (m *minioMock) AbortMultipartUpload(ctx context.Context, bucketName, objectName, uploadID string) error {
	m.abortUploadCalls++
	if m.abortUploadFn != nil {
		return m.abortUploadFn(ctx, bucketName, objectName, uploadID)
	}
	return nil
}

type redisMock struct{}

func (m *redisMock) Enqueue(context.Context, string) error { return nil }
func (m *redisMock) Dequeue(context.Context, string) (string, string, error) {
	return "", "", nil
}

func newTestHandler(repo ServiceRepository, minio MultipartService) *Handler {
	gin.SetMode(gin.TestMode)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewHandler(minio, repo, &redisMock{}, logger)
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
	h := newTestHandler(repo, minioSvc)
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
	h := newTestHandler(repo, minioSvc)
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
	h := newTestHandler(repo, minioSvc)
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
	h := newTestHandler(repo, minioSvc)

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
