# ppz

> Pipes for agents — private, instant, durable interprocess and internet
> communications.

ppz is an open-source tool for streaming pipes between agents,
processes, and machines. It pairs a CLI + daemon on the user's
side with a server that runs NATS / JetStream as the transport.

Hosted ppz is available at [pipescloud.io](https://pipescloud.io).
It's built on this Apache-2.0 core and adds proprietary hosted-only
features (security, scalability, billing, team management, ops
tooling). The OSS code here is fully self-hostable.

## Install

**Quick (linux / macOS, amd64 / arm64):**

```bash
curl -fsSL https://raw.githubusercontent.com/pipescloud/ppz/main/install.sh | bash
```

Detects your OS + arch, downloads the matching pre-built tarball
from the latest [GitHub Release](https://github.com/pipescloud/ppz/releases),
verifies the sha256, and drops the binaries into `~/.local/bin`. No
Go toolchain required. Pin a specific version with `PPZ_VERSION=v0.17.0`
or override the install dir with `PPZ_INSTALL_DIR=/usr/local/bin`.

**From source (requires Go 1.22+):**

```bash
git clone https://github.com/pipescloud/ppz
cd ppz
make build
# binaries land in ./bin
```

## What's in the box

| Binary | Purpose |
|---|---|
| `ppz`              | The user-facing CLI (`ppz source create`, `ppz broadcast`, `ppz read`, `ppz terminal …`). |
| `ppz-server`       | Hosts the org/source/pipe state and embeds a NATS server. Self-hostable; pipescloud.io runs one. |
| `ppz-desktop`      | Local web GUI for browsing pipes. |
| `ppz-seed`         | Bootstraps a server with seed data (test / dev only). |
| `ppz-natsbootstrap`| Generates the NATS NSC chain (operator + system account). |

## Docs

- [`docs/AUTH-V2.md`](docs/AUTH-V2.md) — auth design (GitHub OAuth + per-org NATS account JWTs)
- [`docs/WIRE.md`](docs/WIRE.md) — wire protocol reference (subjects, error codes, retention semantics)
- [`docs/ERRORS.md`](docs/ERRORS.md) — error code catalogue
- [`docs/SECRETS.md`](docs/SECRETS.md) — secret model

A self-hosting / deployment guide is on the way — track issues for
progress.

## Tests

```bash
make test           # Go unit tests
make e2e            # full Docker-Compose integration suite
make e2e-filter F='broadcast/*'   # subset
```

## Releasing

Tags are minted automatically on each merge to `main` from
[Conventional Commits](https://www.conventionalcommits.org/) in the PR
title / commit subjects:

| Prefix                           | Bump   |
|---                               |---     |
| `feat:` / `feat(scope):`         | minor  |
| `fix:` / `fix(scope):`           | patch  |
| `feat!:` / `<type>!:` / `BREAKING CHANGE:` in body | major |
| `chore:`, `docs:`, `refactor:`, `test:`, `ci:`, etc. | no tag |

The highest bump level seen in the new commit range wins. Tagging is
just tagging — it does **not** publish binaries. To cut a distribution,
manually dispatch the **Release** workflow (Actions → Release → Run
workflow) and pick the tag. That's when goreleaser builds the matrix
and attaches the archives to the GitHub Release.

Manual tagging via `make tag {patch|minor|major}` is still available
for special cases.

## License

Apache 2.0. See [`LICENSE`](LICENSE).
