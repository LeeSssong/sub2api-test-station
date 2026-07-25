# Model Release Read-Only Monitor

> Superseded for active operation on 2026-07-23 by
> [Account-Pool Quality Monitor](account-quality-monitor.md). This runbook,
> timer, source, and evidence remain historical records and must not be
> deleted during replacement.

## Purpose

This server-local systemd timer refreshes the secret-free model-release
snapshot and readiness result consumed by the hidden administrator /ops page.
It runs native model-directory discovery only. It does not run a benchmark,
generate a model response, test SSE, promote a model, change routing, mutate
prices or balances, change registration or invitation-code settings, or send a
Feishu action.

## Runtime Boundary

The systemd service runs as ubuntu, which already has Docker access. It starts
one short-lived container from the existing relay-ops Ruby image on the
existing sub2api_default network. The container runs as UID/GID 10002:10002 so
it can read the existing Admin-Key mount and write the existing restricted
model-release evidence directory. No port is published and no key is copied.

The service writes only these already approved secret-free artifacts:

    /opt/sub2api/production/evidence/model-release-20260722/model-release-snapshot.json
    /opt/sub2api/production/evidence/model-release-20260722/model-release-result.json

The evaluator atomically replaces the result only after successful collection.
A failure leaves the prior result in place; relay-ops marks stale evidence
fail-closed.

## Install

From the reviewed workspace, transfer these files to a server-local staging
directory without placing any secret in the transfer command:

    ops/run-model-release-monitor.sh
    ops/collect-model-release-snapshot.rb
    ops/evaluate-model-release-readiness.rb
    ops/model-release-policy.rb
    config/operations/model-release-policy-v1.yaml
    infra/systemd/sub2api-model-release-monitor.service
    infra/systemd/sub2api-model-release-monitor.timer
    infra/systemd/model-release-monitor.env.example

On the server, install them with these modes:

    sudo install -d -m 0755 /opt/sub2api/production/ops/model-release
    sudo install -d -m 0755 /etc/sub2api
    sudo install -m 0755 run-model-release-monitor.sh /opt/sub2api/production/ops/model-release/run-model-release-monitor.sh
    sudo install -m 0644 collect-model-release-snapshot.rb /opt/sub2api/production/ops/model-release/collect-model-release-snapshot.rb
    sudo install -m 0644 evaluate-model-release-readiness.rb /opt/sub2api/production/ops/model-release/evaluate-model-release-readiness.rb
    sudo install -m 0644 model-release-policy.rb /opt/sub2api/production/ops/model-release/model-release-policy.rb
    sudo install -m 0644 model-release-policy-v1.yaml /opt/sub2api/production/ops/model-release/model-release-policy-v1.yaml
    sudo install -m 0644 sub2api-model-release-monitor.service /etc/systemd/system/sub2api-model-release-monitor.service
    sudo install -m 0644 sub2api-model-release-monitor.timer /etc/systemd/system/sub2api-model-release-monitor.timer
    sudo install -m 0600 model-release-monitor.env.example /etc/sub2api/model-release-monitor.env
    sudo systemctl daemon-reload
    sudo systemd-analyze verify /etc/systemd/system/sub2api-model-release-monitor.service /etc/systemd/system/sub2api-model-release-monitor.timer

The environment file contains only a source directory, Admin-Key path, evidence
directory, Docker image name, and Docker network. Do not add the key value, a
provider URL, or an upstream credential.

Start exactly one manual read-only run before enabling the schedule:

    sudo systemctl start sub2api-model-release-monitor.service
    sudo systemctl status --no-pager sub2api-model-release-monitor.service
    sudo journalctl -u sub2api-model-release-monitor.service -n 20 --no-pager
    sudo systemctl enable --now sub2api-model-release-monitor.timer
    systemctl list-timers --all --no-pager | grep sub2api-model-release-monitor

A successful journal has only the monitor state plus the existing secret-free
collector/evaluator summaries. It does not require an upstream generation
request.

## Inspect

    systemctl status --no-pager sub2api-model-release-monitor.timer
    systemctl status --no-pager sub2api-model-release-monitor.service
    sudo journalctl -u sub2api-model-release-monitor.service --since "1 hour ago" --no-pager
    sudo sha256sum /opt/sub2api/production/evidence/model-release-20260722/model-release-result.json

Use /ops with an authenticated Sub2API administrator session to inspect the
current result. Anonymous and non-administrator requests remain 404.

## Disable

Disable the schedule without changing the last valid result or Sub2API state:

    sudo systemctl disable --now sub2api-model-release-monitor.timer
    sudo systemctl stop sub2api-model-release-monitor.service

Do not use this monitor to run upstream-benchmark-v2.rb, a promoter, route
control, or a Feishu enabled command. Compatibility/SSE qualification,
financial evidence, natural quality evidence, controlled model promotion, and
native registration administration remain separate approved activities.
