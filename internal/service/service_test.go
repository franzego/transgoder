package service

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/franzego/transcoder/internal/models"
	"github.com/franzego/transcoder/internal/repository"
	"github.com/franzego/transcoder/internal/sqlc"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

type fakeDB struct {
	exec     func(ctx context.Context, sql string, args ...interface{}) (pgconn.CommandTag, error)
	query    func(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error)
	queryRow func(ctx context.Context, sql string, args ...interface{}) pgx.Row
}

func (f *fakeDB) Exec(ctx context.Context, sql string, args ...interface{}) (pgconn.CommandTag, error) {
	if f.exec != nil {
		return f.exec(ctx, sql, args...)
	}
	return pgconn.CommandTag{}, errors.New("not implemented")
}

func (f *fakeDB) Query(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error) {
	if f.query != nil {
		return f.query(ctx, sql, args...)
	}
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
		_, err := svc.CreateJob(context.Background(), "")
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

func TestRepoService_GetJobByJobID(t *testing.T) {
	t.Run("invalid job ID", func(t *testing.T) {
		svc := buildRepoServiceWithDB(&fakeDB{})
		_, err := svc.GetJobByJobID(context.Background(), "")
		if !errors.Is(err, ErrInvalidJobID) {
			t.Fatalf("expected ErrInvalidJobID, got %v", err)
		}
	})

	t.Run("db error is wrapped", func(t *testing.T) {
		db := &fakeDB{
			queryRow: func(_ context.Context, sql string, _ ...interface{}) pgx.Row {
				if strings.Contains(sql, "GetJobByJobID") {
					return fakeRow{err: errors.New("db down")}
				}
				return fakeRow{err: errors.New("unexpected query")}
			},
		}
		svc := buildRepoServiceWithDB(db)
		_, err := svc.GetJobByJobID(context.Background(), "job-1")
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
				if strings.Contains(sql, "GetJobByJobID") {
					return fakeRow{values: []interface{}{int32(4), "job-1", "pending", ts, ts}}
				}
				return fakeRow{err: errors.New("unexpected query")}
			},
		}
		svc := buildRepoServiceWithDB(db)
		job, err := svc.GetJobByJobID(context.Background(), "job-1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if job.ID != 4 || job.JobID != "job-1" {
			t.Fatalf("unexpected job: %+v", job)
		}
	})
}

