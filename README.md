# Transcoder

Transcoder is a preset-driven internal video transcoding platform built around:
- Go API (upload/status/job orchestration)
- Go gRPC worker (doing all of the ffmpeg transcoding)
- Redis stream queue
- MinIO object storage
- Postgres metadata store
- Vue frontend for upload + job monitoring
The idea is to have it as a service where companies that require transcoding of their videos (cctv cams, private recordings and videos gotten from lower quality phones) can be transcoded to quality or standards that are easy to translate for editing or privavte work. Everything will be done in your ptivate network. 
Everything can be configured by the administrator or user.

## Highlights
- Multipart upload to MinIO with presigned part URLs
- Job lifecycle status machine (`pending -> queued -> downloading -> processing -> uploading -> completed|failed|cancelled`)
- Preset catalog with validated overrides
- Async worker pool with retries/backoff on dependency failures
- Health + readiness + basic metrics endpoints

## Quickstart (Docker)
1. Copy env file:
   - `cp example.env .env`
2. Start the full stack:
   - `docker compose up --build`
3. Verify service endpoints:
   - `GET http://localhost:8084/health`
   - `GET http://localhost:8084/ready`
   - `GET http://localhost:8084/metrics`

## Core API Endpoints
- `POST /upload/initiate`
- `POST /upload/complete`
- `POST /jobs` (preset-based job creation)
- `GET /jobs/:id/source-url`
- `GET /jobs/:id/output-url`
- `GET /jobs/:id/download`
- `GET /jobs/:id/transcode-profile`
- `POST /status/:id/update`
- `GET /status/:id/update`
- `GET /presets`
- `GET /presets/:id`

## Frontend
A simple frontend implmentation for this project.

### What changed in UI
- Preset picker wired to `GET /presets`
- Override fields (codec, resolution, bitrate, framerate, format)
- Upload completion payload now sends:
  - `preset_id`
  - `overrides`
- Existing multipart upload flow is preserved for source ingestion.

### Run frontend locally
```bash
cd frontend
npm install
npm run dev
```

Frontend dev server defaults to `http://localhost:3000` and proxies API requests to backend.

### Frontend request flow
1. `POST /upload/initiate`
2. Upload parts directly to MinIO with presigned URLs
3. `POST /upload/complete` with `preset_id` and optional `overrides`
4. Poll `GET /status/:id/update` until terminal state
5. Use `GET /jobs/:id/output-url` or `GET /jobs/:id/download`

## Preset Submission Example
```bash
curl -X POST http://localhost:8084/jobs \
  -H 'Content-Type: application/json' \
  -d '{
    "video_name": "sample.mp4",
    "description": "demo transcode",
    "preset_id": "web-h264-v1",
    "overrides": {
      "bitrate": 2500,
      "resolution": "720"
    }
  }'
```

## Error Contract
API errors include structured error codes in `error_code`:
- `VALIDATION_ERROR`
- `DEPENDENCY_ERROR`
- `INVALID_STATE`
- `INTERNAL_ERROR`
- `PRESET_NOT_FOUND`
- `PRESET_OVERRIDE_INVALID`
- `TRANSCODE_FAILED`

## Notes
- gRPC protobuf regeneration requires `protoc-gen-go` and `protoc-gen-go-grpc` to be installed.
- Repository module path: `github.com/franzego/transcoder`.
