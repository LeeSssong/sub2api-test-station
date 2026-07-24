# Host-Controlled One-Click Sub2API Update: Task 4 Verification

**Date:** 2026-07-24
**Scope:** systemd packaging and the production-host installer only.

The production image-release runbook was intentionally not changed. Task 2 also
owns that file; the runbook merge points are recorded at the end of this report.

## Implemented

- Added `sub2api-updater.service` as a root-owned, restartable service with:
  - `/run/sub2api-updater` managed as `root:root`, mode `0755`; the Caddy
    container reads the socket through its existing read-only bind mount and
    does not require a host `caddy` group;
  - `/var/lib/sub2api-updater` managed as a mode `0700` state directory;
  - `UMask=0077`, `NoNewPrivileges`, `ProtectSystem=strict`, private temporary
    storage, restricted address families, and explicit write paths;
  - an immutable production executor path and the Unix socket/state paths from
    the environment file.
- Added a secret-free environment example containing only the official API,
  origin, GitHub release endpoint, socket, state, and executor paths.
- Added `ops/install-sub2api-updater.sh`. Before any systemd or filesystem
  mutation it requires Linux, the default Docker context, root, the exact
  `/opt/sub2api/production` checkout, the executor, UI/Caddy/Compose inputs,
  and `systemd-analyze` validation. It then builds a static
  `linux/amd64` binary, installs the binary/executor/env/unit with the required
  modes, reloads systemd, and enables the service.
- Added a shell contract test covering unit hardening, paths, secret-free
  packaging, cross-compilation, file modes, production-host refusal, and the
  optional local `systemd-analyze` check.

## TDD Evidence

The test was written before the package implementation.

RED:

```text
$ bash tests/operations/install_sub2api_updater_test.sh
FAIL: missing infra/systemd/sub2api-updater.service
exit_code=1
```

GREEN:

```text
$ bash tests/operations/install_sub2api_updater_test.sh
SKIP: systemd-analyze is unavailable; unit verification not run
PASS: Sub2API updater packaging contracts
exit_code=0
```

## Verification Run

Passed:

```text
bash -n ops/install-sub2api-updater.sh tests/operations/install_sub2api_updater_test.sh
bash tests/operations/install_sub2api_updater_test.sh
```

The test ran on macOS and proved that the installer refuses a non-Linux host
before creating its mutation marker. No production path was written.

Not run because the dependencies are absent:

- `systemd-analyze`: unavailable, so unit verification requires a Linux host;
- `shellcheck`: unavailable;
- `go`: unavailable, so the installer build path was not executed.

Docker was present in `PATH` but was intentionally not invoked. No production
installation, SSH session, Docker command, `systemctl` command, or service
activation was performed.

## Runbook Merge Points

The main branch owner should add these points to
`docs/runbooks/sub2api-official-image-release.md` alongside Task 2's executor
instructions:

1. Run the installer only through the production checkout at
   `/opt/sub2api/production`, on Linux, as root, with the default Docker
   context and no `DOCKER_HOST`. The installer requires the already-reviewed
   host executor at `ops/update-sub2api-host.sh` and the Task 3 UI/Caddy/Compose
   inputs; it does not require a host `caddy` group.
2. Install the package with `./ops/install-sub2api-updater.sh`. It cross-builds
   `linux/amd64`, installs the binary at
   `/usr/local/libexec/sub2api-updater`, the env file at
   `/etc/sub2api/sub2api-updater.env` mode `0600`, and the unit at
   `/etc/systemd/system/sub2api-updater.service` mode `0644`.
3. The service owns `/run/sub2api-updater/updater.sock` for the Caddy bind and
   persists state at `/var/lib/sub2api-updater/state.json`. Check the unit with
   `systemd-analyze verify` and inspect status with
   `systemctl status sub2api-updater.service`; do not print the env file.
4. Package installation does not submit an update and does not recreate any
   container. Application changes still use the Task 2 executor, which
   recreates only `sub2api` and owns backup/health/rollback behavior.
5. For package rollback, stop and disable the unit first, preserve the state
   and release evidence, then restore the previous unit/binary/env from the
   timestamped host backup. Do not remove update evidence or invoke
   `docker compose down` as part of package rollback.