func TestRepoService_UpdateJobStatus(t *testing.T) {
	arg := sqlc.UpdateJobStatusParams{JobID: "job-1", Status: "processing"}

	t.Run("db error is wrapped", func(t *testing.T) {
		db := &fakeDB{
			queryRow: func(_ context.Context, sql string, _ ...interface{}) pgx.Row {
				if strings.Contains(sql, "UpdateJobStatus") {
					return fakeRow{err: errors.New("write failed")}
				}
				return fakeRow{err: errors.New("unexpected query")}
			},
		}
		svc := buildRepoServiceWithDB(db)
		_, err := svc.UpdateJobStatus(context.Background(), arg)
		var se *ServiceError
		if !errors.As(err, &se) {
			t.Fatalf("expected ServiceError, got %T", err)
		}
		if se.Message != "failed to update job status" {
			t.Fatalf("unexpected message: %q", se.Message)
		}
	})

	t.Run("success", func(t *testing.T) {
		ts := pgtype.Timestamptz{}
		db := &fakeDB{
			queryRow: func(_ context.Context, sql string, _ ...interface{}) pgx.Row {
				if strings.Contains(sql, "UpdateJobStatus") {
					return fakeRow{values: []interface{}{int32(1), "job-1", "processing", ts, ts}}
				}
				return fakeRow{err: errors.New("unexpected query")}
			},
		}
		svc := buildRepoServiceWithDB(db)
		job, err := svc.UpdateJobStatus(context.Background(), arg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if job.Status != "processing" {
			t.Fatalf("unexpected status: %s", job.Status)
		}
	})
}

func TestRepoService_CreateVideoMeta(t *testing.T) {
	base := models.VideoMedataReq{
		JobID:      "job-1",
		VideoName:  pgtype.Text{String: "video.mp4", Valid: true},
		Format:     pgtype.Text{String: "mp4", Valid: true},
		Codec:      "h264",
		Bitrate:    pgtype.Int4{Int32: 1200, Valid: true},
		Resolution: pgtype.Text{String: "1920x1080", Valid: true},
		Duration:   pgtype.Int4{Int32: 60, Valid: true},
		Framerate:  pgtype.Int4{Int32: 30, Valid: true},
	}

	t.Run("invalid job ID", func(t *testing.T) {
		svc := buildRepoServiceWithDB(&fakeDB{})
		req := base
		req.JobID = ""
		_, err := svc.CreateVideoMeta(context.Background(), req)
		if !errors.Is(err, ErrInvalidJobID) {
			t.Fatalf("expected ErrInvalidJobID, got %v", err)
		}
	})

	t.Run("missing required fields", func(t *testing.T) {
		svc := buildRepoServiceWithDB(&fakeDB{})
		req := base
		req.Codec = ""
		_, err := svc.CreateVideoMeta(context.Background(), req)
		var se *ServiceError
		if !errors.As(err, &se) {
			t.Fatalf("expected ServiceError, got %T", err)
		}
		if se.Code != 400 {
			t.Fatalf("expected 400, got %d", se.Code)
		}
	})

	t.Run("db error is wrapped", func(t *testing.T) {
		db := &fakeDB{
			queryRow: func(_ context.Context, sql string, _ ...interface{}) pgx.Row {
				if strings.Contains(sql, "CreateVideoMeta") {
					return fakeRow{err: errors.New("insert failed")}
				}
				return fakeRow{err: errors.New("unexpected query")}
			},
		}
		svc := buildRepoServiceWithDB(db)
		_, err := svc.CreateVideoMeta(context.Background(), base)
		var se *ServiceError
		if !errors.As(err, &se) {
			t.Fatalf("expected ServiceError, got %T", err)
		}
		if se.Message != "failed to create video metadata" {
			t.Fatalf("unexpected message: %q", se.Message)
		}
	})

	t.Run("success", func(t *testing.T) {
		ts := pgtype.Timestamptz{}
		db := &fakeDB{
			queryRow: func(_ context.Context, sql string, _ ...interface{}) pgx.Row {
				if strings.Contains(sql, "CreateVideoMeta") {
					return fakeRow{values: []interface{}{
						int32(5), "job-1", base.VideoName, base.Description, base.Format, base.Bitrate,
						base.Resolution, base.Duration, ts, ts, "h264", base.Framerate,
					}}
				}
				return fakeRow{err: errors.New("unexpected query")}
			},
		}
		svc := buildRepoServiceWithDB(db)
		item, err := svc.CreateVideoMeta(context.Background(), base)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if item.ID != 5 || item.JobID != "job-1" {
			t.Fatalf("unexpected item: %+v", item)
		}
	})
}

func TestRepoService_GetPresignedURLsByJobID_Errors(t *testing.T) {
	t.Run("job lookup failure", func(t *testing.T) {
		db := &fakeDB{
			queryRow: func(_ context.Context, sql string, _ ...interface{}) pgx.Row {
				if strings.Contains(sql, "GetJobByJobID") {
					return fakeRow{err: errors.New("not found")}
				}
				return fakeRow{err: errors.New("unexpected query")}
			},
		}
		svc := buildRepoServiceWithDB(db)
		_, err := svc.GetPresignedURLsByJobID(context.Background(), "job-1")
		var se *ServiceError
		if !errors.As(err, &se) {
			t.Fatalf("expected ServiceError, got %T", err)
		}
		if se.Message != "failed to get job" {
			t.Fatalf("unexpected message: %q", se.Message)
		}
	})

	t.Run("query failure after lookup", func(t *testing.T) {
		ts := pgtype.Timestamptz{}
		db := &fakeDB{
			queryRow: func(_ context.Context, sql string, _ ...interface{}) pgx.Row {
				if strings.Contains(sql, "GetJobByJobID") {
					return fakeRow{values: []interface{}{int32(1), "job-1", "pending", ts, ts}}
				}
				return fakeRow{err: errors.New("unexpected query")}
			},
			query: func(_ context.Context, sql string, _ ...interface{}) (pgx.Rows, error) {
				if strings.Contains(sql, "GetPresignedURLsByJobID") {
					return nil, errors.New("query failed")
				}
				return nil, errors.New("unexpected query")
			},
		}
		svc := buildRepoServiceWithDB(db)
		_, err := svc.GetPresignedURLsByJobID(context.Background(), "job-1")
		var se *ServiceError
		if !errors.As(err, &se) {
			t.Fatalf("expected ServiceError, got %T", err)
		}
		if !strings.Contains(se.Message, "failed to get presigned urls") {
			t.Fatalf("unexpected message: %q", se.Message)
		}
	})
}

func TestRepoService_DeleteJob(t *testing.T) {
	t.Run("invalid id", func(t *testing.T) {
		svc := buildRepoServiceWithDB(&fakeDB{})
		err := svc.DeleteJob(context.Background(), 0)
		if !errors.Is(err, ErrInvalidJobID) {
			t.Fatalf("expected ErrInvalidJobID, got %v", err)
		}
	})

	t.Run("db error is wrapped", func(t *testing.T) {
		db := &fakeDB{
			exec: func(_ context.Context, sql string, _ ...interface{}) (pgconn.CommandTag, error) {
				if strings.Contains(sql, "DeleteJob") {
					return pgconn.CommandTag{}, errors.New("delete failed")
				}
				return pgconn.CommandTag{}, errors.New("unexpected query")
			},
		}
		svc := buildRepoServiceWithDB(db)
		err := svc.DeleteJob(context.Background(), 1)
		var se *ServiceError
		if !errors.As(err, &se) {
			t.Fatalf("expected ServiceError, got %T", err)
		}
		if se.Message != "failed to delete job" {
			t.Fatalf("unexpected message: %q", se.Message)
		}
	})

	t.Run("success", func(t *testing.T) {
		db := &fakeDB{
			exec: func(_ context.Context, sql string, _ ...interface{}) (pgconn.CommandTag, error) {
				if strings.Contains(sql, "DeleteJob") {
					return pgconn.CommandTag{}, nil
				}
				return pgconn.CommandTag{}, errors.New("unexpected query")
			},
		}
		svc := buildRepoServiceWithDB(db)
		if err := svc.DeleteJob(context.Background(), 1); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}
