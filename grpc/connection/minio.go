package connection

import (
	"context"
	"errors"
	"log/slog"
	"math/rand"
	"net"
	"strings"
	"time"

	"github.com/franzego/transcoder/grpc/config"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type MinioClient struct {
	*minio.Client
}

var (
	minioRetrySleep       = sleepWithContext
	minioRetryRandFloat64 = rand.Float64
)

func NewMinioConnection(ctx context.Context, c *config.MinioConfig, logger *slog.Logger) (*MinioClient, error) {
	maxAttempts := c.ConnectMaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 20
	}
	initialBackoff := c.ConnectInitialBackoff
	if initialBackoff <= 0 {
		initialBackoff = time.Second
	}
	maxBackoff := c.ConnectMaxBackoff
	if maxBackoff <= 0 {
		maxBackoff = 10 * time.Second
	}

	logger.Info("Connecting to MinIO", "endpoint", c.Endpoint, "max_attempts", maxAttempts)

	var client *minio.Client
	err := retryWithBackoff(ctx, maxAttempts, initialBackoff, maxBackoff, logger, "minio_connect", func(attempt int) error {
		client, err := minio.New(c.Endpoint, &minio.Options{
			Creds:  credentials.NewStaticV4(c.AccessKey, c.SecretKey, ""),
			Secure: c.UseSSL,
		})
		if err != nil {
			return err
		}
		logger.Info("Ensuring buckets exist", "uploadBucket", c.UploadBucket, "downloadBucket", c.DownloadBucket, "attempt", attempt)
		if err := ensureBucket(ctx, client, c.UploadBucket, logger); err != nil {
			return err
		}
		if err := ensureBucket(ctx, client, c.DownloadBucket, logger); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	logger.Info("Successfully connected to MinIO")
	return &MinioClient{Client: client}, nil
}

func ensureBucket(ctx context.Context, client *minio.Client, name string, logger *slog.Logger) error {
	exists, err := client.BucketExists(ctx, name)
	if err != nil {
		logger.Error("Failed to check bucket existence", "bucket", name, "error", err)
		return err
	}
	if exists {
		return nil
	}
	if err := client.MakeBucket(ctx, name, minio.MakeBucketOptions{}); err != nil {
		logger.Error("Failed to create bucket", "bucket", name, "error", err)
		return err
	}
	return nil
}

func sleepWithContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func withJitter(base time.Duration) time.Duration {
	if base <= 0 {
		return 0
	}
	// +/-20% jitter to avoid synchronized reconnect spikes.
	factor := 0.8 + (minioRetryRandFloat64() * 0.4)
	return time.Duration(float64(base) * factor)
}

func isRetryableMinioError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}
	msg := strings.ToLower(err.Error())
	retryableSubstrings := []string{
		"connection refused",
		"connection reset",
		"i/o timeout",
		"timeout",
		"no such host",
		"temporary",
	}
	for _, s := range retryableSubstrings {
		if strings.Contains(msg, s) {
			return true
		}
	}
	return false
}

func retryWithBackoff(ctx context.Context, maxAttempts int, initialBackoff, maxBackoff time.Duration, logger *slog.Logger, operation string, fn func(attempt int) error) error {
	if maxAttempts <= 0 {
		maxAttempts = 1
	}
	if initialBackoff <= 0 {
		initialBackoff = time.Second
	}
	if maxBackoff <= 0 {
		maxBackoff = 10 * time.Second
	}

	delay := initialBackoff
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		err := fn(attempt)
		if err == nil {
			return nil
		}
		lastErr = err

		if !isRetryableMinioError(err) || attempt == maxAttempts {
			logger.Error("MinIO operation failed", "operation", operation, "attempt", attempt, "max_attempts", maxAttempts, "error", err)
			break
		}

		nextDelay := withJitter(delay)
		logger.Warn("MinIO operation retrying", "operation", operation, "attempt", attempt, "max_attempts", maxAttempts, "next_delay", nextDelay.String(), "error", err)
		if sleepErr := minioRetrySleep(ctx, nextDelay); sleepErr != nil {
			return sleepErr
		}

		delay *= 2
		if delay > maxBackoff {
			delay = maxBackoff
		}
	}

	return lastErr
}
