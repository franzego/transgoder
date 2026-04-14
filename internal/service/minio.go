package service

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/franzego/transgoder/internal/config"
	"github.com/franzego/transgoder/internal/connection"
	"github.com/minio/minio-go/v7"
)

type MinioService struct {
	Cfg    *config.MinioConfig
	Client *connection.MinioClient
}

func NewMinioService(cfg *config.MinioConfig, client *connection.MinioClient) *MinioService {
	return &MinioService{
		Cfg:    cfg,
		Client: client,
	}
}

func (m *MinioService) PutPresignedURL(ctx context.Context, bucketName, jobID string) (string, error) {
	url, err := m.Client.PresignedPutObject(ctx, bucketName, jobID, 60*time.Minute)
	if err != nil {
		return "", err
	}
	return url.String(), nil
}

func (m *MinioService) GetPresignedURL(ctx context.Context, bucketName, jobID string) (string, error) {
	url, err := m.Client.PresignedGetObject(ctx, bucketName, jobID, time.Duration(24*time.Hour), nil)
	if err != nil {
		return "", err
	}
	return url.String(), nil
}

func (m *MinioService) NewMultipartUpload(ctx context.Context, bucketName, objectName string) (string, error) {
	core := minio.Core{Client: m.Client.Client}
	return core.NewMultipartUpload(ctx, bucketName, objectName, minio.PutObjectOptions{})
}

func (m *MinioService) PresignedUploadPartURL(ctx context.Context, bucketName, objectName, uploadID string, partNumber int, expires time.Duration) (string, error) {
	params := url.Values{}
	params.Set("partNumber", strconv.Itoa(partNumber))
	params.Set("uploadId", uploadID)
	u, err := m.Client.Presign(ctx, http.MethodPut, bucketName, objectName, expires, params)
	if err != nil {
		return "", err
	}
	return u.String(), nil
}

func (m *MinioService) CompleteMultipartUpload(ctx context.Context, bucketName, objectName, uploadID string, parts []minio.CompletePart) error {
	core := minio.Core{Client: m.Client.Client}
	_, err := core.CompleteMultipartUpload(ctx, bucketName, objectName, uploadID, parts, minio.PutObjectOptions{})
	return err
}

func (m *MinioService) AbortMultipartUpload(ctx context.Context, bucketName, objectName, uploadID string) error {
	core := minio.Core{Client: m.Client.Client}
	return core.AbortMultipartUpload(ctx, bucketName, objectName, uploadID)
}
