# Verification: UP01 Offline Intake Package

## Scope

Verify the versioned upstream YAML contract, secret controls, safe example, operator guide, deferred live checklist, project state updates, and regression compatibility with the existing infrastructure baseline.

## Commands and Results

| Check | Result |
|---|---|
| `ruby -c ops/validate-upstream.rb` | PASS: syntax OK |
| `ruby -w tests/upstreams/validate_upstream_test.rb` | PASS: 13 runs, 30 assertions, 0 failures/errors/skips, no warnings |
| `ruby -w ops/validate-upstream.rb config/upstreams/UP01.example.yaml` | PASS: example OK |
| Strict validation of the draft example | Expected rejection: exit 1 with exactly `readiness` not `ready_for_live_test` |
| `git check-ignore -q --no-index config/upstreams/UP01.local.yaml` | PASS: local real file path is ignored |
| `bash tests/infra/validate-baseline.sh` | PASS: infrastructure baseline contracts |
| Controlled artifact secret-pattern scan | PASS after excluding the old plan that contains the detector regex itself |
| Markdown fence balance scan | PASS |
| `docker compose ... ps -q` | PASS: no active containers in the project |

Verification bundle completed on 2026-07-15 in the local workspace. No upstream request, login, payment, recharge, purchase, merchant activation, DNS change, or production deployment was performed.

## Evidence

- Red phase: `ruby tests/upstreams/validate_upstream_test.rb` initially failed with `LoadError` because `ops/validate-upstream.rb` did not exist.
- Green phase: 13 validator tests cover normal and strict readiness, HTTPS, URL user information/query/fragment, allowlist host equality, forbidden credential keys, suspected secret values, symbolic secret references, missing supplier facts, duplicate models, incomplete pricing, and concurrency bounds.
- The example contains fictional values and a symbolic `sub2api-admin://accounts/UP01` reference, not a credential.
- Sub2API mapping was checked against v0.1.155 source commit `41cec0db059ffb82d0efdcfcf07a24ab51fbfe97`.

## Review Findings

- No unsupported claim that RPM, TPM, timeout, daily cost cap, balance, refund, or resale fields are native Sub2API account controls; the guide labels them supplier facts.
- `rate_multiplier` is explicitly documented as account cost-statistics behavior rather than user price.
- The OpenAI automatic-passthrough/model-whitelist interaction is documented as a live-test risk.
- Real files are ignored, and secret-shaped fields are rejected anywhere in YAML except the symbolic `secret_ref` field.
- D13 remains intact: all real external actions are deferred.

## Not Verified

- UP01 Base URL, authentication, model list, prices, balance, limits, refund policy, support response, or resale terms.
- Real Sub2API account creation, URL allowlist update, streaming, errors, cancellation, usage, billing, or log redaction.
- Any production server, domain, HTTPS certificate, customer payment, subscription account, or merchant channel.

These items require actual non-sensitive supplier facts and later user-authorized access. They remain open in `docs/superpowers/checklists/upstream-live-acceptance.md`.

## Follow-up

Proceed with L1-4 offline cost and billing simulation using fictional fixtures and explicit inputs. Do not invent UP01 prices or mark live acceptance complete.
