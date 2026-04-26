package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"

	"github.com/franzego/transcoder/grpc/config"
	pb "github.com/franzego/transcoder/grpc/server"
	"github.com/franzego/transcoder/grpc/webserver"
	"github.com/minio/minio-go/v7"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func makeFakeFFmpeg(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ffmpeg")
	content := `#!/usr/bin/env bash
if [ "$FFMPEG_SHOULD_FAIL" = "1" ]; then
  echo "ffmpeg failed" >&2
  exit 2
fi
exit 0
`
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatalf("failed to write fake ffmpeg: %v", err)
	}
	return path
}

func makeFakeFFprobe(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ffprobe")
	content := `#!/usr/bin/env bash
if [ "$FFPROBE_SHOULD_FAIL" = "1" ]; then
  echo "invalid video" >&2
  exit 3
fi
echo "video"
exit 0
`
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatalf("failed to write fake ffprobe: %v", err)
	}
	return path
}

func makeWebServer(t *testing.T) (*httptest.Server, *[]string) {
	t.Helper()
	var mu sync.Mutex
	transitions := make([]string, 0, 4)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/jobs/job-1/source-url":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"success":true,"message":"ok","code":200,"metadata":{"job_id":"job-1","source_url":"https://example.test/source.mp4"}}`))
			return
		case r.Method == http.MethodGet && r.URL.Path == "/jobs/job-fail/source-url":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"success":true,"message":"ok","code":200,"metadata":{"job_id":"job-fail","source_url":"https://example.test/source.mp4"}}`))
			return
		case r.Method != http.MethodPost:
			t.Fatalf("unexpected method: %s path: %s", r.Method, r.URL.Path)
		}

		var req webserver.JobStatusReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("failed to decode body: %v", err)
		}
		mu.Lock()
		transitions = append(transitions, string(req.From)+"->"+string(req.To))
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"message":"ok"}`))
	}))
	return srv, &transitions
}

type fakeUploader struct {
	err       error
	calls     int
	bucket    string
	objectKey string
}

func (f *fakeUploader) FPutObject(_ context.Context, bucketName, objectName, _ string, _ minio.PutObjectOptions) (minio.UploadInfo, error) {
	f.calls++
	f.bucket = bucketName
	f.objectKey = objectName
	if f.err != nil {
		return minio.UploadInfo{}, f.err
	}
	return minio.UploadInfo{Bucket: bucketName, Key: objectName}, nil
}

func TestNewTranscoderService(t *testing.T) {
	if got := NewTranscoderService(nil, nil, nil, "", "", ""); got == nil {
		t.Fatal("expected non-nil service even with nil deps")
	}
	if got := NewTranscoderService(testLogger(), nil, nil, "", "", ""); got.ffmpegPath != "ffmpeg" {
		t.Fatalf("expected default ffmpeg path, got %q", got.ffmpegPath)
	}
}

func TestTranscodeVideo_Validation(t *testing.T) {
	svc := NewTranscoderService(testLogger(), nil, nil, "", makeFakeFFmpeg(t), makeFakeFFprobe(t))

	tests := []struct {
		name string
		req  *pb.TranscodeRequest
		code codes.Code
	}{
		{name: "nil request", req: nil, code: codes.InvalidArgument},
		{name: "missing job id", req: &pb.TranscodeRequest{}, code: codes.InvalidArgument},
		{
			name: "missing webserver client",
			req:  &pb.TranscodeRequest{JobId: "job-1"},
			code: codes.FailedPrecondition,
		},
		{
			name: "missing minio uploader",
			req:  &pb.TranscodeRequest{JobId: "job-1"},
			code: codes.FailedPrecondition,
		},
	}

	for _, tt := range tests {
		testSvc := svc
		if tt.name == "missing minio uploader" {
			cfg := &config.Config{WebServer: config.WebServerConfig{ServerUrl: "http://example.test"}}
			testSvc = NewTranscoderService(testLogger(), webserver.NewWebserverClient(cfg), nil, "", makeFakeFFmpeg(t), makeFakeFFprobe(t))
		}
		_, err := testSvc.TranscodeVideo(context.Background(), tt.req)
		if status.Code(err) != tt.code {
			t.Fatalf("%s: expected %s, got %s (%v)", tt.name, tt.code, status.Code(err), err)
		}
	}
}

