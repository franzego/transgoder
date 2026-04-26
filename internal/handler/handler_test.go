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

	transcoderpb "github.com/franzego/transcoder/grpc/server"
	"github.com/franzego/transgoder/internal/models"
	"github.com/franzego/transgoder/internal/sqlc"
	"github.com/franzego/transgoder/pkg"
	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/minio/minio-go/v7"
)

type repoMock struct {
	createJobFn          func(ctx context.Context, jobID string) (sqlc.Job, error)
	createPresignedURLFn func(ctx context.Context, jobID, presignedURL string, partNumber int32) (sqlc.PresignedUrl, error)
	deleteJobFn          func(ctx context.Context, id int32) error
	getJobByJobIDFn      func(ctx context.Context, jobID string) (sqlc.Job, error)
	getVideoMetaByJobID  func(ctx context.Context, jobID string) (sqlc.Videometum, error)
	createVideoMetaFn    func(ctx context.Context, arg models.VideoMedataReq) (sqlc.Videometum, error)
	transitionToFn       func(ctx context.Context, jobID string, from, to models.Status) error
	createJobCalls       int
	createPresignedCalls int
	deleteJobCalls       int
	transitionToCalls    int
}

func (m *repoMock) CreateJob(ctx context.Context, jobID string) (sqlc.Job, error) {
	m.createJobCalls++
	if m.createJobFn != nil {
		return m.createJobFn(ctx, jobID)
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

func (m *repoMock) CreateVideoMeta(ctx context.Context, arg models.VideoMedataReq) (sqlc.Videometum, error) {
	if m.createVideoMetaFn != nil {
		return m.createVideoMetaFn(ctx, arg)
	}
	return sqlc.Videometum{}, nil
}

func (m *repoMock) GetVideoMetaByJobID(ctx context.Context, jobID string) (sqlc.Videometum, error) {
	if m.getVideoMetaByJobID != nil {
		return m.getVideoMetaByJobID(ctx, jobID)
	}
	return sqlc.Videometum{}, errors.New("not implemented in this test")
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
	downloadBucket   string
	getPresignedFn   func(ctx context.Context, bucketName, jobID string) (string, error)
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

func (m *minioMock) DownloadBucket() string {
	return m.downloadBucket
}

func (m *minioMock) GetPresignedURL(ctx context.Context, bucketName, jobID string) (string, error) {
	if m.getPresignedFn != nil {
		return m.getPresignedFn(ctx, bucketName, jobID)
	}
	return "https://example.test/source.mp4", nil
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

type grpcMock struct {
	transcodeFn    func(ctx context.Context, req *transcoderpb.TranscodeRequest) (*transcoderpb.TranscodeResponse, error)
	transcodeCalls int
}

func (m *grpcMock) TranscodeVideo(ctx context.Context, req *transcoderpb.TranscodeRequest) (*transcoderpb.TranscodeResponse, error) {
	m.transcodeCalls++
	if m.transcodeFn != nil {
		return m.transcodeFn(ctx, req)
	}
	return &transcoderpb.TranscodeResponse{Success: true}, nil
}

func newTestHandler(repo ServiceRepository, minio MultipartService, redis Queuer, grpcClient TranscoderClient) *Handler {
	gin.SetMode(gin.TestMode)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	validate := validator.New()
	pkg.RegisterCustomValidations(validate)
	return NewHandler(minio, repo, redis, grpcClient, logger, validate)
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

func performUpdateStatus(t *testing.T, h *Handler, jobID string, payload map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("failed to marshal payload: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/status/"+jobID+"/update", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	c.Params = gin.Params{{Key: "id", Value: jobID}}
	h.UpdateStatus(c)
	return w
}

func performGetJobStatus(t *testing.T, h *Handler, jobID string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/status/"+jobID+"/update", nil)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	c.Params = gin.Params{{Key: "id", Value: jobID}}
	h.GetJobStatus(c)
	return w
}

func performGetSourceURL(t *testing.T, h *Handler, jobID string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/jobs/"+jobID+"/source-url", nil)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	c.Params = gin.Params{{Key: "id", Value: jobID}}
	h.GetSourceVideoURL(c)
	return w
}

func performGetOutputURL(t *testing.T, h *Handler, jobID string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/jobs/"+jobID+"/output-url", nil)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	c.Params = gin.Params{{Key: "id", Value: jobID}}
	h.GetOutputVideoURL(c)
	return w
}

func performDownloadOutput(t *testing.T, h *Handler, jobID string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/jobs/"+jobID+"/download", nil)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	c.Params = gin.Params{{Key: "id", Value: jobID}}
	h.DownloadOutputVideo(c)
	return w
}

func TestInitiateMultipartUploadHandler_NewMultipartUploadFailureCleansUpJob(t *testing.T) {
	repo := &repoMock{
		createJobFn: func(_ context.Context, jobID string) (sqlc.Job, error) {
			return sqlc.Job{ID: 77, JobID: jobID}, nil
		},
	}
	minioSvc := &minioMock{
		uploadBucket: "uploads",
		newUploadFn: func(context.Context, string, string) (string, error) {
			return "", errors.New("minio unavailable")
		},
	}
	h := newTestHandler(repo, minioSvc, &redisMock{}, nil)
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
		createJobFn: func(_ context.Context, jobID string) (sqlc.Job, error) {
			return sqlc.Job{ID: 42, JobID: jobID}, nil
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
	h := newTestHandler(repo, minioSvc, &redisMock{}, nil)
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
		createJobFn: func(_ context.Context, jobID string) (sqlc.Job, error) {
			return sqlc.Job{ID: 10, JobID: jobID}, nil
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
	h := newTestHandler(repo, minioSvc, &redisMock{}, nil)
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
	h := newTestHandler(repo, minioSvc, &redisMock{}, nil)

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
			createVideoMetaFn: func(ctx context.Context, arg models.VideoMedataReq) (sqlc.Videometum, error) {
				if arg.JobID != "JB-123" {
					t.Fatalf("expected video meta for JB-123, got %s", arg.JobID)
				}
				if !arg.Format.Valid || arg.Format.String != "mp4" {
					t.Fatalf("expected required format mp4, got %+v", arg.Format)
				}
				if !arg.Resolution.Valid || arg.Resolution.String != "1080" {
					t.Fatalf("expected default resolution 1080, got %+v", arg.Resolution)
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
	h := newTestHandler(repo, minioSvc, redisSvc, nil)

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
		createVideoMetaFn: func(ctx context.Context, arg models.VideoMedataReq) (sqlc.Videometum, error) {
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
	h := newTestHandler(repo, minioSvc, redisSvc, nil)

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

func TestCompleteMultipartUploadHandler_InvalidResolutionReturns400(t *testing.T) {
	repo := &repoMock{
		getJobByJobIDFn: func(_ context.Context, jobID string) (sqlc.Job, error) {
			return sqlc.Job{JobID: jobID}, nil
		},
	}
	minioSvc := &minioMock{uploadBucket: "uploads"}
	h := newTestHandler(repo, minioSvc, &redisMock{}, nil)

	w := performComplete(t, h, map[string]any{
		"job_id":     "JB-771",
		"upload_id":  "upload-1",
		"resolution": "360",
		"parts": []map[string]any{
			{"part_number": 1, "etag": "etag-1"},
		},
		"video_name": "my_video.mp4",
		"format":     "mp4",
	})

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d, body: %s", w.Code, w.Body.String())
	}
	if !bytes.Contains(w.Body.Bytes(), []byte(`"Invalid resolution"`)) {
		t.Fatalf("expected invalid resolution response, got %s", w.Body.String())
	}
}

func TestUpdateStatus_Success(t *testing.T) {
	repo := &repoMock{
		transitionToFn: func(_ context.Context, jobID string, from, to models.Status) error {
			if jobID != "JB-987" {
				t.Fatalf("expected JB-987, got %s", jobID)
			}
			if from != models.StatusPending || to != models.StatusQueued {
				t.Fatalf("unexpected transition: %s -> %s", from, to)
			}
			return nil
		},
	}
	h := newTestHandler(repo, &minioMock{uploadBucket: "uploads"}, &redisMock{}, nil)
	w := performUpdateStatus(t, h, "JB-987", map[string]any{
		"id":   "JB-987",
		"from": "pending",
		"to":   "queued",
	})

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d, body: %s", w.Code, w.Body.String())
	}
	if repo.transitionToCalls != 1 {
		t.Fatalf("expected TransitionTo called once, got %d", repo.transitionToCalls)
	}
}

func TestUpdateStatus_ValidationFailureReturns400(t *testing.T) {
	repo := &repoMock{}
	h := newTestHandler(repo, &minioMock{uploadBucket: "uploads"}, &redisMock{}, nil)
	w := performUpdateStatus(t, h, "JB-111", map[string]any{
		"from": "pending",
	})

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d, body: %s", w.Code, w.Body.String())
	}
	if repo.transitionToCalls != 0 {
		t.Fatalf("expected TransitionTo not called, got %d", repo.transitionToCalls)
	}
}

func TestUpdateStatus_TransitionFailureReturns500(t *testing.T) {
	repo := &repoMock{
		transitionToFn: func(_ context.Context, _ string, _ models.Status, _ models.Status) error {
			return errors.New("invalid transition")
		},
	}
	h := newTestHandler(repo, &minioMock{uploadBucket: "uploads"}, &redisMock{}, nil)
	w := performUpdateStatus(t, h, "JB-112", map[string]any{
		"id":   "JB-112",
		"from": "pending",
		"to":   "failed",
	})

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d, body: %s", w.Code, w.Body.String())
	}
}

func TestGetJobStatus_Success(t *testing.T) {
	repo := &repoMock{
		getJobByJobIDFn: func(_ context.Context, jobID string) (sqlc.Job, error) {
			if jobID != "JB-700" {
				t.Fatalf("expected JB-700, got %s", jobID)
			}
			return sqlc.Job{JobID: jobID, Status: "processing"}, nil
		},
	}
	h := newTestHandler(repo, &minioMock{uploadBucket: "uploads"}, &redisMock{}, nil)
	w := performGetJobStatus(t, h, "JB-700")

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d, body: %s", w.Code, w.Body.String())
	}
	if !bytes.Contains(w.Body.Bytes(), []byte(`"status":"processing"`)) {
		t.Fatalf("expected processing status in response, got %s", w.Body.String())
	}
}

func TestGetJobStatus_FailureReturns500(t *testing.T) {
	repo := &repoMock{
		getJobByJobIDFn: func(_ context.Context, _ string) (sqlc.Job, error) {
			return sqlc.Job{}, errors.New("db unavailable")
		},
	}
	h := newTestHandler(repo, &minioMock{uploadBucket: "uploads"}, &redisMock{}, nil)
	w := performGetJobStatus(t, h, "JB-701")

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d, body: %s", w.Code, w.Body.String())
	}
}

func TestGetSourceVideoURL_Success(t *testing.T) {
	repo := &repoMock{
		getJobByJobIDFn: func(_ context.Context, jobID string) (sqlc.Job, error) {
			return sqlc.Job{JobID: jobID}, nil
		},
	}
	minioSvc := &minioMock{
		uploadBucket: "uploads",
		downloadBucket: "downloads",
		getPresignedFn: func(_ context.Context, bucketName, jobID string) (string, error) {
			if bucketName != "uploads" || jobID != "JB-999" {
				t.Fatalf("unexpected args bucket=%s job=%s", bucketName, jobID)
			}
			return "https://example.test/source.mp4", nil
		},
	}
	h := newTestHandler(repo, minioSvc, &redisMock{}, nil)
	w := performGetSourceURL(t, h, "JB-999")
	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d, body: %s", w.Code, w.Body.String())
	}
	if !bytes.Contains(w.Body.Bytes(), []byte(`"source_url":"https://example.test/source.mp4"`)) {
		t.Fatalf("expected source_url in response body, got %s", w.Body.String())
	}
}

func TestGetSourceVideoURL_FailureReturns500(t *testing.T) {
	t.Run("job lookup fails", func(t *testing.T) {
		repo := &repoMock{
			getJobByJobIDFn: func(_ context.Context, _ string) (sqlc.Job, error) {
				return sqlc.Job{}, errors.New("job missing")
			},
		}
		h := newTestHandler(repo, &minioMock{uploadBucket: "uploads"}, &redisMock{}, nil)
		w := performGetSourceURL(t, h, "JB-998")
		if w.Code != http.StatusInternalServerError {
			t.Fatalf("expected status 500, got %d, body: %s", w.Code, w.Body.String())
		}
	})

	t.Run("presign fails", func(t *testing.T) {
		repo := &repoMock{
			getJobByJobIDFn: func(_ context.Context, jobID string) (sqlc.Job, error) {
				return sqlc.Job{JobID: jobID}, nil
			},
		}
		minioSvc := &minioMock{
			uploadBucket: "uploads",
			downloadBucket: "downloads",
			getPresignedFn: func(_ context.Context, _, _ string) (string, error) {
				return "", errors.New("minio down")
			},
		}
		h := newTestHandler(repo, minioSvc, &redisMock{}, nil)
		w := performGetSourceURL(t, h, "JB-997")
		if w.Code != http.StatusInternalServerError {
			t.Fatalf("expected status 500, got %d, body: %s", w.Code, w.Body.String())
		}
	})
}

func TestGetOutputVideoURL_Success(t *testing.T) {
	repo := &repoMock{
		getJobByJobIDFn: func(_ context.Context, jobID string) (sqlc.Job, error) {
			return sqlc.Job{JobID: jobID, Status: "completed"}, nil
		},
		getVideoMetaByJobID: func(_ context.Context, jobID string) (sqlc.Videometum, error) {
			return sqlc.Videometum{JobID: jobID, Format: pgtype.Text{String: "mov", Valid: true}}, nil
		},
	}
	minioSvc := &minioMock{
		uploadBucket:   "uploads",
		downloadBucket: "downloads",
		getPresignedFn: func(_ context.Context, bucketName, objectKey string) (string, error) {
			if bucketName != "downloads" || objectKey != "JB-801.mov" {
				t.Fatalf("unexpected args bucket=%s object=%s", bucketName, objectKey)
			}
			return "https://example.test/download.mov", nil
		},
	}
	h := newTestHandler(repo, minioSvc, &redisMock{}, nil)
	w := performGetOutputURL(t, h, "JB-801")
	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d, body: %s", w.Code, w.Body.String())
	}
	if !bytes.Contains(w.Body.Bytes(), []byte(`"output_url":"https://example.test/download.mov"`)) {
		t.Fatalf("expected output_url in response body, got %s", w.Body.String())
	}
}

func TestGetOutputVideoURL_NotReadyReturns409(t *testing.T) {
	repo := &repoMock{
		getJobByJobIDFn: func(_ context.Context, jobID string) (sqlc.Job, error) {
			return sqlc.Job{JobID: jobID, Status: "processing"}, nil
		},
	}
	h := newTestHandler(repo, &minioMock{uploadBucket: "uploads", downloadBucket: "downloads"}, &redisMock{}, nil)
	w := performGetOutputURL(t, h, "JB-802")
	if w.Code != http.StatusConflict {
		t.Fatalf("expected status 409, got %d, body: %s", w.Code, w.Body.String())
	}
}

func TestGetOutputVideoURL_Failures(t *testing.T) {
	t.Run("job lookup fails", func(t *testing.T) {
		repo := &repoMock{
			getJobByJobIDFn: func(_ context.Context, _ string) (sqlc.Job, error) {
				return sqlc.Job{}, errors.New("db down")
			},
		}
		h := newTestHandler(repo, &minioMock{uploadBucket: "uploads", downloadBucket: "downloads"}, &redisMock{}, nil)
		w := performGetOutputURL(t, h, "JB-803")
		if w.Code != http.StatusInternalServerError {
			t.Fatalf("expected status 500, got %d, body: %s", w.Code, w.Body.String())
		}
	})

	t.Run("presign fails", func(t *testing.T) {
		repo := &repoMock{
			getJobByJobIDFn: func(_ context.Context, jobID string) (sqlc.Job, error) {
				return sqlc.Job{JobID: jobID, Status: "completed"}, nil
			},
		}
		minioSvc := &minioMock{
			uploadBucket:   "uploads",
			downloadBucket: "downloads",
			getPresignedFn: func(_ context.Context, _, _ string) (string, error) {
				return "", errors.New("minio down")
			},
		}
		h := newTestHandler(repo, minioSvc, &redisMock{}, nil)
		w := performGetOutputURL(t, h, "JB-804")
		if w.Code != http.StatusInternalServerError {
			t.Fatalf("expected status 500, got %d, body: %s", w.Code, w.Body.String())
		}
	})
}

func TestDownloadOutputVideo_CompletedStreamsVideo(t *testing.T) {
	videoSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "video/mp4")
		_, _ = w.Write([]byte("video-bytes"))
	}))
	defer videoSrv.Close()

	repo := &repoMock{
		getJobByJobIDFn: func(_ context.Context, jobID string) (sqlc.Job, error) {
			return sqlc.Job{JobID: jobID, Status: "completed"}, nil
		},
	}
	minioSvc := &minioMock{
		uploadBucket:   "uploads",
		downloadBucket: "downloads",
		getPresignedFn: func(_ context.Context, _, _ string) (string, error) {
			return videoSrv.URL, nil
		},
	}
	h := newTestHandler(repo, minioSvc, &redisMock{}, nil)
	w := performDownloadOutput(t, h, "JB-901")
	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d, body: %s", w.Code, w.Body.String())
	}
	if w.Body.String() != "video-bytes" {
		t.Fatalf("expected streamed body, got %q", w.Body.String())
	}
}

