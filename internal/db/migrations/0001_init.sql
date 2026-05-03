-- Phase 2 schema (post terminology inversion). Three tables; no users /
-- sessions / workspaces — those are deferred. The migration runner re-applies
-- every boot, so every statement is IF-NOT-EXISTS-style for idempotency.

CREATE TABLE IF NOT EXISTS organisations (
    id          uuid PRIMARY KEY,
    name        text NOT NULL UNIQUE,
    created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS api_keys (
    id               uuid PRIMARY KEY,
    organisation_id  uuid NOT NULL REFERENCES organisations(id) ON DELETE CASCADE,
    key_hash         text NOT NULL,
    key_prefix       char(8) NOT NULL,
    label            text NOT NULL,
    created_at       timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS api_keys_org_idx ON api_keys (organisation_id);
CREATE INDEX IF NOT EXISTS api_keys_prefix_idx ON api_keys (key_prefix);

CREATE TABLE IF NOT EXISTS sources (
    id                       uuid PRIMARY KEY,
    organisation_id          uuid NOT NULL REFERENCES organisations(id) ON DELETE CASCADE,
    handle                   text NOT NULL,
    kind                     text NOT NULL DEFAULT 'message',
    created_at               timestamptz NOT NULL DEFAULT now(),
    last_broadcast_at        timestamptz,
    last_broadcast_payload   text,
    UNIQUE (organisation_id, handle)
);

CREATE INDEX IF NOT EXISTS sources_org_handle_idx ON sources (organisation_id, handle);
