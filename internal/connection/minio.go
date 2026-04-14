package connection

import (
	"context"

	"github.com/franzego/transgoder/internal/config"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type MinioClient struct {
	*minio.Client
}

func NewMinioConnection(ctx context.Context, c *config.MinioConfig) (*MinioClient, error) {
	client, err := minio.New(c.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(c.AccessKey, c.SecretKey, ""),
		Secure: c.UseSSL,
	})
	if err != nil {
		return nil, err
	}

	if err := ensureBucket(ctx, client, c.UploadBucket); err != nil {
		return nil, err
	}
	if err := ensureBucket(ctx, client, c.DownloadBucket); err != nil {
		return nil, err
	}

	return &MinioClient{
		client,
	}, nil
}

func ensureBucket(ctx context.Context, client *minio.Client, name string) error {
	exists, err := client.BucketExists(ctx, name)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	return client.MakeBucket(ctx, name, minio.MakeBucketOptions{})
}
