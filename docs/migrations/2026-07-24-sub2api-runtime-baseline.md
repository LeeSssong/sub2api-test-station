# Local Sub2API Runtime Baseline

Captured from the Mac `colima` Docker context at `2026-07-24T14:43:37Z`.
This is a local development deployment, not the production server. The
`/Users/...` paths and Compose project `sub2api-deploy` must never be used as
production release inputs. Production is reached through `ssh sub2api-prod`,
uses project `sub2api`, and lives at `/opt/sub2api/production`.

## Capture Command

```bash
EXPECTED_SUB2API_DATA='/Users/gongtengxinwen/Documents/Codex/2026-07-11/readme-cn-md-https-github-com/sub2api-deploy/data' \
EXPECTED_POSTGRES_DATA='/Users/gongtengxinwen/Documents/Codex/2026-07-11/readme-cn-md-https-github-com/sub2api-deploy/postgres_data' \
EXPECTED_REDIS_DATA='/Users/gongtengxinwen/Documents/Codex/2026-07-11/readme-cn-md-https-github-com/sub2api-deploy/redis_data' \
ops/capture-sub2api-runtime-baseline.sh > /tmp/sub2api-runtime-baseline.json
```

The captured JSON SHA-256 is
`3bfd854f2ca9ab81819fec6ac533278b7010f3b7ab3f6227c5abaeeb2424dd99`.
The collector emits inspected identity, mount, and network metadata only; it does
not emit container environment values.

## Runtime Identity

| Field | Captured value |
| --- | --- |
| Compose project | `sub2api-deploy` |
| Compose file | `/Users/gongtengxinwen/Documents/Codex/2026-07-11/readme-cn-md-https-github-com/sub2api-deploy/docker-compose.yml` |
| Compose working directory | `/Users/gongtengxinwen/Documents/Codex/2026-07-11/readme-cn-md-https-github-com/sub2api-deploy` |
| Current custom image/tag | `xingqiao-sub2api:v0.1.164-contact-v1` |
| Current custom image ID | `sha256:939e6f88068e82fd65f212bcc7b28b9ef2a9af27b8cce64e0b819a8b65fc3220` |
| Rollback tag | `xingqiao-sub2api:v0.1.164-contact-v1` |
| Network | `sub2api-deploy_sub2api-network` |

## Authoritative Storage

| Service | Source | Destination | Writable |
| --- | --- | --- | --- |
| Sub2API | `/Users/gongtengxinwen/Documents/Codex/2026-07-11/readme-cn-md-https-github-com/sub2api-deploy/data` | `/app/data` | yes |
| PostgreSQL | `/Users/gongtengxinwen/Documents/Codex/2026-07-11/readme-cn-md-https-github-com/sub2api-deploy/postgres_data` | `/var/lib/postgresql/data` | yes |
| Redis | `/Users/gongtengxinwen/Documents/Codex/2026-07-11/readme-cn-md-https-github-com/sub2api-deploy/redis_data` | `/data` | yes |

PostgreSQL also has an anonymous mount at `/var/lib/postgresql`; it is not the
authoritative database storage location. The collector requires the bind at
`/var/lib/postgresql/data`.
