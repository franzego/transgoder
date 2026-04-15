CREATE TABLE IF NOT EXISTS presigned_urls (
    id SERIAL PRIMARY KEY,
    job_id VARCHAR(20) NOT NULL REFERENCES jobs(job_id) ON DELETE CASCADE,
    part_number INTEGER NOT NULL,
    presigned_url TEXT NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_presigned_urls_job_id ON presigned_urls (job_id);
CREATE INDEX IF NOT EXISTS idx_presigned_urls_job_id_part_number ON presigned_urls (job_id, part_number);
