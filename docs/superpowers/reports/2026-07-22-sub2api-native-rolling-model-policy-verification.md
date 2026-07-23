# Sub2API Native Rolling Model Policy Verification

**Acceptance window:** 2026-07-22 to 2026-07-23 (Asia/Shanghai)  
**Result:** `PASS` for read-only discovery and `/ops` projection; `NOT READY` for qualification or publication  
**Production modes:** relay-ops `read_only`, Feishu commands `dry_run`, D04 `read_only`, registration closed

## Scope And Reuse

This increment reuses Sub2API `v0.1.161` as the authority for active/schedulable accounts, group membership, upstream model discovery, account `model_mapping`, public-group `models_list_config`, and model pricing. Relay-ops only reads a bounded, secret-free result file and displays it on the hidden-admin `/ops` page.

The production deployment does not contain an alternate upstream editor, model editor, discovery scheduler, paid probe scheduler, or promotion endpoint. Promotion remains a separately invoked operator action and is valid only for a fresh `可升级` proposal.

## Discovery Result

The current account set is derived only from `status=active && schedulable=true`:

| Account | Public group IDs | Discovered candidate subset |
|---|---:|---|
| `10` | `6` | all five bootstrap models |
| `11` | `2` | all five bootstrap models |
| `12` | `6` | `gpt-5.5`, `gpt-5.6-sol`, `gpt-5.6-terra` |

Native discovery also returned older, special-purpose, dated, image, audio, realtime, Codex, compact, and mini models. Policy excluded those from the bootstrap customer catalog. No GPT minor family newer than `5.6` was discovered.

The approved bootstrap candidate is exactly:

```text
gpt-5.5
gpt-5.6
gpt-5.6-luna
gpt-5.6-sol
gpt-5.6-terra
```

Sub2API native pricing was complete for all five models. Both public groups had identical 13-model stored lists but `models_list_config.enabled=false`, so no new restricted public catalog was published.

## Evaluation Result

```text
status: 待测试
proposal_id: eda38d86e130d156d2eb1c267cca8289771278e4da5ce9ddb8651f390bf3d09b
account_set_sha256: cf28d87d0070ac5eca5847714ad4512b01b8e1cc098bf47691924cbf484aef3c
base_config_sha256: 1261a40c660b6b6d6a4e47c3e6ce63825e36302b7c01832ef5ed676c71690f68
result_file_sha256: b41ec345b9f8c2b24f94c94e120d3eb07b2623657840e4e112b694e49c0a77fd
```

Current blockers are:

```text
bootstrap_qualification_required
financial_evidence_missing
group_model_coverage_incomplete
quality_evidence_missing
```

Discovery issued exactly one native model-sync operation for each of the three current accounts. It sent zero model generation requests. No synchronous/SSE candidate qualification was started, no upstream cost was created by this acceptance, and no candidate model was marked qualified.

## Production Deployment

Only relay-ops was recreated, using:

```text
image: sub2api-relay-ops:model-release-read-only-20260722-v1
manifest: sha256:d3ce06995450fab24eaa87e3d1d826bf7df914be78822f0d97b2c4b851c242f5
container: f55c89f823f0be50749f69b1d100ed9752f136d7e0a7e69990d7f9cdf0f39156
health: healthy
restart_count: 0
```

The secret-free result is mounted at `/run/relay-ops/model-release-result.json` with `rw=false`. The production Compose file backup is `compose.yaml.bak-model-release-20260722-v1`; its deployed SHA-256 is `52d52751634f4b1c26bd9ad14b317a276891924c776c28d1be4d7e5b5b89ad30`.

Sub2API had already been minimally recreated during discovery preparation because the two current account hostnames were absent from its outbound URL allowlist. The prior allowlist value remains in the restricted server evidence directory. The final Sub2API container is healthy with restart count `0`; no account mapping, group model list, route, multiplier, price, balance, Key, or database record was changed by the rolling-model deployment.

## `/ops` Acceptance

- `/healthz`, `/readyz`, `/ops`, `/monitor`, and `/pricing` returned HTTP `200`.
- Anonymous and invalid-token calls to `/relay-ops/api/ops-view` returned HTTP `404`.
- A real administrator session displayed `模型版本`, `待测试`, the five bootstrap candidates, two public groups, three current accounts, and Chinese blocker summaries.
- The rendered page contained zero forms and zero `input/select/textarea/button` elements.
- The desktop page had no full-page horizontal overflow.
- No Base URL, Key, model editor, probe action, or publication action was exposed.

## Zero-Write Evidence

The fresh post-deployment native GET re-read produced the same account-set and base-configuration hashes as the pre-deployment proposal. Current account mapping counts remain `20/20/20`; public groups `2` and `6` still have identical 13-model stored lists with `enabled=false`.

The following unaffected container IDs remain unchanged:

```text
sub2api: 970f06ea63b84529580ef2522522e6bd9b2e7281e55a249c911ac9b4537de992
postgres: 2db52788ad733522b3398f3ba9c0ff4c45a418c360a57424a9e115feb43d4db6
redis: c45202c0d9e64f27d21191e87681c3ccb70e927555b74a4b9a47eb701afaa475
caddy: 9199ce54eacdc0e3e12bde41217a0946f7670384484b34a10c7f95d415917ce9
```

D04 remains `read_only` with `D04_REGISTRATION_OPEN=false`. The Feishu routing file remains read-only and its SHA-256 is `3262403ac7e948e9453e1487922ac538e066f60fd7d23474e66f4ee917f7435e`. Sub2API logs contained no account bulk-update or group PUT during this acceptance.

## Remaining Work

Read-only discovery and display are complete. Publication is not approved and is not technically ready. The next bounded upstream-test stage must:

1. Collect trustworthy USD balance evidence and fresh account-attributed natural quality evidence for every participating current account.
2. Run three minimal synchronous and three terminal-complete SSE attempts for each candidate/account pair that the account discovered.
3. Regenerate the proposal and require complete union coverage in every public group.
4. Stop at `可升级`; execute no promotion until a separate action-time approval.

This work precedes D04 controlled opening. D04 must also independently pass its dynamic-account lightweight launch gate before registration can be enabled.
