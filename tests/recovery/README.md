# tests/recovery/

End-to-end scenarios that exercise state-recovery paths — situations
where ppz-server's runtime state diverges from postgres or NATS, and
the system must self-heal on next access.

Each scenario simulates a real production state we've observed (or
that operations could plausibly produce, e.g. key rotation, org
migration) and asserts the user-facing operations (broadcast, read,
list, etc.) recover automatically.

Compose tests can drive the divergence via dev-gated admin hooks
mounted under `/api/v1/admin/...` (gated on `DevLogin`, returning
404 in production).
