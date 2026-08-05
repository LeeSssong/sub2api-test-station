# Cloudflare Free Edge Migration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Move `api.xingqiaolab.top` to Cloudflare Free without changing the hostname or restarting production, retain it only when all gates pass, and otherwise restore same-hostname direct access.

**Architecture:** Mirror DNSPod into a non-authoritative Cloudflare Free zone with `api` DNS only. Switch authoritative nameservers only after record parity, wait for the zone and Universal SSL certificate to become active, then enable the existing `api` record as Proxied. Compare the affected Mac before and after, correlate real streams with production logs, and keep DNSPod plus the reachable origin as rollback layers.

**Tech Stack:** Cloudflare Free dashboard, Tencent Cloud/DNSPod dashboard, `dig`, `openssl`, `curl`, SSH, Docker, PostgreSQL `psql`, Git, Codex Desktop.

## Global Constraints

- Keep the endpoint exactly `api.xingqiaolab.top`.
- Use Cloudflare Free only; do not buy or enable Argo, China Network, Business, Workers, Tunnel, Load Balancing, or paid add-ons.
- Do not restart or recreate Caddy, Sub2API, PostgreSQL, Redis, worker, or relay-ops.
- Keep DNSPod records intact for at least 48 hours after cutover.
- Do not create API tokens or expose passwords, cookies, OTPs, API keys, TXT values, or `.env` contents.
- Keep the origin publicly reachable during this change.
- Keep `api` DNS only until the zone is Active and Universal SSL covers the hostname.
- Use Full (strict), minimum TLS 1.2 with TLS 1.3 enabled, WebSockets enabled, and no cache or interactive challenge for `/responses`, `/v1/*`, or `/api/*`.
- Treat 100 MB as the Cloudflare Free public request limit.
- Register `docs/project/project-progress.md` as “进行中” before external mutation.
- Mark “已完成” only after push, live deployment, and all production gates pass.
- Preserve unrelated untracked evidence and user worktree changes.

---

### Task 1: Register The Change And Capture Direct Baseline

**Files:**
- Modify: `docs/project/project-progress.md`
- Create: `docs/superpowers/reports/2026-08-05-cloudflare-free-edge-migration-production-verification.md`
- Reference: `docs/superpowers/specs/2026-08-05-cloudflare-free-edge-migration-design.md`

**Interfaces:**
- Consumes: Production SSH alias `sub2api-prod`, public DNS, and the approved design.
- Produces: A committed in-progress ledger entry and immutable direct DNS/TLS/HTTP/error/container baseline.

- [ ] **Step 1: Add the in-progress ledger entry with `apply_patch`**

Add under `## 当前最重要进行中事项`:

~~~markdown
13. **Cloudflare Free 公网入口迁移**：保持 `api.xingqiaolab.top` 和首尔源站不变，将权威 DNS 从 DNSPod 迁移到 Cloudflare Free；区域与边缘证书激活后才开启 `api` 橙色云，并以受影响 Mac 的 TLS 1.2/1.3、健康请求、真实 Codex 流式请求和站内日志作为保留门禁。**状态：进行中（已确认设计；正在采集直连基线，尚未修改权威 NS 或开启代理）**。
~~~

- [ ] **Step 2: Capture UTC time, DNS, and DNSSEC**

~~~bash
date -u '+captured_at_utc=%Y-%m-%dT%H:%M:%SZ'
dig +short NS xingqiaolab.top
dig +short A api.xingqiaolab.top
dig +short AAAA api.xingqiaolab.top
dig +short DS xingqiaolab.top
dig +short CAA xingqiaolab.top
~~~

Expected: NS includes `golf.dnspod.net.` and `train.dnspod.net.` before migration; A contains the origin. A non-empty DS blocks Task 3 until DNSSEC is reconciled.

- [ ] **Step 3: Capture 20 TLS 1.2 and 20 TLS 1.3 handshakes**

~~~bash
for version in tls1_2 tls1_3; do
  for attempt in $(seq 1 20); do
    if printf '' | openssl s_client -brief "-$version" -connect api.xingqiaolab.top:443 -servername api.xingqiaolab.top >/dev/null 2>&1; then
      printf '%s success\n' "$version"
    else
      printf '%s failure\n' "$version"
    fi
  done
done | sort | uniq -c
~~~

Expected: Record exact counts; the known-broken direct TLS 1.2 result is baseline evidence.

- [ ] **Step 4: Capture 20 direct health samples**

