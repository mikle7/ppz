-- User attribution for api_keys, sources, pipes (`ppz ls` HUMAN).
--
-- Every key, source, and pipe now points at the user who created it so
-- the CLI / GUI can render an attribution column (HUMAN). Idempotent —
-- the migration runner re-applies on every boot.
--
-- Backfill:
--   - api_keys.created_by_user_id  ← accounts.owner_user_id
--   - sources.created_by_user_id   ← accounts.owner_user_id
--   - pipes.created_by_user_id     ← sources.created_by_user_id (post-fill)
--
-- The owner is always known (account_members + owner_user_id are both
-- NOT NULL since users-v1), so backfill is total and we can flip the
-- new columns to NOT NULL in one migration.

ALTER TABLE api_keys ADD COLUMN IF NOT EXISTS created_by_user_id uuid REFERENCES users(id);
ALTER TABLE sources  ADD COLUMN IF NOT EXISTS created_by_user_id uuid REFERENCES users(id);
ALTER TABLE pipes    ADD COLUMN IF NOT EXISTS created_by_user_id uuid REFERENCES users(id);

UPDATE api_keys k
   SET created_by_user_id = a.owner_user_id
  FROM accounts a
 WHERE k.account_id = a.id
   AND k.created_by_user_id IS NULL;

UPDATE sources s
   SET created_by_user_id = a.owner_user_id
  FROM accounts a
 WHERE s.account_id = a.id
   AND s.created_by_user_id IS NULL;

UPDATE pipes p
   SET created_by_user_id = s.created_by_user_id
  FROM sources s
 WHERE p.source_id = s.id
   AND p.created_by_user_id IS NULL;

ALTER TABLE api_keys ALTER COLUMN created_by_user_id SET NOT NULL;
ALTER TABLE sources  ALTER COLUMN created_by_user_id SET NOT NULL;
ALTER TABLE pipes    ALTER COLUMN created_by_user_id SET NOT NULL;

CREATE INDEX IF NOT EXISTS api_keys_created_by_idx ON api_keys (created_by_user_id);
CREATE INDEX IF NOT EXISTS sources_created_by_idx  ON sources  (created_by_user_id);
CREATE INDEX IF NOT EXISTS pipes_created_by_idx    ON pipes    (created_by_user_id);
