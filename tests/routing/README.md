# tests/routing/

End-to-end tests for the path-based routing layer between Caddy and
ppz-server.

In production, Caddy fronts `pipescloud.io` and:
- Serves the **pipescloud marketing static site** (from
  `/srv/pipescloud-site/`) for explicit marketing paths (`/`, `/about`,
  `/pricing`, `/legal/*`).
- **Reverse-proxies everything else** to `ppz-server:8080`.

These scenarios exercise that boundary against a compose stack that
mirrors the prod topology — i.e. tests hit Caddy at `$PPZ_PUBLIC_URL`
(default `http://caddy:80`), not ppz-server directly.

## Pre-reqs (currently unmet — these tests are RED on purpose)

For the scenarios to go green, the compose stack must include a
`caddy` service that:
- Listens on a port reachable from the test-runner as
  `http://caddy:80`.
- Serves the static site from `/srv/pipescloud-site/`.
- Proxies non-marketing paths to `ppz-server:8080`.

The static site (`site/dist/{index,about,pricing}.html` plus
`legal/{terms,privacy}.html`) must exist with at least a DOCTYPE +
"pipescloud" brand marker per page.

## Convention

- One scenario per routing rule we care about.
- `run.sh` curls a single path, prints `key=value` lines.
- `expected.txt` is the exact normalized output.
- Scenarios that distinguish "static" vs "ppz-server" responses do so
  via DOCTYPE presence, body markers, status code, or response shape
  — not by parsing implementation-specific HTML.
