-- Auth V2 §Phase 3.5 — per-org NATS account isolation. Each org
-- gets its own NATS account (Operator-signed) so cross-tenant
-- $JS.API access is impossible (closed in Phase 3 only at the
-- subject layer; control-plane was still shared).
--
-- Lazily populated: on first /auth/exchange touch (or org create)
-- the server mints a fresh keypair, signs an Account JWT with the
-- Operator key, registers the JWT with the in-memory resolver,
-- and writes these three columns. Idempotent re-touch is a no-op.

ALTER TABLE organisations
    ADD COLUMN IF NOT EXISTS nats_account_pub          text,
    ADD COLUMN IF NOT EXISTS nats_account_jwt          text,
    ADD COLUMN IF NOT EXISTS nats_account_signing_seed text;

CREATE INDEX IF NOT EXISTS organisations_nats_account_pub_idx
    ON organisations (nats_account_pub);
