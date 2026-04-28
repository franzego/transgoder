package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/franzego/transcoder/internal/models"
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
		ts := pgtype.Timestamptz{}
		svc := buildRepoServiceWithDB(&fakeDB{
			queryRow: func(_ context.Context, sql string, _ ...interface{}) pgx.Row {
				if strings.Contains(sql, "GetJobByJobID") {
					return fakeRow{values: []interface{}{int32(1), "JB-1", string(models.StatusQueued), ts, ts}}
				}
				if strings.Contains(sql, "UpdateJobStatus") {
					t.Fatal("db should not call update for invalid transition")
				}
				return fakeRow{err: errors.New("unexpected query")}
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
		calls := 0
		db := &fakeDB{
			queryRow: func(_ context.Context, sql string, args ...interface{}) pgx.Row {
				calls++
				switch {
				case strings.Contains(sql, "GetJobByJobID"):
					if len(args) != 1 || args[0] != "JB-2" {
						return fakeRow{err: errors.New("unexpected get args")}
					}
					return fakeRow{values: []interface{}{int32(1), "JB-2", string(models.StatusPending), ts, ts}}
				case strings.Contains(sql, "UpdateJobStatus"):
					if len(args) != 2 {
						return fakeRow{err: errors.New("unexpected query args")}
					}
					if args[0] != "JB-2" || args[1] != string(models.StatusQueued) {
						return fakeRow{err: errors.New("unexpected transition args")}
					}
					return fakeRow{values: []interface{}{int32(1), "JB-2", string(models.StatusQueued), ts, ts}}
				default:
					return fakeRow{err: errors.New("unexpected query")}
				}
			},
		}
		svc := buildRepoServiceWithDB(db)

		err := svc.TransitionTo(context.Background(), "JB-2", models.StatusPending, models.StatusQueued)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if calls != 2 {
			t.Fatalf("expected 2 db calls, got %d", calls)
		}
	})

	t.Run("db failure is wrapped", func(t *testing.T) {
		db := &fakeDB{
			queryRow: func(_ context.Context, sql string, _ ...interface{}) pgx.Row {
				if strings.Contains(sql, "GetJobByJobID") {
					ts := pgtype.Timestamptz{}
					return fakeRow{values: []interface{}{int32(1), "JB-3", string(models.StatusPending), ts, ts}}
				}
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

	t.Run("stale transition returns conflict", func(t *testing.T) {
		ts := pgtype.Timestamptz{}
		svc := buildRepoServiceWithDB(&fakeDB{
			queryRow: func(_ context.Context, sql string, _ ...interface{}) pgx.Row {
				if strings.Contains(sql, "GetJobByJobID") {
					return fakeRow{values: []interface{}{int32(1), "JB-4", string(models.StatusCompleted), ts, ts}}
				}
				if strings.Contains(sql, "UpdateJobStatus") {
					t.Fatal("db should not update for stale transition")
				}
				return fakeRow{err: errors.New("unexpected query")}
			},
		})

		err := svc.TransitionTo(context.Background(), "JB-4", models.StatusQueued, models.StatusDownloading)
		var se *ServiceError
		if !errors.As(err, &se) {
			t.Fatalf("expected ServiceError, got %T", err)
		}
		if !errors.Is(err, ErrStaleTransition) {
			t.Fatalf("expected ErrStaleTransition, got %v", err)
		}
		if se.Code != 409 {
			t.Fatalf("expected conflict code, got %d", se.Code)
		}
	})

	t.Run("same-state transition is idempotent", func(t *testing.T) {
		ts := pgtype.Timestamptz{}
		svc := buildRepoServiceWithDB(&fakeDB{
			queryRow: func(_ context.Context, sql string, _ ...interface{}) pgx.Row {
				if strings.Contains(sql, "GetJobByJobID") {
					return fakeRow{values: []interface{}{int32(1), "JB-5", string(models.StatusQueued), ts, ts}}
				}
				if strings.Contains(sql, "UpdateJobStatus") {
					t.Fatal("db should not update for same-state transition")
				}
				return fakeRow{err: errors.New("unexpected query")}
			},
		})

		err := svc.TransitionTo(context.Background(), "JB-5", models.StatusQueued, models.StatusQueued)
		if err != nil {
			t.Fatalf("expected nil for idempotent transition, got %v", err)
		}
	})
}
