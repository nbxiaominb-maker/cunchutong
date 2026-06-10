CREATE TABLE IF NOT EXISTS files (
    id          TEXT PRIMARY KEY,
    sha256      TEXT NOT NULL,
    filename    TEXT NOT NULL,
    size        INTEGER NOT NULL,
    mime_type   TEXT NOT NULL,
    bucket      TEXT NOT NULL DEFAULT 'default',
    tags        TEXT,
    is_public   INTEGER NOT NULL DEFAULT 1,
    ref_count   INTEGER NOT NULL DEFAULT 1,
    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_files_sha256 ON files(sha256);
CREATE INDEX IF NOT EXISTS idx_files_bucket ON files(bucket);
CREATE INDEX IF NOT EXISTS idx_files_created_at ON files(created_at);
CREATE INDEX IF NOT EXISTS idx_files_mime_type ON files(mime_type);

CREATE TABLE IF NOT EXISTS api_keys (
    key         TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    permissions TEXT NOT NULL DEFAULT 'read,write',
    created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    last_used   TEXT
);

CREATE TABLE IF NOT EXISTS multipart_uploads (
    upload_id   TEXT PRIMARY KEY,
    filename    TEXT NOT NULL,
    total_size  INTEGER NOT NULL,
    mime_type   TEXT,
    bucket      TEXT NOT NULL DEFAULT 'default',
    chunk_size  INTEGER NOT NULL,
    total_chunks INTEGER NOT NULL,
    received_chunks TEXT NOT NULL DEFAULT '{}',
    status      TEXT NOT NULL DEFAULT 'pending',
    created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);
