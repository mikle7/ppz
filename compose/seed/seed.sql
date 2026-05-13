-- Reference seed for two accounts. The actual UUID + key insertion is
-- performed by the Go seeder (cmd/ppz-seed) which can produce argon2id hashes
-- and write the plaintext keys + account IDs into the /seed shared volume.
--
-- This file is kept as documentation of the minimum seeded fixtures and is
-- runnable for manual db inspection during development.

INSERT INTO accounts (id, name, created_at) VALUES
  ('00000000-0000-0000-0000-0000000a1pha', 'alpha', now()),
  ('00000000-0000-0000-0000-00000000beta', 'beta',  now())
ON CONFLICT DO NOTHING;
