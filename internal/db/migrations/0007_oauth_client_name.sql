-- Auth V2 Phase 2 follow-up: name the calling client on the verify
-- page (mirrors Claude Code's "<app name> would like to connect"
-- consent screen). The CLI passes a free-form `client_name` with
-- /oauth/device/code; the browser pulls it back via user_code lookup.
--
-- Defaults to '' so existing rows + clients that don't supply the
-- field render a generic message.
ALTER TABLE oauth_device_codes
    ADD COLUMN IF NOT EXISTS client_name text NOT NULL DEFAULT '';
