# Account-Pool Quality Monitor

## Purpose

This server-local timer measures the current Sub2API account pool every
15 minutes. Pool membership is read from the native account list and is
exactly `status=active && schedulable=true`. Each account receives one native
account-test request, in account-ID order, using one selected text model.

The result records only stability, first-token timing, configured multiplier,
and stable error codes. An explicit insufficient-balance response is recorded
as `balance_exhausted`; it never stops later accounts from being tested.

This is not a router, failover controller, model publisher, paid probe, or
Feishu action. It does not change Sub2API routes, groups, priorities,
account scheduling, prices, multipliers, balances, Keys, models, registration
settings, invitation-code settings, or Feishu. The account-test endpoint is
the only executor and is started by the systemd task, never by an ad hoc
operator command or an LLM request.

## Runtime Boundary

The service runs as `root` for host orchestration and starts one short-lived container on the
existing `sub2api_default` network. The worker is UID/GID `10002:10002`, has
a read-only root filesystem, no Linux capabilities, no-new-privileges, a
16 MiB noexec temporary filesystem, a 64 PID limit, 128 MiB memory limit,
and 0.25 CPU.

The only secret mount is the existing restricted Sub2API Admin-Key file. It
is never read into logs or evidence. The worker writes two mode-0600,
secret-free files under:

```text
/opt/sub2api/production/evidence/account-quality/
  account-quality-result.json
  account-quality-history.json
```

The history file retains at most 24 hours of timing samples. Relay-ops mounts
the containing evidence directory read-only, so atomic file replacement is
visible inside the container.

## Install

Install reviewed artifacts on the server without placing any credential in
the transfer command:

```text
ops/run-account-quality-monitor.sh
ops/account-quality-failure-signal.sh
ops/collect-account-quality-pulse.rb
infra/systemd/sub2api-account-quality-monitor.service
infra/systemd/sub2api-account-quality-monitor-failure.service
infra/systemd/sub2api-account-quality-monitor.timer
infra/systemd/account-quality-monitor.env.example
```

Use the fixed server paths and modes:

```sh
sudo install -d -m 0755 /opt/sub2api/production/ops/account-quality
sudo install -d -o 10002 -g 10002 -m 0700 /opt/sub2api/production/evidence/account-quality
sudo install -d -m 0755 /etc/sub2api
sudo install -m 0755 run-account-quality-monitor.sh /opt/sub2api/production/ops/account-quality/run-account-quality-monitor.sh
sudo install -m 0755 account-quality-failure-signal.sh /opt/sub2api/production/ops/account-quality/account-quality-failure-signal.sh
sudo install -m 0644 collect-account-quality-pulse.rb /opt/sub2api/production/ops/account-quality/collect-account-quality-pulse.rb
sudo install -m 0644 sub2api-account-quality-monitor.service /etc/systemd/system/sub2api-account-quality-monitor.service
sudo install -m 0644 sub2api-account-quality-monitor.timer /etc/systemd/system/sub2api-account-quality-monitor.timer
sudo install -m 0600 account-quality-monitor.env.example /etc/sub2api/account-quality-monitor.env
sudo systemctl daemon-reload
sudo systemd-analyze verify /etc/systemd/system/sub2api-account-quality-monitor.service /etc/systemd/system/sub2api-account-quality-monitor.timer
sudo systemctl enable --now sub2api-account-quality-monitor.timer
```

The existing native alert-events projection and relay-ops/Feishu path remain
the only failure-signal integration. The controlled `203/EXEC` delivery drill
is explicitly waived for this release by the user; receiver health is not
delivery evidence and no receipt is claimed. The missing drill remains an
unverified residual operational risk for a later window.

The environment file contains only paths, the image reference, and the
Docker network. Do not add any credential value, upstream URL, model name, or
account name. After enablement, wait for the timer to produce the first pulse;
do not invoke the native account-test endpoint manually.

## Inspect

The following inspection is read-only and does not trigger an account test:

```sh
systemctl status --no-pager sub2api-account-quality-monitor.timer
systemctl status --no-pager sub2api-account-quality-monitor.service
sudo journalctl -u sub2api-account-quality-monitor.service --since "1 hour ago" --no-pager
sudo sha256sum /opt/sub2api/production/evidence/account-quality/account-quality-result.json
```

Inspect `/ops` only with an authenticated Sub2API administrator session.
Anonymous and non-administrator requests remain 404. The page is read-only;
there is no account-test, route, or configuration control.

## Disable

Disable the schedule without changing the last valid evidence, account state,
or any route:

```sh
sudo systemctl disable --now sub2api-account-quality-monitor.timer
sudo systemctl stop sub2api-account-quality-monitor.service
```

Do not delete evidence to force a rerun. Do not use this task for
`upstream-benchmark`, model synchronization, capacity testing, provider
probing, model publication, native registration administration, or Feishu
enabled commands.
