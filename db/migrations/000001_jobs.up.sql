CREATE TABLE IF NOT EXISTS jobs (
    id SERIAL PRIMARY KEY,
    job_id VARCHAR(20) NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_jobs_job_id ON jobs (job_id);

CREATE TABLE IF NOT EXISTS videometa (
    id SERIAL PRIMARY KEY,
    job_id VARCHAR(20) NOT NULL REFERENCES jobs(job_id) ON DELETE CASCADE,
    video_name TEXT,
    description TEXT,
    format TEXT,
    bitrate INTEGER,
    resolution TEXT,
    duration INTEGER,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_videometa_job_id ON videometa (job_id);