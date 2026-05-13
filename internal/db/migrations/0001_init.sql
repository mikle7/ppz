-- v0.30+ schema baseline.
--
-- Consolidated from the historical 0001–0010 migration chain. Pre-launch
-- (locked decision #5) we squash to a single file describing the final
-- state-of-the-world. Future migrations stack on top as 0002_*, 0003_*,
-- etc.
--
-- Why squash:
--   - Earlier migrations renamed `organisation` → `account`, added users +
--     OAuth + invites + CREATOR attribution in chained steps. The chain
--     was idempotent but accumulated cruft, and the v0.29 → v0.30 rename
--     didn't survive an upgrade cleanly. Squashing avoids the upgrade-path
--     trap entirely: environments are nuked + reinitialised against this
--     baseline.
--   - Anyone reading this file sees the schema, not its rename history.
--
-- Idempotent: every statement is IF NOT EXISTS so the migration runner
-- can re-apply this file on every boot without side effects.
--
-- Pre-condition for v0.29 → v0.30 deploys: the database must be empty.
-- Use the "Reset Database" GitHub Action to nuke before deploying. After
-- v0.30, normal additive migrations should work without nuke.

-- ─── Accounts (tenancy boundary) ──────────────────────────────────────

CREATE TABLE IF NOT EXISTS accounts (
    id                          uuid PRIMARY KEY,
    name                        text NOT NULL UNIQUE,
    owner_user_id               uuid,                       -- FK constraint added after users table exists
    created_at                  timestamptz NOT NULL DEFAULT now(),

    -- Auth V2 §Phase 3.5 — per-account NATS account JWT.
    nats_account_pub            text,
    nats_account_jwt            text,
    nats_account_signing_seed   text
);

CREATE INDEX IF NOT EXISTS accounts_nats_account_pub_idx ON accounts (nats_account_pub);

-- ─── Users ────────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS users (
    id          uuid PRIMARY KEY,
    username    text NOT NULL UNIQUE,
    email       text NOT NULL,
    mode        text NOT NULL,                              -- 'github' or 'internal'
    github_id   bigint,
    avatar_url  text,
    created_at  timestamptz NOT NULL DEFAULT now(),
    CHECK (mode IN ('github', 'internal'))
);

-- Partial unique on github_id: many internal users may have NULL; among
-- non-null GitHub IDs, each maps to one user row.
CREATE UNIQUE INDEX IF NOT EXISTS users_github_id_unique
    ON users (github_id)
    WHERE github_id IS NOT NULL;

-- Seed: the unauthenticated placeholder. Fixed UUID so tests / GUI flows
-- depending on a stable identity work deterministically across rebuilds.
INSERT INTO users (id, username, email, mode)
VALUES ('00000000-0000-0000-0000-000000000001',
        'unauthenticated',
        'unauthenticated@local',
        'internal')
ON CONFLICT (username) DO NOTHING;

-- accounts.owner_user_id FK — added now that users exists.
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.table_constraints
        WHERE table_name = 'accounts' AND constraint_name = 'accounts_owner_user_id_fkey'
    ) THEN
        ALTER TABLE accounts
            ADD CONSTRAINT accounts_owner_user_id_fkey
            FOREIGN KEY (owner_user_id) REFERENCES users(id);
    END IF;
END $$;

-- ─── Account membership ───────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS account_members (
    account_id  uuid NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    user_id     uuid NOT NULL REFERENCES users(id)    ON DELETE CASCADE,
    added_at    timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (account_id, user_id)
);

CREATE INDEX IF NOT EXISTS account_members_user_idx ON account_members (user_id);

-- ─── API keys ─────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS api_keys (
    id                  uuid PRIMARY KEY,
    account_id          uuid NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    created_by_user_id  uuid NOT NULL REFERENCES users(id),
    key_hash            text NOT NULL,
    key_prefix          char(8) NOT NULL,
    label               text NOT NULL,
    created_at          timestamptz NOT NULL DEFAULT now(),
    revoked_at          timestamptz                            -- NULL = active
);

