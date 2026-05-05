-- Phase 4 — invites: org-level invitations targeting a GitHub username
-- (matched against users.username). An invite is created in the
-- 'pending' state and transitions to one of: accepted (invitee
-- accepted, joined as non-owner member), declined (invitee refused),
-- revoked (inviter cancelled). Per-org partial unique index keeps at
-- most one PENDING invite per (org, username) — declined/revoked
-- rows can coexist so re-inviting after a refusal is allowed.

CREATE TABLE IF NOT EXISTS invites (
    id                  uuid PRIMARY KEY,
    organisation_id     uuid NOT NULL REFERENCES organisations(id) ON DELETE CASCADE,
    invitee_username    text NOT NULL,
    inviter_user_id     uuid NOT NULL REFERENCES users(id),
    status              text NOT NULL DEFAULT 'pending'
                          CHECK (status IN ('pending','accepted','declined','revoked')),
    created_at          timestamptz NOT NULL DEFAULT now(),
    decided_at          timestamptz
);

CREATE INDEX IF NOT EXISTS invites_invitee_username_pending_idx
    ON invites (invitee_username) WHERE status = 'pending';

CREATE UNIQUE INDEX IF NOT EXISTS invites_one_pending_per_org_idx
    ON invites (organisation_id, invitee_username) WHERE status = 'pending';

CREATE INDEX IF NOT EXISTS invites_org_idx
    ON invites (organisation_id);
