package retry

import (
	"context"
	"errors"
	"math"
	"math/rand"
	"net"
	"time"

	"github.com/franzego/transcoder/grpc/webserver"
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
	// I need this for the jitter. Jitter is supposed to include some form of randomness to prevet retry storms.
	// So I am using a random number generator with a seed based on the current time.
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
	backoffDuration += time.Duration(r.randomIntn(r.Jitter.Nanoseconds()))

	return backoffDuration
}

// Context is important to respect cancellations and timeouts, especially in a payment processing system where we don't want to keep retrying indefinitely
//
//	if the client has already given up or if the request has timed out.
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

	var webErr *webserver.RequestError
	if errors.As(err, &webErr) {
		return webErr.Retryable()
	}

	var netErr net.Error
	if errors.As(err, &netErr) {
		return netErr.Timeout()
	}

	return false
}
