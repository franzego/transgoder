package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/franzego/transgoder/internal/models"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestCanTransition(t *testing.T) {
	tests := []struct {
		name string
		from models.Status
		to   models.Status
		want bool
	}{
		{name: "pending to queued is valid", from: models.StatusPending, to: models.StatusQueued, want: true},
		{name: "queued to completed is invalid", from: models.StatusQueued, to: models.StatusCompleted, want: false},
		{name: "completed to failed is invalid", from: models.StatusCompleted, to: models.StatusFailed, want: false},
		{name: "processing to uploading is valid", from: models.StatusProcessing, to: models.StatusUploading, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := canTransition(tt.from, tt.to)
			if got != tt.want {
				t.Fatalf("canTransition(%s, %s) = %v, want %v", tt.from, tt.to, got, tt.want)
			}
		})
	}
}

func TestRepoService_TransitionTo(t *testing.T) {
	t.Run("invalid transition returns service error", func(t *testing.T) {
		svc := buildRepoServiceWithDB(&fakeDB{
			queryRow: func(_ context.Context, _ string, _ ...interface{}) pgx.Row {
				t.Fatal("db should not be called for invalid transition")
				return fakeRow{}
			},
		})

		err := svc.TransitionTo(context.Background(), "JB-1", models.StatusQueued, models.StatusCompleted)
		var se *ServiceError
		if !errors.As(err, &se) {
			t.Fatalf("expected ServiceError, got %T", err)
		}
		if !errors.Is(err, ErrInvalidTransition) {
			t.Fatalf("expected ErrInvalidTransition, got %v", err)
		}
	})

	t.Run("valid transition updates status", func(t *testing.T) {
		ts := pgtype.Timestamptz{}
		db := &fakeDB{
			queryRow: func(_ context.Context, sql string, args ...interface{}) pgx.Row {
				if !strings.Contains(sql, "UpdateJobStatus") {
					return fakeRow{err: errors.New("unexpected query")}
				}
				if len(args) != 2 {
					return fakeRow{err: errors.New("unexpected query args")}
				}
				if args[0] != "JB-2" || args[1] != string(models.StatusQueued) {
					return fakeRow{err: errors.New("unexpected transition args")}
				}
				return fakeRow{values: []interface{}{int32(1), "JB-2", string(models.StatusQueued), ts, ts}}
			},
		}
		svc := buildRepoServiceWithDB(db)

		err := svc.TransitionTo(context.Background(), "JB-2", models.StatusPending, models.StatusQueued)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("db failure is wrapped", func(t *testing.T) {
		db := &fakeDB{
			queryRow: func(_ context.Context, sql string, _ ...interface{}) pgx.Row {
				if strings.Contains(sql, "UpdateJobStatus") {
					return fakeRow{err: errors.New("db down")}
				}
				return fakeRow{err: errors.New("unexpected query")}
			},
		}
		svc := buildRepoServiceWithDB(db)

		err := svc.TransitionTo(context.Background(), "JB-3", models.StatusPending, models.StatusQueued)
		var se *ServiceError
		if !errors.As(err, &se) {
			t.Fatalf("expected ServiceError, got %T", err)
		}
		if se.Message != "Failed to update job status" {
			t.Fatalf("unexpected message: %q", se.Message)
		}
	})
}