func TestTranscodeVideo_Success(t *testing.T) {
	webSrv, transitions := makeWebServer(t)
	defer webSrv.Close()

	cfg := &config.Config{WebServer: config.WebServerConfig{ServerUrl: webSrv.URL}}
	uploader := &fakeUploader{}
	svc := NewTranscoderService(testLogger(), webserver.NewWebserverClient(cfg), nil, "downloads", makeFakeFFmpeg(t), makeFakeFFprobe(t))
	svc.uploader = uploader

	req := &pb.TranscodeRequest{
		JobId:        "job-1",
		OutputPath:   "output.mp4",
		OutputFormat: "mp4",
		Options: &pb.VideoOptions{
			Codec:      "h264",
			Bitrate:    1200,
			Framerate:  30,
			Resolution: "1280x720",
		},
	}

	resp, err := svc.TranscodeVideo(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Success || resp.Status != "completed" || resp.JobId != "job-1" {
		t.Fatalf("unexpected response: %+v", resp)
	}
	if uploader.calls != 1 || uploader.bucket != "downloads" || uploader.objectKey != "output.mp4" {
		t.Fatalf("unexpected upload call: calls=%d bucket=%s object=%s", uploader.calls, uploader.bucket, uploader.objectKey)
	}
	want := []string{
		"queued->downloading",
		"downloading->processing",
		"processing->uploading",
		"uploading->completed",
	}
	if !slices.Equal(*transitions, want) {
		t.Fatalf("unexpected transitions: got=%v want=%v", *transitions, want)
	}
}

func TestTranscodeVideo_FFmpegFailureMarksJobFailed(t *testing.T) {
	t.Setenv("FFMPEG_SHOULD_FAIL", "1")
	webSrv, transitions := makeWebServer(t)
	defer webSrv.Close()

	cfg := &config.Config{WebServer: config.WebServerConfig{ServerUrl: webSrv.URL}}
	uploader := &fakeUploader{}
	svc := NewTranscoderService(testLogger(), webserver.NewWebserverClient(cfg), nil, "downloads", makeFakeFFmpeg(t), makeFakeFFprobe(t))
	svc.uploader = uploader

	_, err := svc.TranscodeVideo(context.Background(), &pb.TranscodeRequest{JobId: "job-fail"})
	if status.Code(err) != codes.Internal {
		t.Fatalf("expected Internal, got %s (%v)", status.Code(err), err)
	}
	want := []string{
		"queued->downloading",
		"downloading->processing",
		"processing->failed",
	}
	if !slices.Equal(*transitions, want) {
		t.Fatalf("unexpected transitions: got=%v want=%v", *transitions, want)
	}
}

func TestTranscodeVideo_DefaultOutputPath(t *testing.T) {
	webSrv, _ := makeWebServer(t)
	defer webSrv.Close()

	cfg := &config.Config{WebServer: config.WebServerConfig{ServerUrl: webSrv.URL}}
	uploader := &fakeUploader{}
	svc := NewTranscoderService(testLogger(), webserver.NewWebserverClient(cfg), nil, "downloads", makeFakeFFmpeg(t), makeFakeFFprobe(t))
	svc.uploader = uploader

	resp, err := svc.TranscodeVideo(context.Background(), &pb.TranscodeRequest{JobId: "job-1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.OutputPath != "job-1.mp4" {
		t.Fatalf("expected default object key, got %s", resp.OutputPath)
	}
}

func TestTranscodeVideo_UploadFailureMarksJobFailed(t *testing.T) {
	webSrv, transitions := makeWebServer(t)
	defer webSrv.Close()

	cfg := &config.Config{WebServer: config.WebServerConfig{ServerUrl: webSrv.URL}}
	uploader := &fakeUploader{err: errors.New("upload failed")}
	svc := NewTranscoderService(testLogger(), webserver.NewWebserverClient(cfg), nil, "downloads", makeFakeFFmpeg(t), makeFakeFFprobe(t))
	svc.uploader = uploader

	_, err := svc.TranscodeVideo(context.Background(), &pb.TranscodeRequest{JobId: "job-1"})
	if status.Code(err) != codes.Internal {
		t.Fatalf("expected Internal, got %s (%v)", status.Code(err), err)
	}
	want := []string{
		"queued->downloading",
		"downloading->processing",
		"processing->uploading",
		"uploading->failed",
	}
	if !slices.Equal(*transitions, want) {
		t.Fatalf("unexpected transitions: got=%v want=%v", *transitions, want)
	}
}

func TestBuildFFmpegArgs(t *testing.T) {
	req := &pb.TranscodeRequest{
		OutputFormat: "mp4",
		Options: &pb.VideoOptions{
			Codec:      "h265",
			Bitrate:    900,
			Framerate:  24,
			Resolution: "720",
		},
	}
	args := buildFFmpegArgs("in.mp4", "out.mp4", req)
	got := []string{"-y", "-i", "in.mp4", "-c:v", "libx265", "-b:v", "900k", "-r", "24", "-vf", "scale=1280x720:flags=lanczos", "-f", "mp4", "out.mp4"}
	if !slices.Equal(args, got) {
		t.Fatalf("unexpected ffmpeg args:\n got=%v\nwant=%v", args, got)
	}
}

func TestTranscodeVideo_FFprobeFailureMarksJobFailed(t *testing.T) {
	t.Setenv("FFPROBE_SHOULD_FAIL", "1")
	webSrv, transitions := makeWebServer(t)
	defer webSrv.Close()

	cfg := &config.Config{WebServer: config.WebServerConfig{ServerUrl: webSrv.URL}}
	uploader := &fakeUploader{}
	svc := NewTranscoderService(testLogger(), webserver.NewWebserverClient(cfg), nil, "downloads", makeFakeFFmpeg(t), makeFakeFFprobe(t))
	svc.uploader = uploader

	_, err := svc.TranscodeVideo(context.Background(), &pb.TranscodeRequest{JobId: "job-1"})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("expected InvalidArgument, got %s (%v)", status.Code(err), err)
	}
	want := []string{
		"queued->downloading",
		"downloading->failed",
	}
	if !slices.Equal(*transitions, want) {
		t.Fatalf("unexpected transitions: got=%v want=%v", *transitions, want)
	}
}
