package webserver

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

type JobStatusReq struct {
	JobID string `json:"id" validate:"required"`
	From  Status `json:"from" validate:"required,oneof=pending queued downloading processing uploading completed failed cancelled"`
	To    Status `json:"to" validate:"required,oneof=pending queued downloading processing uploading completed failed cancelled"`
}
type JobStatusResponse struct {
	Message string `json:"message"`
}

type SourceURLResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Code    int    `json:"code"`
	Metadata struct {
		JobID     string `json:"job_id"`
		SourceURL string `json:"source_url"`
	} `json:"metadata"`
}
