-- Auth V2 Phase 2: device flow + bearer-token storage.
-- Idempotent — every statement is guarded so the migration runner
-- can re-apply on every boot.

-- oauth_device_codes: pending device-flow sessions.
--   device_code: long random opaque token, polled by the CLI
--   user_code:   short readable code (e.g. ABCD-1234) typed/confirmed
--                by the user in the browser
--   user_id:     NULL until the user verifies in the browser
--   verified_at: NULL until verified; once set, the CLI's next poll
--                consumes the code and gets a bearer token
CREATE TABLE IF NOT EXISTS oauth_device_codes (
    device_code   text PRIMARY KEY,
    user_code     text NOT NULL UNIQUE,
    user_id       uuid REFERENCES users(id) ON DELETE CASCADE,
    expires_at    timestamptz NOT NULL,
    verified_at   timestamptz,
    consumed_at   timestamptz,
    created_at    timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS oauth_device_codes_user_code_idx
    ON oauth_device_codes (user_code);

-- oauth_tokens: bearer tokens issued by the device-flow exchange.
-- Format on the wire: ppz_oauth_<48 random alphanum>; stored hashed.
-- Coexists with api_keys (which prefix as ppz_live_<…>); the unified
-- requireAuth middleware looks both up.
CREATE TABLE IF NOT EXISTS oauth_tokens (
    id           uuid PRIMARY KEY,
    user_id      uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash   text NOT NULL UNIQUE,
    token_prefix text NOT NULL,           -- first 8 chars of plaintext, for display
    expires_at   timestamptz NOT NULL,
    revoked_at   timestamptz,
    created_at   timestamptz NOT NULL DEFAULT now(),
    last_used_at timestamptz
);

CREATE INDEX IF NOT EXISTS oauth_tokens_user_idx     ON oauth_tokens (user_id);
CREATE INDEX IF NOT EXISTS oauth_tokens_token_hash_idx ON oauth_tokens (token_hash);
