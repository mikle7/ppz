# Scheduled sends

Status: agreed 2026-07-07 — RED tests written, implementation pending.

`ppz send` gains three mutually exclusive switches that register a
**server-side durable schedule** instead of publishing immediately:

```
ppz send bob "standup in 5" --at "2026-07-08 09:55"     # one-off
ppz send room "heartbeat"   --every 15m                 # recurring, interval
ppz send team "weekly sync" --cron "0 10 * * MON"       # recurring, wall-clock
```

Management lives under a new noun:

```
ppz schedule ls [--json | --iso]
ppz schedule rm <id>
```

## Why server-side

- Daemon timers pause across OS sleep (see wake_watchdog.go) and the
  daemon isn't guaranteed to be running at fire time. "Durable" means
  "fires with the laptop lid closed" — only the server can offer that.
- JetStream has no native deliver-at-time; any delay mechanism reduces
  to "something with a durable timer publishes later".
- The server already holds each org's NATS account signing key and a
  `>`-scoped per-org connection (account_pool.go). Envelopes are not
  client-signed — `sender` is stamped by whoever publishes — so a
  server-side fire needs no client credentials and does not change the
  trust model.

## CLI surface

- Exactly one of `--at` / `--every` / `--cron`; combining any two is an
  error ("mutually exclusive"). `--request-ack` cannot combine with any
  of them (a scheduled fire has no live session to ack back to).
- `--at <when>` — one-off. Accepts RFC3339 (`2026-07-08T09:55:00Z`,
  with offset), `"YYYY-MM-DD HH:MM"` (device-local), or relative
  `+<goduration>` (`+5m`, `+2h30m`). A past instant is rejected:
  "--at is in the past".
- `--every <dur>` — Go duration, minimum 1s. First fire = creation
  instant + interval; subsequent fires stay on that anchor grid.
- `--cron "<expr>"` — standard 5-field cron
  (minute hour dom month dow), parsed with robfig/cron/v3 (parser
  only — the firing loop is ours). The device's IANA timezone is
  captured at creation ($TZ env, else /etc/localtime, else UTC),
  stored on the schedule, and shown in `schedule ls`.
- Creation prints `scheduled id=<id8> to=<path> next=<RFC3339>` on the
  same stream as `sent id=…`. `<id8>` is the short id used by
  `schedule rm`.
- Target resolution (bare-name `.inbox` sugar, uncollared fallback)
  is identical to a plain send; resolution happens at creation time
  and the resolved target is stored on the schedule.
- Sender: the creator's current handle at creation time, stored on the
  schedule and stamped on every fired envelope. Fired envelopes also
  carry `schedule_id` (id8) so receivers can distinguish scheduled
  messages.

## `schedule ls` output

Follows the `ppz ls` table conventions exactly (two-space gaps,
display-width padding, `-` for missing, header only when rows exist,
`--json`/`--iso` mutually exclusive):

```
ID        NAMESPACE  PIPE       SCHEDULE                      NEXT          LAST            PAYLOAD          CREATOR
a1b2c3d4  -          bob.inbox  at 2026-07-08T09:55:00+01:00  in 18 hours   -               standup in 5     jimmy
e5f6a7b8  -          alerts     every 15m                     in 4 minutes  11 minutes ago  heartbeat check  bot-a
```

- SCHEDULE cell: `at <RFC3339 as typed, creator's offset>`,
  `every <dur>`, `cron <expr> <IANA tz>`.
- NEXT uses future-relative time ("in 5 minutes"); LAST uses the
  existing past-relative form; `--iso` flips both to RFC3339 UTC.
- Rows sort by NEXT ascending (soonest first).
- PAYLOAD truncated at 60 chars in the table, full in `--json`.
- JSON rows (JSONL): `id`, `namespace`, `handle`, `pipe`,
  `schedule` (at|every|cron), `spec`, `tz`, `next_at`, `last_at`
  (null when never fired), `payload`, `creator`.
- Fired one-offs and removed schedules leave the table; no STATUS
  column.

## Wire & storage

- IPC verbs: `ScheduleCreate`, `ScheduleList`, `ScheduleRemove`
  (cliproto). CLI resolves `--at` to a concrete RFC3339 instant
  before IPC; the daemon resolves the target and forwards to REST.
- REST: `POST/GET /api/v1/schedules`, `DELETE /api/v1/schedules/{id}`,
  bearer-gated like the other /api/v1 routes.
- Postgres migration `0004_schedules.sql`: schedule rows keyed by
  account, carrying resolved target parts, payload, sender, kind,
  spec, tz, `next_fire_at`, `last_fired_at`, creator.
- Firing loop: a server ticker claims due rows with
  `SELECT … FOR UPDATE SKIP LOCKED` (multi-replica safe), ensures the
  pipe stream exists, publishes via the org's pooled NATS connection,
  then advances `next_fire_at` (recurring) or deletes the row
  (one-off).

## Missfire policy

If the server was down (or the loop stalled) past a fire time:

- one-off: fires once immediately, however late.
- recurring: fires only if overdue ≤ 60s (`MissfireGrace`); when
  further overdue, missed occurrences are dropped and `next_fire_at`
  advances to the first occurrence after now. No catch-up bursts.

## Out of scope (v1)

- Pause/resume, edit-in-place (`rm` + recreate instead).
- A STATUS column / fire-history forensics (future `schedule show`).
- Cryptographic sender attribution (unchanged from plain sends).
