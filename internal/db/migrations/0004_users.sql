-- Users v1: introduce a `users` table, give every account an owner, and
-- add `account_members` for the many-to-many relationship. Idempotent —
-- the migration runner re-runs on every boot so every statement is
-- guarded.
--
-- The seeded "unauthenticated" user (mode=internal) becomes the owner of
-- any account that doesn't yet have one, including alpha and beta from
-- the e2e seed. v2 (GitHub OAuth) layers real users on top of the same
-- schema; the unauthenticated user just stays as a fallback for tests /
-- non-OAuth flows.

CREATE TABLE IF NOT EXISTS users (
    id          uuid PRIMARY KEY,
    username    text NOT NULL UNIQUE,
    email       text NOT NULL,
    mode        text NOT NULL,            -- 'github' or 'internal'
    created_at  timestamptz NOT NULL DEFAULT now(),
    CHECK (mode IN ('github', 'internal'))
);

-- Seed the placeholder user that owns un-attributed / e2e accounts.
-- Fixed UUID so the seed is deterministic across rebuilds.
INSERT INTO users (id, username, email, mode)
VALUES ('00000000-0000-0000-0000-000000000001',
        'unauthenticated',
        'unauthenticated@local',
        'internal')
ON CONFLICT (username) DO NOTHING;

-- Accounts get an owner. NULL during the migration window;
-- backfilled to the unauthenticated user, then made NOT NULL.
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS owner_user_id uuid REFERENCES users(id);

UPDATE accounts
   SET owner_user_id = (SELECT id FROM users WHERE username = 'unauthenticated')
 WHERE owner_user_id IS NULL;

ALTER TABLE accounts ALTER COLUMN owner_user_id SET NOT NULL;

CREATE TABLE IF NOT EXISTS account_members (
    account_id  uuid NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    user_id     uuid NOT NULL REFERENCES users(id)    ON DELETE CASCADE,
    added_at    timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (account_id, user_id)
);

CREATE INDEX IF NOT EXISTS account_members_user_idx
    ON account_members (user_id);
