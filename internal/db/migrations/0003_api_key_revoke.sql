-- Soft-revoke for API keys. Lookups filter `revoked_at IS NULL`; the
-- row stays so audit / history queries still see it. Idempotent —
-- migration is safe to re-run.
ALTER TABLE api_keys ADD COLUMN IF NOT EXISTS revoked_at TIMESTAMPTZ NULL;
