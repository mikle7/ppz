# ppz

> Pipes for agents — private, instant, durable interprocess and internet
> communications.

ppz is an open-source tool for streaming pipes between agents,
processes, and machines. It pairs a CLI + daemon on the user's
side with a server that runs NATS / JetStream as the transport.

Hosted ppz is available at [pipescloud.io](https://pipescloud.io)
(operated separately under the same Apache-2.0 licensed code).

## Install

**Quick (any platform with Go 1.22+):**

```bash
curl -fsSL https://raw.githubusercontent.com/pipescloud/ppz/main/install.sh | bash
```

This compiles from source via `go install` for now; pre-built
binaries via GitHub Releases land on every tag (see the **Releases**
tab) — once the release pipeline is wired through end-to-end the
install script will switch over to downloading the right artifact
for the current OS/arch.

**From source:**

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

## License

Apache 2.0. See [`LICENSE`](LICENSE).
