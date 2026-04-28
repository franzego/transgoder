package connection

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"
)

func TestRetryWithBackoff_RetriesAndEventuallySucceeds(t *testing.T) {
	oldSleep := minioRetrySleep
	oldRand := minioRetryRandFloat64
	t.Cleanup(func() {
		minioRetrySleep = oldSleep
		minioRetryRandFloat64 = oldRand
	})

	sleepCalls := 0
	minioRetrySleep = func(context.Context, time.Duration) error {
		sleepCalls++
		return nil
	}
	minioRetryRandFloat64 = func() float64 { return 0 }

	attempts := 0
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	err := retryWithBackoff(context.Background(), 5, time.Millisecond, 5*time.Millisecond, logger, "test", func(_ int) error {
		attempts++
		if attempts < 3 {
			return errors.New("connect: connection refused")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if attempts != 3 {
		t.Fatalf("expected 3 attempts, got %d", attempts)
	}
	if sleepCalls != 2 {
		t.Fatalf("expected 2 sleep calls, got %d", sleepCalls)
	}
}

func TestRetryWithBackoff_DoesNotRetryNonRetryable(t *testing.T) {
	oldSleep := minioRetrySleep
	t.Cleanup(func() {
		minioRetrySleep = oldSleep
	})

	sleepCalls := 0
	minioRetrySleep = func(context.Context, time.Duration) error {
		sleepCalls++
		return nil
	}

	attempts := 0
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	err := retryWithBackoff(context.Background(), 5, time.Millisecond, 5*time.Millisecond, logger, "test", func(_ int) error {
		attempts++
		return errors.New("access denied")
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if attempts != 1 {
		t.Fatalf("expected single attempt, got %d", attempts)
	}
	if sleepCalls != 0 {
		t.Fatalf("expected no sleep calls, got %d", sleepCalls)
	}
}

func TestRetryWithBackoff_ExhaustsAttemptsOnRetryableError(t *testing.T) {
	oldSleep := minioRetrySleep
	t.Cleanup(func() {
		minioRetrySleep = oldSleep
	})

	sleepCalls := 0
	minioRetrySleep = func(context.Context, time.Duration) error {
		sleepCalls++
		return nil
	}

	attempts := 0
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	err := retryWithBackoff(context.Background(), 3, time.Millisecond, 5*time.Millisecond, logger, "test", func(_ int) error {
		attempts++
		return errors.New("dial tcp: i/o timeout")
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if attempts != 3 {
		t.Fatalf("expected 3 attempts, got %d", attempts)
	}
	if sleepCalls != 2 {
		t.Fatalf("expected 2 sleep calls, got %d", sleepCalls)
	}
}
