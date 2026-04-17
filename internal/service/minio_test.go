package service

import (
	"context"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/franzego/transgoder/internal/config"
	"github.com/franzego/transgoder/internal/connection"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

func buildMinioService(t *testing.T) *MinioService {
	t.Helper()

	client, err := minio.New("localhost:9000", &minio.Options{
		Creds:  credentials.NewStaticV4("minioadmin", "minioadmin", ""),
		Secure: false,
	})
	if err != nil {
		t.Fatalf("failed to build minio client: %v", err)
	}

	return NewMinioService(
		&config.MinioConfig{
			UploadBucket:   "uploads",
			DownloadBucket: "downloads",
		},
		&connection.MinioClient{Client: client},
	)
}

func TestNewMinioService(t *testing.T) {
	client, _ := minio.New("localhost:9000", &minio.Options{
		Creds:  credentials.NewStaticV4("minioadmin", "minioadmin", ""),
		Secure: false,
		Region: "us-east-1",
	})

	if got := NewMinioService(nil, &connection.MinioClient{Client: client}); got != nil {
		t.Fatalf("expected nil with nil cfg, got %#v", got)
	}
	if got := NewMinioService(&config.MinioConfig{}, nil); got != nil {
		t.Fatalf("expected nil with nil client, got %#v", got)
	}
}

func TestMinioService_PresignedURLs(t *testing.T) {
	svc := buildMinioService(t)
	ctx := context.Background()

	t.Run("PutPresignedURL", func(t *testing.T) {
		raw, err := svc.PutPresignedURL(ctx, "uploads", "job-123")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		u, err := url.Parse(raw)
		if err != nil {
			t.Fatalf("failed to parse URL: %v", err)
		}
		if u.Host != "localhost:9000" {
			t.Fatalf("unexpected host %q", u.Host)
		}
		if !strings.Contains(u.Path, "/uploads/job-123") {
			t.Fatalf("unexpected path %q", u.Path)
		}
		if u.Query().Get("X-Amz-Signature") == "" {
			t.Fatalf("expected signature in query, got %q", raw)
		}
	})

	t.Run("GetPresignedURL", func(t *testing.T) {
		raw, err := svc.GetPresignedURL(ctx, "downloads", "job-123")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		u, err := url.Parse(raw)
		if err != nil {
			t.Fatalf("failed to parse URL: %v", err)
		}
		if !strings.Contains(u.Path, "/downloads/job-123") {
			t.Fatalf("unexpected path %q", u.Path)
		}
		if u.Query().Get("X-Amz-Signature") == "" {
			t.Fatalf("expected signature in query, got %q", raw)
		}
	})

	t.Run("PresignedUploadPartURL", func(t *testing.T) {
		raw, err := svc.PresignedUploadPartURL(ctx, "uploads", "job-123", "upload-abc", 3, time.Hour)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		u, err := url.Parse(raw)
		if err != nil {
			t.Fatalf("failed to parse URL: %v", err)
		}
		if u.Query().Get("partNumber") != "3" {
			t.Fatalf("expected partNumber=3, got %q", u.Query().Get("partNumber"))
		}
		if u.Query().Get("uploadId") != "upload-abc" {
			t.Fatalf("expected uploadId=upload-abc, got %q", u.Query().Get("uploadId"))
		}
		if u.Query().Get("X-Amz-Signature") == "" {
			t.Fatalf("expected signature in query, got %q", raw)
		}
	})
}