~~~bash
for attempt in $(seq 1 20); do
  curl -sS -o /dev/null --connect-timeout 5 --max-time 15 \
    -w 'code=%{http_code} remote=%{remote_ip} connect=%{time_connect} tls=%{time_appconnect} total=%{time_total}\n' \
    https://api.xingqiaolab.top/health || printf 'curl_failed=1\n'
done
~~~

Expected: Record success count and median of successful `total` values; do not commit client IPs.

- [ ] **Step 5: Capture containers and error aggregation**

~~~bash
ssh -o BatchMode=yes sub2api-prod \
  'sudo -n docker ps --filter label=com.docker.compose.project=sub2api --format "{{.Names}}|{{.ID}}|{{.Status}}"; sudo -n docker stats --no-stream --format "{{.Name}}|{{.CPUPerc}}|{{.MemUsage}}|{{.PIDs}}"'
~~~

~~~bash
printf '%s\n' "SELECT status_code, error_phase, error_type, COALESCE(error_message,'_'), count(*) FROM ops_error_logs WHERE created_at >= now() - interval '10 minutes' GROUP BY 1,2,3,4 ORDER BY 5 DESC,1;" |
  ssh -o BatchMode=yes sub2api-prod \
    'sudo -n docker exec -i sub2api-postgres-1 sh -c '\''exec psql -X -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$POSTGRES_DB" -P pager=off -F "|" -At'\'''
~~~

Expected: Seven containers remain running; database output contains classifications and counts only.

- [ ] **Step 6: Create the initial report with `apply_patch`**

Populate: in-progress conclusion; unchanged hostname/free plan/no-container-change boundary; exact UTC window; DNS/DS/CAA result; TLS counts; health success count and median; seven container names/short IDs/statuses; error aggregation; and explicit “NS and proxy unchanged”. Leave no placeholder language.

- [ ] **Step 7: Verify and commit Task 1**

~~~bash
rg -n 'TBD|TODO|待填写|待替换' docs/project/project-progress.md docs/superpowers/reports/2026-08-05-cloudflare-free-edge-migration-production-verification.md || true
git diff --check -- docs/project/project-progress.md docs/superpowers/reports/2026-08-05-cloudflare-free-edge-migration-production-verification.md
git add docs/project/project-progress.md docs/superpowers/reports/2026-08-05-cloudflare-free-edge-migration-production-verification.md
git commit -m "docs: register Cloudflare edge migration"
~~~

Expected: No placeholder match, silent `git diff --check`, and only the two task files committed.

---

### Task 2: Create The Free Zone And Prove DNS Parity

**Files:**
- Modify: `docs/superpowers/reports/2026-08-05-cloudflare-free-edge-migration-production-verification.md`

**Interfaces:**
- Consumes: Task 1 baseline and logged-in Cloudflare/DNSPod sessions.
- Produces: A pending Cloudflare Free zone with every enabled DNSPod record represented and DNS only.

- [ ] **Step 1: Add the existing domain**

Use:

~~~text
Cloudflare Account home -> Add a domain -> xingqiaolab.top -> Free -> continue
~~~

Stop for user handoff on payment, new legal agreement, CAPTCHA, OTP, or reauthentication. Do not create an API token.

- [ ] **Step 2: Inventory DNSPod records**

Open `我的域名 -> xingqiaolab.top -> 记录管理` and compare each enabled tuple:

~~~text
(host/name, type, target/value, routing/line, TTL, enabled state)
~~~

Record only enabled totals and counts by type; never record TXT values.

- [ ] **Step 3: Correct Cloudflare import**

Require equivalence for every enabled DNSPod record. Keep all records DNS only during activation. Preserve MX priorities and exact values; do not invent AAAA. A line-routing record without safe Cloudflare equivalence blocks Task 3.

- [ ] **Step 4: Verify readiness**

Require Free plan, pending zone, exactly two assigned nameservers, `api` DNS only with the origin A, and Cloudflare DNSSEC disabled before registrar migration. A Task 1 DS record blocks Task 3.

- [ ] **Step 5: Update and commit parity evidence**

Add Free/pending status, enabled totals by type on both providers, `api` DNS-only state, DNSSEC result, and parity decision.