func TestDownloadOutputVideo_TriggersTranscodeWhenNotCompleted(t *testing.T) {
	videoSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "video/mp4")
		_, _ = w.Write([]byte("video-bytes"))
	}))
	defer videoSrv.Close()

	calls := 0
	repo := &repoMock{
		getJobByJobIDFn: func(_ context.Context, jobID string) (sqlc.Job, error) {
			calls++
			if calls == 1 {
				return sqlc.Job{JobID: jobID, Status: "queued"}, nil
			}
			return sqlc.Job{JobID: jobID, Status: "completed"}, nil
		},
		getVideoMetaByJobID: func(_ context.Context, jobID string) (sqlc.Videometum, error) {
			return sqlc.Videometum{
				JobID:      jobID,
				Codec:      "h264",
				Format:     pgtype.Text{String: "mp4", Valid: true},
				Resolution: pgtype.Text{String: "1280x720", Valid: true},
				Bitrate:    pgtype.Int4{Int32: 900, Valid: true},
				Framerate:  pgtype.Int4{Int32: 30, Valid: true},
			}, nil
		},
	}
	minioSvc := &minioMock{
		uploadBucket:   "uploads",
		downloadBucket: "downloads",
		getPresignedFn: func(_ context.Context, _, _ string) (string, error) {
			return videoSrv.URL, nil
		},
	}
	grpcSvc := &grpcMock{
		transcodeFn: func(_ context.Context, req *transcoderpb.TranscodeRequest) (*transcoderpb.TranscodeResponse, error) {
			if req.JobId != "JB-902" {
				t.Fatalf("unexpected job id: %s", req.JobId)
			}
			return &transcoderpb.TranscodeResponse{Success: true, OutputPath: "JB-902.mp4"}, nil
		},
	}
	h := newTestHandler(repo, minioSvc, &redisMock{}, grpcSvc)
	w := performDownloadOutput(t, h, "JB-902")
	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d, body: %s", w.Code, w.Body.String())
	}
	if grpcSvc.transcodeCalls != 1 {
		t.Fatalf("expected transcode called once, got %d", grpcSvc.transcodeCalls)
	}
}
