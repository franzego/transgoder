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

const (
	ErrorCodeValidation      = "VALIDATION_ERROR"
	ErrorCodeDependency      = "DEPENDENCY_ERROR"
	ErrorCodeInvalidState    = "INVALID_STATE"
	ErrorCodeInternal        = "INTERNAL_ERROR"
	ErrorCodePresetNotFound  = "PRESET_NOT_FOUND"
	ErrorCodePresetOverride  = "PRESET_OVERRIDE_INVALID"
	ErrorCodeTranscodeFailed = "TRANSCODE_FAILED"
)

type UpdateStatusRequest struct {
	JobID string `json:"id" validate:"required"`
	From  Status `json:"from" validate:"required,oneof=pending queued downloading processing uploading completed failed cancelled"`
	To    Status `json:"to" validate:"required,oneof=pending queued downloading processing uploading completed failed cancelled"`
}

type ApiMessage struct {
	Success   bool   `json:"success"`
	Message   string `json:"message"`
	Code      int    `json:"code"`
	ErrorCode string `json:"error_code,omitempty"`
	Error     string `json:"error,omitempty"`
	Metadata  any    `json:"metadata,omitempty"`
}

type JobStatusResponse struct {
	Status string `json:"status"`
}

type PresetOverrides struct {
	Codec      *string `json:"codec,omitempty"`
	Resolution *string `json:"resolution,omitempty"`
	Bitrate    *int32  `json:"bitrate,omitempty"`
	Framerate  *int32  `json:"framerate,omitempty"`
	Format     *string `json:"format,omitempty"`
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
	PresetID    string                `json:"preset_id,omitempty"`
	Overrides   PresetOverrides       `json:"overrides,omitempty"`
	Format      string                `json:"format"`
	Resolution  string                `json:"resolution"`
	Codec       string                `json:"codec"`
	Framerate   *int32                `json:"framerate"`
	Duration    *int32                `json:"duration"`
}

type CreateJobRequest struct {
	JobID       string          `json:"job_id,omitempty"`
	VideoName   string          `json:"video_name" binding:"required"`
	Description string          `json:"description,omitempty"`
	PresetID    string          `json:"preset_id" binding:"required"`
	Overrides   PresetOverrides `json:"overrides,omitempty"`
	Duration    *int32          `json:"duration,omitempty"`
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
