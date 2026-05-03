-- User-creatable pipes. Each row carves out a named pipe on a source with
-- optional retention overrides. NULL values fall back to the default JetStream
-- stream config (24h / 1000 msgs / 64 MiB) at provisioning time.
--
-- Auto-provisioned pipes (broadcast, stdin, stdout) are NOT stored here —
-- they're derived from the source's kind. The server-side handler joins
-- auto + user pipes when responding to GET /api/v1/sources.

CREATE TABLE IF NOT EXISTS pipes (
    id           uuid PRIMARY KEY,
    source_id    uuid NOT NULL REFERENCES sources(id) ON DELETE CASCADE,
    name         text NOT NULL,
    ttl_seconds  int,
    max_msgs     int,
    max_bytes    bigint,
    created_at   timestamptz NOT NULL DEFAULT now(),
    UNIQUE (source_id, name)
);

CREATE INDEX IF NOT EXISTS pipes_source_idx ON pipes (source_id);
