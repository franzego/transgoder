package connection

import (
	"context"
	"log/slog"

	"github.com/franzego/transcoder/internal/config"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type MinioClient struct {
	*minio.Client
}

func NewMinioConnection(ctx context.Context, c *config.MinioConfig, logger *slog.Logger) (*MinioClient, error) {
	logger.Info("Connecting to MinIO", "endpoint", c.Endpoint)

	client, err := minio.New(c.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(c.AccessKey, c.SecretKey, ""),
		Secure: c.UseSSL,
	})
	if err != nil {
		logger.Error("Failed to create MinIO client", "error", err)
		return nil, err
	}

	logger.Info("Ensuring buckets exist", "uploadBucket", c.UploadBucket, "downloadBucket", c.DownloadBucket)

	if err := ensureBucket(ctx, client, c.UploadBucket, logger); err != nil {
		logger.Error("Failed to ensure upload bucket", "bucket", c.UploadBucket, "error", err)
		return nil, err
	}
	if err := ensureBucket(ctx, client, c.DownloadBucket, logger); err != nil {
		logger.Error("Failed to ensure download bucket", "bucket", c.DownloadBucket, "error", err)
		return nil, err
	}

	logger.Info("Successfully connected to MinIO")
	return &MinioClient{
		client,
	}, nil
}

func ensureBucket(ctx context.Context, client *minio.Client, name string, logger *slog.Logger) error {
	logger.Info("Checking bucket", "bucket", name)

	exists, err := client.BucketExists(ctx, name)
	if err != nil {
		logger.Error("Failed to check bucket existence", "bucket", name, "error", err)
		return err
	}

	if exists {
		logger.Info("Bucket already exists", "bucket", name)
		return nil
	}

	logger.Info("Creating bucket", "bucket", name)
	err = client.MakeBucket(ctx, name, minio.MakeBucketOptions{})
	if err != nil {
		logger.Error("Failed to create bucket", "bucket", name, "error", err)
		return err
	}

	logger.Info("Bucket created successfully", "bucket", name)
	return nil
}
