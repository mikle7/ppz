-- ─── OAuth org selection (multi-org login) ────────────────────────────
--
-- The device flow now lets a multi-org user choose which org the CLI
-- session authorizes into. Two pieces of state:
--
--   oauth_device_codes.account_id  — the org picked on the verify page,
--       carried through to the token reply so the CLI can pass it to
--       /auth/exchange (which mints the NATS JWT in that org).
--
--   users.last_selected_account_id — remembers the org a user last
--       authorized into, so the verify-page dropdown defaults to it on
--       the next login (falling back to their default org). ON DELETE
--       SET NULL so deleting an org just drops the preference.

ALTER TABLE oauth_device_codes
    ADD COLUMN IF NOT EXISTS account_id uuid REFERENCES accounts(id) ON DELETE SET NULL;

ALTER TABLE users
    ADD COLUMN IF NOT EXISTS last_selected_account_id uuid REFERENCES accounts(id) ON DELETE SET NULL;
