-- Auth V2 §Phase 3.5 — per-account NATS account isolation. Each ppz
-- account gets its own NATS account (Operator-signed) so cross-tenant
-- $JS.API access is impossible (closed in Phase 3 only at the subject
-- layer; control-plane was still shared).
--
-- Lazily populated: on first /auth/exchange touch (or account create)
-- the server mints a fresh keypair, signs an Account JWT with the
-- Operator key, registers the JWT with the in-memory resolver, and
-- writes these three columns. Idempotent re-touch is a no-op.
--
-- "Account" here means two things at once: the ppz-side tenancy row
-- (renamed from "organisation" pre-launch) and the NATS-side Account
-- credential. They map 1:1 — one ppz account owns exactly one NATS
-- account.

ALTER TABLE accounts
    ADD COLUMN IF NOT EXISTS nats_account_pub          text,
    ADD COLUMN IF NOT EXISTS nats_account_jwt          text,
    ADD COLUMN IF NOT EXISTS nats_account_signing_seed text;

CREATE INDEX IF NOT EXISTS accounts_nats_account_pub_idx
    ON accounts (nats_account_pub);
