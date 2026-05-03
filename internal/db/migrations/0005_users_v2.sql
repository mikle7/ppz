-- Users v2: extend the users table with the columns GitHub OAuth needs.
-- Idempotent — every statement is guarded so the migration runner can
-- re-apply on every boot.
--
-- v1 introduced users + organisation_members + owner_user_id.
-- v2 adds the GitHub-specific columns. All nullable so existing
-- mode='internal' rows (the seeded `unauthenticated`, `foo`, `bar`)
-- continue to satisfy the schema without backfill.

ALTER TABLE users ADD COLUMN IF NOT EXISTS github_id  BIGINT;
ALTER TABLE users ADD COLUMN IF NOT EXISTS avatar_url TEXT;

-- One GitHub identity ↔ one user row. Allowing NULL because internal
-- users have no github_id; the partial-unique pattern is equivalent
-- to "unique among non-null values".
CREATE UNIQUE INDEX IF NOT EXISTS users_github_id_unique
    ON users (github_id)
    WHERE github_id IS NOT NULL;
