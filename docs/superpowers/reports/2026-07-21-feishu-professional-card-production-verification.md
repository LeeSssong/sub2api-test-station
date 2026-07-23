# Feishu Professional Card Production Verification

**Date:** 2026-07-21 (Asia/Shanghai)  
**Result:** `PASS` (functional mainline closed; natural-event visual observation remains)  
**Production modes:** `RELAY_OPS_MODE=read_only`, `RELAY_OPS_FEISHU_COMMAND_MODE=dry_run`

## Scope

This follow-up verifies that the professional Interactive Card implementation is present in the running production image and renders in the real Feishu group. It does not enable route writes, change upstreams, reset notification deduplication or manufacture a production incident.

## Production Evidence

- Running image: `sub2api-relay-ops:feishu-proactive-alert-20260721-v3`.
- Image ID: `sha256:48474bc66f93af15a92d54d4ceec984677f071274b01b8ca43d4f70eb8dfaa4d`.
- Container was healthy with zero observed restarts during this check.
- The image was built after the typed card model, `interactive` App Bot payload and command-card sender were integrated.
- Production remained `read_only + dry_run`; no route, account, multiplier, price, balance or Key mutation was performed.

## Real Feishu Visual Acceptance

The signed-in Feishu web client was inspected without reading cookies, Local Storage, credentials or message identifiers.

Passed:

- Daily operations report rendered as an Interactive Card with a card title, structured status/result sections and an `运维后台` action button.
- At 02:32, one exact read-only command, `查询当前分组状态`, was sent in the existing acceptance group.
- The bot reply rendered as an Interactive Card titled `飞书命令执行结果`.
- The card reported `succeeded` and explicitly displayed `dry-run，仅预测，未写入路由`.

Pending natural-event verification:

- Existing synthetic alert and recovery messages were delivered before the current card image and are plain-text historical messages.
- Current alert/recovery templates, size limit, redaction and App Bot `interactive` wire payload are covered by automated tests and share the same production delivery client as the verified daily/command cards.
- Their real visual rendering remains pending until the next natural alert/recovery event. Deduplication rows were not deleted and no fault was created solely to generate evidence.

## Permission Boundary

A read-only Feishu OpenAPI history query was attempted with the existing App identity and correctly failed because the application does not have message-history scope. No scope was added. Visual verification used the already signed-in Feishu client instead.

## Conclusion

Production deployment and real rendering are confirmed for daily-report and command-reply cards. Alert/recovery production transport is implemented and regression-tested, and the alert/recovery semantics, delivery, deduplication and deterministic analysis path were already accepted in production before the visual-card follow-up. Their next natural event remains useful visual evidence, but it is a non-blocking observation item rather than an incomplete feature or release gate.

## Closure Audit

| Capability | Authoritative evidence | Result |
|---|---|---|
| Native monitor alert | `internal/nativealerts` tests cover two-window confirmation and notification; proactive production report records delivered alert | PASS |
| Recovery notification | `internal/nativealerts` and `internal/acceptance` cover one recovery transition; proactive production report records delivered recovery | PASS |
| Duplicate suppression | incident and delivery tests cover stable evidence deduplication; production retained exactly two synthetic deliveries and one same-date report after repeats | PASS |
| Daily operations report | `internal/dailyreport` covers stable Shanghai-date identity and all public groups; real Feishu card with action button was visually accepted | PASS |
| Read-only analysis | versioned redacted contract and deterministic fallback are tested; production report returned `fallback` without blocking delivery | PASS |
| Interactive Card transport | App Bot and webhook wire payload, official stable element shape, redaction and 30 KB limit are regression-tested | PASS |
| Command card | real group response showed `succeeded` and `dry-run，仅预测，未写入路由` | PASS |
| Alert/recovery card visual | transport and templates are tested; real screenshot waits for the next natural event without deleting dedup rows or manufacturing a fault | NON-BLOCKING OBSERVATION |

## Final Read-only Recheck

The latest production image superseding the original card image is `sub2api-relay-ops:candidate-admin-intake-20260721-v2` (`sha256:e103d218572255afbc1123a9a553f9ce5cf242d435d5636b7b2c4bf976e6c7f5`). It retains the verified alert/card implementation and was healthy with restart count `0` during closure. Production remained `read_only + dry_run`; `/healthz`, `/readyz`, `/pricing`, `/ops` and `/monitor` returned HTTP `200`.

The current Feishu routing hash still matches the post-deployment evidence:

```text
3262403ac7e948e9453e1487922ac538e066f60fd7d23474e66f4ee917f7435e
```

The latest redacted route/account/model canonical snapshot remains:

```text
4791b8f093077dc50316daa8e0f5c16aaf18d0d402aa47ca1b9bc0380020e1e3
```

Fresh verification of the current worktree passed:

```text
go test ./... -race -count=1
go vet ./...
node --check internal/http/static/ops.js
node --check internal/http/static/ops-admin.js
bash tests/relay_ops/validate_relay_ops_contract.sh
ruby ops/upstream-benchmark-v2.rb validate
git diff --check
```

No service was rebuilt, no synthetic event was sent, and no route, multiplier, price, balance, Key, candidate, probe or deduplication row was changed during this closure audit. The Feishu Agent/message mainline is complete. At the time of this audit the next mainline was D04 low-budget acceptance; the 2026-07-22 recheck below records its later completion.

## 2026-07-22 Mainline Recheck

D04 low-budget acceptance has since completed, so the project mainline has advanced to launch readiness. Feishu remains functionally closed. A fresh read-only recheck found relay-ops on `sub2api-relay-ops:candidate-admin-intake-20260721-v2`, healthy with restart count `0`, `RELAY_OPS_MODE=read_only`, and `RELAY_OPS_FEISHU_COMMAND_MODE=dry_run`; all five public endpoints returned HTTP `200`. Fresh relay-ops race tests, `go vet`, static JavaScript checks, and the deployment contract passed.

No Feishu event was sent, no service was rebuilt, and no deduplication state was touched. The alert/recovery card visual remains a non-blocking natural-event observation only.
