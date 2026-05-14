-- 0002_manifold.sql — Phase 1.5 data model
--
-- Adds the structural primitives Phase 1's CLI surface implied:
--   1. `manifold` column on sources and pipes — hierarchical-grouping
--      segment. Empty string '' represents the root namespace (a real
--      value, never NULL). pipescloud and multi-team self-hosters use
--      richer values; OSS-default deploys leave everything at ''.
--   2. `pipes.source_id` becomes nullable — uncollared (sourceless)
--      pipes have no source. NULL is meaningful here: absence of a
--      referenced row, as opposed to the manifold case where '' is a
--      real path value.
--   3. `pipes.account_id` — denormalised from `source.account_id` for
--      collared pipes, explicit for uncollared. Needed because the
--      account is no longer reachable via source_id when source_id IS
--      NULL.
--
-- See pipes-internal/docs/PHASE-1.5-IMPLEMENTATION-PLAN.md.
--
-- Idempotent — every statement uses IF NOT EXISTS / IF EXISTS so the
-- migration runner can re-apply on every boot. Pre-launch we still
-- nuke+redeploy via the Reset Database action; idempotency is for
-- migration-runner robustness, not user-visible upgrade paths.

ALTER TABLE sources ADD COLUMN IF NOT EXISTS manifold text NOT NULL DEFAULT '';
ALTER TABLE pipes ADD COLUMN IF NOT EXISTS manifold text NOT NULL DEFAULT '';

ALTER TABLE pipes ALTER COLUMN source_id DROP NOT NULL;

-- pipes.account_id — add nullable, backfill from source.account_id,
-- then SET NOT NULL. The backfill is a no-op on a fresh DB (0001 +
-- 0002 against an empty schema produce no pipe rows) but lets the
-- migration handle any pre-existing v0.30 data cleanly.
ALTER TABLE pipes ADD COLUMN IF NOT EXISTS account_id uuid;
UPDATE pipes
   SET account_id = sources.account_id
  FROM sources
 WHERE pipes.account_id IS NULL
   AND pipes.source_id IS NOT NULL
   AND sources.id = pipes.source_id;
ALTER TABLE pipes ALTER COLUMN account_id SET NOT NULL;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.table_constraints
        WHERE table_name = 'pipes' AND constraint_name = 'pipes_account_id_fkey'
    ) THEN
        ALTER TABLE pipes
            ADD CONSTRAINT pipes_account_id_fkey
            FOREIGN KEY (account_id) REFERENCES accounts(id) ON DELETE CASCADE;
    END IF;
END $$;

-- UNIQUE constraints — replace the pre-Phase-1.5 shapes:
--
--   sources: was UNIQUE (account_id, handle); now UNIQUE
--   (account_id, manifold, handle) — same handle can exist in
--   different manifolds within the same account.
--
--   pipes: was UNIQUE (source_id, name); now two partial UNIQUE
--   indexes — one for collared (source_id IS NOT NULL), one for
--   uncollared (source_id IS NULL). A single UNIQUE over a nullable
--   column doesn't constrain NULL rows in standard SQL, hence the
--   split.

ALTER TABLE sources DROP CONSTRAINT IF EXISTS sources_account_id_handle_key;
CREATE UNIQUE INDEX IF NOT EXISTS sources_account_manifold_handle_unique
    ON sources (account_id, manifold, handle);

ALTER TABLE pipes DROP CONSTRAINT IF EXISTS pipes_source_id_name_key;
CREATE UNIQUE INDEX IF NOT EXISTS pipes_collared_unique
    ON pipes (account_id, manifold, source_id, name)
    WHERE source_id IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS pipes_uncollared_unique
    ON pipes (account_id, manifold, name)
    WHERE source_id IS NULL;

CREATE INDEX IF NOT EXISTS pipes_account_manifold_idx ON pipes (account_id, manifold);
