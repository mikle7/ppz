-- 0003_password_hash.sql — Phase 2 auth model
--
-- Adds the nullable `password_hash` column on users. Backs the
-- `auth_mode=password` admin web UI login. Nullable because users
-- created under `auth_mode=none` or `auth_mode=oauth` have no
-- password — only password-mode users do.
--
-- See pipes-internal/docs/PHASE-2-IMPLEMENTATION-PLAN.md Cycle A.

ALTER TABLE users ADD COLUMN IF NOT EXISTS password_hash text;
