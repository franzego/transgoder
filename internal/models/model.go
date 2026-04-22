// Package models contains the data models for the transcoder service.
package models

import "github.com/jackc/pgx/v5/pgtype"

type Status string

const (
	StatusPending     Status = "pending"
	StatusQueued      Status = "queued"
	StatusDownloading Status = "downloading"
	StatusProcessing  Status = "processing"
	StatusUploading   Status = "uploading" // after transcoding has been done, we upload back to minio
	StatusCompleted   Status = "completed"
	StatusFailed      Status = "failed"
	StatusCancelled   Status = "cancelled"
)

type UpdateStatusRequest struct {
	JobID string `json:"id" validate:"required"`
	From  Status `json:"from" validate:"required,oneof=pending queued downloading processing uploading completed failed cancelled"`
	To    Status `json:"to" validate:"required,oneof=pending queued downloading processing uploading completed failed cancelled"`
}

type ApiMessage struct {
	Success  bool   `json:"success"`
	Message  string `json:"message"`
	Code     int    `json:"code"`
	Error    string `json:"error,omitempty"`
	Metadata any    `json:"metadata,omitempty"`
}
type JobStatusResponse struct {
	Status string `json:"status"`
}

type MultipartInitiateRequest struct {
	FileName string `json:"file_name"`
	FileSize int64  `json:"file_size" binding:"required"`
	PartSize int64  `json:"part_size"`
}

type MultipartUploadPart struct {
	PartNumber int    `json:"part_number" binding:"required"`
	ETag       string `json:"etag" binding:"required"`
}

type MultipartCompleteRequest struct {
	JobID       string                `json:"job_id" binding:"required"`
	UploadID    string                `json:"upload_id" binding:"required"`
	Parts       []MultipartUploadPart `json:"parts" binding:"required"`
	VideoName   string                `json:"video_name"`
	Description string                `json:"description"`
	Format      string                `json:"format" binding:"required"` // mp4 or mov
	Codec       string                `json:"codec"`                     // h.264 or h.265
	Framerate   *int32                `json:"framerate"`                 // 1920x1080
	Duration    *int32                `json:"duration"`
}

type VideoMedataReq struct {
	JobID       string      `json:"job_id"`
	VideoName   pgtype.Text `json:"video_name"`
	Description pgtype.Text `json:"description"`
	Format      pgtype.Text `json:"format"`
	Bitrate     pgtype.Int4 `json:"bitrate"`
	Resolution  pgtype.Text `json:"resolution"`
	Codec       string      `json:"codec"`
	Framerate   pgtype.Int4 `json:"framerate"`
	Duration    pgtype.Int4 `json:"duration"`
}
