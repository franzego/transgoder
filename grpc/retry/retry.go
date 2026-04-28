package retry

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"net"
	"time"

	"github.com/franzego/transcoder/grpc/weberror"
)

type Retry struct {
	MaxRetries       int
	InitialBackoff   time.Duration
	MaxBackoff       time.Duration
	Jitter           time.Duration
	sleepwithcontext func(context.Context, time.Duration) error
	randomIntn       func(int64) int64
}

func NewRetry() *Retry {
	rnd := rand.New(rand.NewSource(time.Now().UnixNano()))

	return &Retry{
		MaxRetries:       3,
		InitialBackoff:   500 * time.Millisecond,
		MaxBackoff:       2 * time.Second,
		Jitter:           70 * time.Millisecond,
		sleepwithcontext: sleepWithContext,
		randomIntn:       rnd.Int63n,
	}
}

func (r *Retry) backoff(attempt int) time.Duration {
	exponential := math.Pow(2, float64(attempt))
	backoffDuration := time.Duration(float64(r.InitialBackoff) * exponential)
	if backoffDuration > r.MaxBackoff {
		backoffDuration = r.MaxBackoff
	}
	if r.Jitter > 0 && r.randomIntn != nil {
		backoffDuration += time.Duration(r.randomIntn(r.Jitter.Nanoseconds()))
	}

	return backoffDuration
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

func isRetryableError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}

	var webErr *weberror.RequestError
	if errors.As(err, &webErr) {
		return webErr.Retryable()
	}

	var netErr net.Error
	if errors.As(err, &netErr) {
		return netErr.Timeout()
	}

	return false
}

func (r *Retry) Do(ctx context.Context, operation string, fn func() error) error {
	return r.DoWithCheck(ctx, operation, fn, isRetryableError)
}

func (r *Retry) DoWithCheck(ctx context.Context, operation string, fn func() error, retryable func(error) bool) error {
	if r == nil {
		return fmt.Errorf("retry: nil config")
	}
	if fn == nil {
		return fmt.Errorf("retry: nil function")
	}
	if retryable == nil {
		retryable = isRetryableError
	}

	attempts := r.MaxRetries + 1
	if attempts <= 0 {
		attempts = 1
	}

	var lastErr error
	for attempt := 0; attempt < attempts; attempt++ {
		if attempt > 0 {
			if err := r.sleepwithcontext(ctx, r.backoff(attempt-1)); err != nil {
				return err
			}
		}

		err := fn()
		if err == nil {
			return nil
		}
		lastErr = err

		if !retryable(err) {
			return fmt.Errorf("%s failed (non-retryable): %w", operation, err)
		}
	}

	return fmt.Errorf("%s failed after %d attempts: %w", operation, attempts, lastErr)
}
