# Secrets

Production secrets are owned by Pulumi. Almost everything is generated
on first `pulumi up` and stored in stack state + AWS Secrets Manager.

## What you set manually

Just one — your GitHub OAuth app's client secret:

```bash
cd infra
pulumi config set --secret ppz-infra:github_client_secret <value>
```

Get the value from `github.com/settings/applications/<your-app>`.

## What Pulumi generates

Everything else, on first apply:

| Secret | Provisioned by |
|---|---|
| `PPZ_DB_URL` | RDS password (`random.RandomPassword`) + RDS endpoint, baked into a DSN |
| `PPZ_SESSION_KEY` | `random.RandomPassword` |
| NATS NSC chain (Operator JWT, Account JWT, System Account JWT, Account signing seed) | `random.RandomBytes` × 4 → derived via `nkeys` + `jwt/v2` in `infra/natsauth.go` |

Each lands in its own AWS Secrets Manager entry under `ppz/{env}/...`,
EC2 user_data fetches them at boot via the instance role.

## Rotation

```bash
# Rotate the session key (every signed-in user gets logged out):
pulumi up --replace 'urn:pulumi:production::ppz-infra::random:index/randomPassword:RandomPassword::ppz-production-session-key'

# Rotate the NATS account signing seed (existing daemons re-login on next refresh):
pulumi up --replace 'urn:pulumi:production::ppz-infra::random:index/randomBytes:RandomBytes::ppz-production-nats-signing-entropy'

# Rotate the GitHub client secret (after generating a new one in github.com):
pulumi config set --secret ppz-infra:github_client_secret <new-value>
pulumi up
```

## Local development

`make dev` runs the compose stack with ephemeral NATS credentials
minted by `compose/server-entrypoint.sh` on each `make e2e-up`. The
only values you supply via `.env.local` are the GitHub OAuth ones:

```
PPZ_GITHUB_CLIENT_ID=...
PPZ_GITHUB_CLIENT_SECRET=...
PPZ_SESSION_KEY=...           # any 32+ chars; rotate by changing
```