~~~bash
rg -n 'TBD|TODO|待填写|待替换' docs/superpowers/reports/2026-08-05-cloudflare-free-edge-migration-production-verification.md || true
git diff --check -- docs/superpowers/reports/2026-08-05-cloudflare-free-edge-migration-production-verification.md
git add docs/superpowers/reports/2026-08-05-cloudflare-free-edge-migration-production-verification.md
git commit -m "docs: verify Cloudflare DNS parity"
~~~

---

### Task 3: Switch Nameservers And Gate On Edge TLS

**Files:**
- Modify: `docs/superpowers/reports/2026-08-05-cloudflare-free-edge-migration-production-verification.md`

**Interfaces:**
- Consumes: Task 2 parity pass and the two Cloudflare nameservers visible in the UI.
- Produces: Active Cloudflare authoritative DNS while `api` remains direct, plus active Universal SSL.

- [ ] **Step 1: Recheck no-downtime conditions**

Require matching record counts, `api` DNS only with the DNSPod A, both assigned Cloudflare nameservers, no DNSSEC blocker, and unchanged DNSPod records.

- [ ] **Step 2: Obtain action-time confirmation**

Tell the user the next save replaces authoritative nameservers in Tencent Cloud. Do not submit before confirmation.

- [ ] **Step 3: Replace nameservers**

Change only `xingqiaolab.top` nameserver fields to the exact two visible Cloudflare values. Do not change registrant, renewal, transfer lock, contacts, or billing.

- [ ] **Step 4: Poll no more than once per minute**

~~~bash
date -u '+checked_at_utc=%Y-%m-%dT%H:%M:%SZ'
dig +short NS xingqiaolab.top
dig +short A api.xingqiaolab.top
dig +short MX xingqiaolab.top
dig +short TXT xingqiaolab.top
~~~

Expected: NS may vary during propagation, but `api` remains the origin and required MX/TXT answers remain.

- [ ] **Step 5: Gate on zone and certificate**

Keep `api` DNS only. Require Cloudflare zone Active and an Active edge certificate covering `api.xingqiaolab.top`.

- [ ] **Step 6: Update and commit activation evidence**

Record registrar save UTC time, NS transition, activation time, direct `api` result, and certificate state.

~~~bash
git diff --check -- docs/superpowers/reports/2026-08-05-cloudflare-free-edge-migration-production-verification.md
git add docs/superpowers/reports/2026-08-05-cloudflare-free-edge-migration-production-verification.md
git commit -m "docs: record Cloudflare DNS activation"
~~~

---

### Task 4: Configure And Enable The API Proxy

**Files:**
- Modify: `docs/superpowers/reports/2026-08-05-cloudflare-free-edge-migration-production-verification.md`

**Interfaces:**
- Consumes: Active zone, active edge certificate, healthy DNS-only origin.
- Produces: The official hostname served through Cloudflare Free with API-safe settings.

- [ ] **Step 1: Configure minimum settings**

Require: Full (strict); minimum TLS 1.2; TLS 1.3 enabled; WebSockets enabled; Under Attack disabled; no interactive challenge or Cache Everything on API paths. Do not enable paid features, Workers, Tunnel, Argo, Origin/Transform Rules, or firewall changes.

- [ ] **Step 2: Recheck direct health and container identity**

~~~bash
curl -sS -o /dev/null --connect-timeout 5 --max-time 15 -w 'code=%{http_code} remote=%{remote_ip} total=%{time_total}\n' https://api.xingqiaolab.top/health
ssh -o BatchMode=yes sub2api-prod 'sudo -n docker ps --filter label=com.docker.compose.project=sub2api --format "{{.Names}}|{{.ID}}|{{.Status}}"'
~~~

Expected: HTTP 200 and Task 1 identities.

- [ ] **Step 3: Obtain action-time confirmation**

Tell the user the next toggle changes the official hostname from direct origin to Cloudflare Free and DNS only is immediate rollback.

- [ ] **Step 4: Toggle only the existing `api` A record to Proxied**

Do not alter name, origin value, TTL semantics, or another record.

- [ ] **Step 5: Prove edge service**

~~~bash
dig +short A api.xingqiaolab.top
curl -sS -D - -o /dev/null --connect-timeout 5 --max-time 15 https://api.xingqiaolab.top/health | sed -n '1,30p'
~~~

Expected: A answers no longer expose the origin, HTTP 200, and `cf-ray` or `server: cloudflare` is present. Any TLS/Cloudflare error triggers rollback.

- [ ] **Step 6: Update and commit cutover evidence**

Record UTC time, checked settings, A classification, first health result, and Cloudflare header presence.

