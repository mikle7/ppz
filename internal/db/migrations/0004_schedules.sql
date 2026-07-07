-- Scheduled sends (docs/specs/schedule.md).
--
-- One row per live schedule. The row carries everything the server-
-- side firing loop needs to publish without any client involvement:
-- the resolved target (manifold / source_handle / pipe — source_handle
-- '' = uncollared), the payload, and the sender to stamp on the fired
-- envelope. Fired one-offs and removed schedules DELETE their row —
-- `schedule ls` shows live schedules only, no tombstones.
--
-- kind/spec/tz describe the shape:
--   at    — spec is the fire instant, RFC3339 with the creator's
--           offset preserved (rendered as typed in `schedule ls`)
--   every — spec is a Go duration; created_at anchors the interval
--           grid (fires at created_at + n·every)
--   cron  — spec is a 5-field cron expression; tz is the IANA zone
--           its wall-clock times resolve in
--
-- Idempotent (IF NOT EXISTS) to match the runner's re-apply-on-boot
-- behaviour.

CREATE TABLE IF NOT EXISTS schedules (
    id                  uuid PRIMARY KEY,
    account_id          uuid NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    manifold            text NOT NULL DEFAULT '',
    source_handle       text NOT NULL DEFAULT '',
    pipe                text NOT NULL,
    payload             text NOT NULL,
    sender              text NOT NULL DEFAULT '',
    kind                text NOT NULL,
    spec                text NOT NULL,
    tz                  text NOT NULL DEFAULT '',
    next_fire_at        timestamptz NOT NULL,
    last_fired_at       timestamptz,
    created_by_user_id  uuid NOT NULL REFERENCES users(id),
    created_at          timestamptz NOT NULL DEFAULT now(),
    CHECK (kind IN ('at', 'every', 'cron'))
);

-- The firing loop polls `next_fire_at <= now()` every tick.
CREATE INDEX IF NOT EXISTS schedules_next_fire_at_idx ON schedules (next_fire_at);

-- `schedule ls` lists per account, soonest first.
CREATE INDEX IF NOT EXISTS schedules_account_next_idx ON schedules (account_id, next_fire_at);
