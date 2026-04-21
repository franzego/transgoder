package service

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/franzego/transcoder/grpc/connection"
	pb "github.com/franzego/transcoder/grpc/server"
	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestNewTranscoderService(t *testing.T) {
	client := &connection.RedisClient{Client: redis.NewClient(&redis.Options{Addr: "127.0.0.1:0"})}

	if got := NewTranscoderService(nil, nil); got == nil {
		t.Fatal("expected non-nil service even with nil deps")
	}
	if got := NewTranscoderService(testLogger(), client); got == nil {
		t.Fatal("expected non-nil service with valid deps")
	}
}

func TestTranscodeVideo_Validation(t *testing.T) {
	svc := NewTranscoderService(testLogger(), nil)

	tests := []*pb.TranscodeRequest{
		nil,
		{JobId: ""},
	}
	for _, req := range tests {
		_, err := svc.TranscodeVideo(context.Background(), req)
		if status.Code(err) != codes.InvalidArgument {
			t.Fatalf("expected InvalidArgument, got code=%s err=%v", status.Code(err), err)
		}
	}
}

func TestTranscodeVideo_RedisFailure(t *testing.T) {
	rdb := redis.NewClient(&redis.Options{
		Addr:         "127.0.0.1:0",
		DialTimeout:  20 * time.Millisecond,
		ReadTimeout:  20 * time.Millisecond,
		WriteTimeout: 20 * time.Millisecond,
	})
	defer rdb.Close()

	svc := NewTranscoderService(testLogger(), &connection.RedisClient{Client: rdb})
	_, err := svc.TranscodeVideo(context.Background(), &pb.TranscodeRequest{JobId: "job-1"})
	if status.Code(err) != codes.Internal {
		t.Fatalf("expected Internal code, got code=%s err=%v", status.Code(err), err)
	}
}
