-- Initial schema. Three tables; no users / sessions / workspaces — those are
-- deferred. The migration runner re-applies every boot, so every statement is
-- IF-NOT-EXISTS-style for idempotency.
--
-- Terminology note (Phase 1 / v0.30): the tenancy boundary is called
-- "account" throughout. Earlier OSS pre-releases called it "organisation";
-- that name was retired pre-launch as the user-facing organisation concept
-- moved to pipescloud's control plane. See OSS-PIPESCLOUD-ARCHITECTURE-SPLIT
-- (private), locked decisions #11 and #18.

CREATE TABLE IF NOT EXISTS accounts (
    id          uuid PRIMARY KEY,
    name        text NOT NULL UNIQUE,
    created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS api_keys (
    id          uuid PRIMARY KEY,
    account_id  uuid NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    key_hash    text NOT NULL,
    key_prefix  char(8) NOT NULL,
    label       text NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS api_keys_account_idx ON api_keys (account_id);
CREATE INDEX IF NOT EXISTS api_keys_prefix_idx  ON api_keys (key_prefix);

CREATE TABLE IF NOT EXISTS sources (
    id                       uuid PRIMARY KEY,
    account_id               uuid NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    handle                   text NOT NULL,
    kind                     text NOT NULL DEFAULT 'message',
    created_at               timestamptz NOT NULL DEFAULT now(),
    last_broadcast_at        timestamptz,
    last_broadcast_payload   text,
    UNIQUE (account_id, handle)
);

CREATE INDEX IF NOT EXISTS sources_account_handle_idx ON sources (account_id, handle);