CREATE INDEX IF NOT EXISTS api_keys_account_idx    ON api_keys (account_id);
CREATE INDEX IF NOT EXISTS api_keys_prefix_idx     ON api_keys (key_prefix);
CREATE INDEX IF NOT EXISTS api_keys_created_by_idx ON api_keys (created_by_user_id);

-- ─── Sources (actor identities — keep until Phase 1.5 collapses them) ─

CREATE TABLE IF NOT EXISTS sources (
    id                          uuid PRIMARY KEY,
    account_id                  uuid NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    created_by_user_id          uuid NOT NULL REFERENCES users(id),
    handle                      text NOT NULL,
    kind                        text NOT NULL DEFAULT 'message',
    created_at                  timestamptz NOT NULL DEFAULT now(),
    last_broadcast_at           timestamptz,                  -- dead post-Phase 1, kept until schema collapse
    last_broadcast_payload      text,
    UNIQUE (account_id, handle)
);

CREATE INDEX IF NOT EXISTS sources_account_handle_idx ON sources (account_id, handle);
CREATE INDEX IF NOT EXISTS sources_created_by_idx     ON sources (created_by_user_id);

-- ─── Pipes (user-creatable, with retention overrides) ─────────────────

CREATE TABLE IF NOT EXISTS pipes (
    id                  uuid PRIMARY KEY,
    source_id           uuid NOT NULL REFERENCES sources(id) ON DELETE CASCADE,
    created_by_user_id  uuid NOT NULL REFERENCES users(id),
    name                text NOT NULL,
    ttl_seconds         int,
    max_msgs            int,
    max_bytes           bigint,
    created_at          timestamptz NOT NULL DEFAULT now(),
    UNIQUE (source_id, name)
);

CREATE INDEX IF NOT EXISTS pipes_source_idx     ON pipes (source_id);
CREATE INDEX IF NOT EXISTS pipes_created_by_idx ON pipes (created_by_user_id);

-- ─── OAuth (device flow + bearer tokens) ──────────────────────────────

CREATE TABLE IF NOT EXISTS oauth_device_codes (
    device_code   text PRIMARY KEY,
    user_code     text NOT NULL UNIQUE,
    user_id       uuid REFERENCES users(id) ON DELETE CASCADE,
    expires_at    timestamptz NOT NULL,
    verified_at   timestamptz,
    consumed_at   timestamptz,
    client_name   text NOT NULL DEFAULT '',                  -- "<app> would like to connect"
    created_at    timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS oauth_device_codes_user_code_idx ON oauth_device_codes (user_code);

CREATE TABLE IF NOT EXISTS oauth_tokens (
    id            uuid PRIMARY KEY,
    user_id       uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash    text NOT NULL UNIQUE,
    token_prefix  text NOT NULL,                              -- first 8 chars of plaintext
    expires_at    timestamptz NOT NULL,
    revoked_at    timestamptz,
    last_used_at  timestamptz,
    created_at    timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS oauth_tokens_user_idx       ON oauth_tokens (user_id);
CREATE INDEX IF NOT EXISTS oauth_tokens_token_hash_idx ON oauth_tokens (token_hash);

-- ─── Invites ──────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS invites (
    id                uuid PRIMARY KEY,
    account_id        uuid NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    invitee_username  text NOT NULL,
    inviter_user_id   uuid NOT NULL REFERENCES users(id),
    status            text NOT NULL DEFAULT 'pending'
                        CHECK (status IN ('pending','accepted','declined','revoked')),
    created_at        timestamptz NOT NULL DEFAULT now(),
    decided_at        timestamptz
);

CREATE INDEX IF NOT EXISTS invites_account_idx                  ON invites (account_id);
CREATE INDEX IF NOT EXISTS invites_invitee_username_pending_idx ON invites (invitee_username) WHERE status = 'pending';
CREATE UNIQUE INDEX IF NOT EXISTS invites_one_pending_per_account_idx
    ON invites (account_id, invitee_username) WHERE status = 'pending';
