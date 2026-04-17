package service

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/franzego/transgoder/internal/repository"
	"github.com/franzego/transgoder/internal/sqlc"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

type fakeDB struct {
	queryRow func(ctx context.Context, sql string, args ...interface{}) pgx.Row
}

func (f *fakeDB) Exec(context.Context, string, ...interface{}) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, errors.New("not implemented")
}

func (f *fakeDB) Query(context.Context, string, ...interface{}) (pgx.Rows, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeDB) QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row {
	if f.queryRow == nil {
		return fakeRow{err: errors.New("queryRow not configured")}
	}
	return f.queryRow(ctx, sql, args...)
}

type fakeRow struct {
	values []interface{}
	err    error
}

func (r fakeRow) Scan(dest ...interface{}) error {
	if r.err != nil {
		return r.err
	}
	if len(dest) != len(r.values) {
		return fmt.Errorf("scan mismatch: got %d dest, want %d", len(dest), len(r.values))
	}
	for i := range dest {
		if err := assign(dest[i], r.values[i]); err != nil {
			return err
		}
	}
	return nil
}

func assign(dst, src interface{}) error {
	dv := reflect.ValueOf(dst)
	if dv.Kind() != reflect.Ptr || dv.IsNil() {
		return errors.New("destination must be non-nil pointer")
	}
	target := dv.Elem()
	if src == nil {
		target.SetZero()
		return nil
	}
	sv := reflect.ValueOf(src)
	if sv.Type().AssignableTo(target.Type()) {
		target.Set(sv)
		return nil
	}
	if sv.Type().ConvertibleTo(target.Type()) {
		target.Set(sv.Convert(target.Type()))
		return nil
	}
	return fmt.Errorf("cannot assign %s to %s", sv.Type(), target.Type())
}

func buildRepoServiceWithDB(db sqlc.DBTX) *RepoService {
	repo := &repository.Repo{
		Q: sqlc.New(db),
	}
	return NewRepoService(repo)
}

func TestRepoService_CreateJob(t *testing.T) {
	t.Run("invalid job ID", func(t *testing.T) {
		svc := buildRepoServiceWithDB(&fakeDB{})
		_, err := svc.CreateJob(context.Background(), "sqlc.CreateJobParams{}")
		if !errors.Is(err, ErrInvalidJobID) {
			t.Fatalf("expected ErrInvalidJobID, got %v", err)
		}
	})

	t.Run("db error is wrapped as service error", func(t *testing.T) {
		db := &fakeDB{
			queryRow: func(_ context.Context, sql string, _ ...interface{}) pgx.Row {
				if strings.Contains(sql, "CreateJob") {
					return fakeRow{err: errors.New("db down")}
				}
				return fakeRow{err: errors.New("unexpected query")}
			},
		}
		svc := buildRepoServiceWithDB(db)
		_, err := svc.CreateJob(context.Background(), "job-123")
		var se *ServiceError
		if !errors.As(err, &se) {
			t.Fatalf("expected ServiceError, got %T", err)
		}
		if se.Code != 500 || se.Message != "failed to create job" {
			t.Fatalf("unexpected service error: %+v", se)
		}
		if !strings.Contains(se.Err.Error(), "db down") {
			t.Fatalf("expected wrapped db error, got %v", se.Err)
		}
	})

	t.Run("success", func(t *testing.T) {
		ts := pgtype.Timestamptz{}
		db := &fakeDB{
			queryRow: func(_ context.Context, sql string, _ ...interface{}) pgx.Row {
				if strings.Contains(sql, "CreateJob") {
					return fakeRow{values: []interface{}{int32(1), "job-1", "pending", ts, ts}}
				}
				return fakeRow{err: errors.New("unexpected query")}
			},
		}
		svc := buildRepoServiceWithDB(db)
		job, err := svc.CreateJob(context.Background(), "job-1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if job.ID != 1 || job.JobID != "job-1" || job.Status != "pending" {
			t.Fatalf("unexpected job: %+v", job)
		}
	})
}

func TestRepoService_UpdateJobStatus_Validation(t *testing.T) {
	svc := buildRepoServiceWithDB(&fakeDB{})
	_, err := svc.UpdateJobStatus(context.Background(), sqlc.UpdateJobStatusParams{JobID: "", Status: "done"})
	if !errors.Is(err, ErrInvalidJobID) {
		t.Fatalf("expected ErrInvalidJobID, got %v", err)
	}
}

func TestRepoService_CreatePresignedURL(t *testing.T) {
	t.Run("invalid job ID", func(t *testing.T) {
		svc := buildRepoServiceWithDB(&fakeDB{})
		_, err := svc.CreatePresignedURL(context.Background(), "", "https://example.com", 1)
		if !errors.Is(err, ErrInvalidJobID) {
			t.Fatalf("expected ErrInvalidJobID, got %v", err)
		}
	})

	t.Run("fails when job lookup fails", func(t *testing.T) {
		db := &fakeDB{
			queryRow: func(_ context.Context, sql string, _ ...interface{}) pgx.Row {
				if strings.Contains(sql, "GetJobByJobID") {
					return fakeRow{err: errors.New("job not found")}
				}
				return fakeRow{err: errors.New("unexpected query")}
			},
		}
		svc := buildRepoServiceWithDB(db)
		_, err := svc.CreatePresignedURL(context.Background(), "job-1", "https://example.com", 2)
		var se *ServiceError
		if !errors.As(err, &se) {
			t.Fatalf("expected ServiceError, got %T", err)
		}
		if se.Message != "failed to get job" {
			t.Fatalf("unexpected message: %q", se.Message)
		}
	})

	t.Run("success", func(t *testing.T) {
		ts := pgtype.Timestamptz{}
		db := &fakeDB{
			queryRow: func(_ context.Context, sql string, _ ...interface{}) pgx.Row {
				switch {
				case strings.Contains(sql, "GetJobByJobID"):
					return fakeRow{values: []interface{}{int32(1), "job-1", "pending", ts, ts}}
				case strings.Contains(sql, "CreatePresignedURL"):
					return fakeRow{values: []interface{}{int32(9), "job-1", int32(2), "https://example.com", ts}}
				default:
					return fakeRow{err: errors.New("unexpected query")}
				}
			},
		}
		svc := buildRepoServiceWithDB(db)
		item, err := svc.CreatePresignedURL(context.Background(), "job-1", "https://example.com", 2)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if item.ID != 9 || item.JobID != "job-1" || item.PartNumber != 2 {
			t.Fatalf("unexpected item: %+v", item)
		}
	})
}

func TestRepoService_GetPresignedURLsByJobID_Validation(t *testing.T) {
	svc := buildRepoServiceWithDB(&fakeDB{})
	_, err := svc.GetPresignedURLsByJobID(context.Background(), "")
	if !errors.Is(err, ErrEmptyJobID) {
		t.Fatalf("expected ErrEmptyJobID, got %v", err)
	}
}
