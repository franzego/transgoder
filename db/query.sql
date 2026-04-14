-- name: CreateJob :one
INSERT INTO jobs (job_id, status)
VALUES ($1, COALESCE($2, 'pending'))
RETURNING id, job_id, status, created_at, updated_at;

-- name: GetJobByID :one
SELECT id, job_id, status, created_at, updated_at
FROM jobs
WHERE id = $1;

-- name: GetJobByJobID :one
SELECT id, job_id, status, created_at, updated_at
FROM jobs
WHERE job_id = $1;

-- name: ListJobs :many
SELECT id, job_id, status, created_at, updated_at
FROM jobs
ORDER BY created_at DESC
LIMIT $1 OFFSET $2;

-- name: UpdateJobStatus :one
UPDATE jobs
SET status = $2,
    updated_at = NOW()
WHERE id = $1
RETURNING id, job_id, status, created_at, updated_at;

-- name: DeleteJob :exec
DELETE FROM jobs
WHERE id = $1;

-- name: CreateVideoMeta :one
INSERT INTO videometa (
	job_id,
	video_name,
	description,
	format,
	bitrate,
	resolution,
	duration
)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING id, job_id, video_name, description, format, bitrate, resolution, duration, created_at, updated_at;

-- name: GetVideoMetaByID :one
SELECT id, job_id, video_name, description, format, bitrate, resolution, duration, created_at, updated_at
FROM videometa
WHERE id = $1;

-- name: GetVideoMetaByJobID :one
SELECT id, job_id, video_name, description, format, bitrate, resolution, duration, created_at, updated_at
FROM videometa
WHERE job_id = $1;

-- name: ListVideoMeta :many
SELECT id, job_id, video_name, description, format, bitrate, resolution, duration, created_at, updated_at
FROM videometa
ORDER BY created_at DESC
LIMIT $1 OFFSET $2;

-- name: UpdateVideoMeta :one
UPDATE videometa
SET video_name = $2,
	description = $3,
	format = $4,
	bitrate = $5,
	resolution = $6,
	duration = $7,
	updated_at = NOW()
WHERE id = $1
RETURNING id, job_id, video_name, description, format, bitrate, resolution, duration, created_at, updated_at;

-- name: DeleteVideoMeta :exec
DELETE FROM videometa
WHERE id = $1;