~~~bash
git diff --check -- docs/superpowers/reports/2026-08-05-cloudflare-free-edge-migration-production-verification.md
git add docs/superpowers/reports/2026-08-05-cloudflare-free-edge-migration-production-verification.md
git commit -m "docs: record Cloudflare proxy cutover"
~~~

---

### Task 5: Accept Or Roll Back And Close The Ledger

**Files:**
- Modify: `docs/project/project-progress.md`
- Modify: `docs/superpowers/reports/2026-08-05-cloudflare-free-edge-migration-production-verification.md`

**Interfaces:**
- Consumes: Direct baseline, proxied edge, current Codex Desktop, and production logs.
- Produces: A verified retained edge or same-hostname rollback, plus pushed evidence matching live state.

- [ ] **Step 1: Run edge TLS acceptance**

Repeat Task 1 handshake loop. Require 20/20 TLS 1.2 and 20/20 TLS 1.3.

- [ ] **Step 2: Run 20 edge health samples**

Repeat Task 1 health loop. Require 20 HTTP 200 and no curl failure. Calculate median:

~~~bash
sort -n | awk '{v[NR]=$1} END {if (NR==0) exit 1; if (NR%2) print v[(NR+1)/2]; else print (v[NR/2]+v[NR/2+1])/2}'
~~~

Require no more than 30% degradation from successful direct median; if direct had no successes, compare success rate only.

- [ ] **Step 3: Complete five real Codex streams**

Use current Codex Desktop and existing key. Complete five minimal distinct `/responses` requests with first event and normal completion. Record UTC boundaries, result, and sanitized request IDs only.

- [ ] **Step 4: Correlate the exact acceptance window**

Query `ops_error_logs` by captured UTC start/end and inspect API logs. Require no request-body read failure, abnormal 499, Cloudflare 403/413/502/522/524, panic/fatal, or duplicate completed request signal.

- [ ] **Step 5: Recheck production invariants**

Repeat Task 1 Docker snapshots and inspect the window for OOM/kernel errors. Require same container IDs, no restart increase, and healthy services.

- [ ] **Step 6: Roll back any failed gate**

Toggle only `api` from Proxied to DNS only, then verify:

~~~bash
dig +short A api.xingqiaolab.top
curl -sS -o /dev/null --connect-timeout 5 --max-time 15 -w 'code=%{http_code} remote=%{remote_ip} total=%{time_total}\n' https://api.xingqiaolab.top/health
~~~

Keep ledger “进行中” and record the failed gate plus direct restoration. Restore DNSPod NS only for persistent authoritative DNS failure and only after fresh action-time confirmation.

- [ ] **Step 7: Finalize report and ledger**

On full pass, set item 13 to:

~~~markdown
13. **Cloudflare Free 公网入口迁移**：保持 `api.xingqiaolab.top` 和首尔源站不变，权威 DNS 已迁移至 Cloudflare Free，`api` 已启用橙色云。受影响 Mac 的 TLS 1.2/1.3、连续健康请求、真实 Codex 流式请求、站内错误日志和容器不变性均已通过验收；DNSPod 区域保留 48 小时用于回退。**状态：已完成（配置已在线生效，正式域名已验证）**。
~~~

On rollback, keep “进行中” and state the failed gate and verified direct restoration.

- [ ] **Step 8: Verify final documentation**

~~~bash
rg -n 'TBD|TODO|待填写|待替换' docs/project/project-progress.md docs/superpowers/reports/2026-08-05-cloudflare-free-edge-migration-production-verification.md || true
git diff --check -- docs/project/project-progress.md docs/superpowers/reports/2026-08-05-cloudflare-free-edge-migration-production-verification.md
git status --short
~~~

Expected: No placeholders, silent `git diff --check`, unrelated files untouched.

- [ ] **Step 9: Commit and push**

~~~bash
git add docs/project/project-progress.md docs/superpowers/reports/2026-08-05-cloudflare-free-edge-migration-production-verification.md
git commit -m "docs: verify Cloudflare edge migration"
git push origin main
~~~

Expected: Push succeeds. Otherwise keep ledger “进行中” and do not claim completion.

- [ ] **Step 10: Run final whole-branch review**

Require approval that the hostname and origin stayed unchanged, only Free is enabled, no containers changed, DNSPod remains for 48 hours, evidence proves retention or rollback, ledger matches reality, and Git contains no credentials or DNS verification values.

