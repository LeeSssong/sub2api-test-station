# Manual Ledger Simulation Implementation Plan

> **For agentic workers:** Execute inline under the primary-agent workflow. Use test-driven development and preserve D13.

**Goal:** Build a tamper-evident local manual-recharge ledger and reconciliation tool without connecting to payment or Sub2API.

**Architecture:** Store one canonical JSON event per line, chain events with SHA-256, validate cross-event invariants, and derive summaries and no-send API previews. Keep real ledgers in ignored `*.local.jsonl` files with mode 0600.

**Tech Stack:** Ruby 2.6 standard library, JSON, SHA-256, BigDecimal, Minitest, Markdown.

## Global Constraints

- Do not call a network endpoint or use a real credential.
- Example events must use ledger mode `simulation` and simulated references.
- Sub2API balance adjustments use positive amounts with `add` or `subtract` and a required idempotency key.
- Never use `set` in routine recharge or correction workflows.
- Never delete or rewrite an existing event to fix an error.

---

### Task 1: Test and implement ledger core

**Files:**
- Create: `tests/ledger/manual_ledger_test.rb`
- Create: `ops/manual-ledger.rb`

**Interfaces:**
- `ManualLedger.build_event(existing_events, payload)` returns a chained event or raises `ValidationError`.
- `ManualLedger.verify(events)` returns validation errors.
- `ManualLedger.summary(events)` returns a JSON-serializable reconciliation.
- `ManualLedger.request_preview(event)` returns a no-send Admin API request description.

- [x] Write chain, tamper, semantic, reconciliation, preview, and secret-rejection tests.
- [x] Run tests and confirm failure because the ledger core does not exist.
- [x] Implement the minimum core and CLI commands: `init`, `append`, `verify`, `summary`, `request-preview`.
- [x] Run tests in warning mode with no failures or warnings.

### Task 2: Add simulation artifacts and runbook

**Files:**
- Modify: `.gitignore`
- Create: `config/ledger/manual-ledger.example.jsonl`
- Create: `docs/project/manual-operations.md`

- [x] Ignore `config/ledger/*.local.jsonl` and operational ledger directories.
- [x] Add a valid simulation chain with payment, balance credit, and usage snapshot.
- [x] Document file permissions, two-person facts check, API preview, append-only correction, daily reconciliation, and evidence.
- [x] Verify and summarize the example without sending requests.

### Task 3: Persist and verify state

**Files:**
- Modify: `docs/project/current-state.md`
- Modify: `docs/superpowers/plans/2026-07-15-commercial-ai-api-relay-implementation-plan.md`
- Create: `docs/superpowers/reports/2026-07-15-manual-ledger-verification.md`

- [x] Record offline readiness while leaving D05, actual payment, actual balance changes, and live reconciliation open.
- [x] Run ledger, pricing, upstream, infrastructure, secret, Markdown, ignore, permission, and project-container checks.
- [x] Self-review Sub2API contract, units, hash behavior, D13, and simulation labeling.

## Verification Commands

- `ruby -w tests/ledger/manual_ledger_test.rb`
- `ruby -w ops/manual-ledger.rb verify --ledger config/ledger/manual-ledger.example.jsonl`
- `ruby -w ops/manual-ledger.rb summary --ledger config/ledger/manual-ledger.example.jsonl --format json`
- `ruby -w tests/pricing/pricing_calculator_test.rb`
- `ruby -w tests/upstreams/validate_upstream_test.rb`
- `bash tests/infra/validate-baseline.sh`

## Acceptance

- [x] Example chain verifies and reconciles to zero payment-credit variance.
- [x] No-send request preview has correct path, idempotency key, operation, and amount, with no auth header.
- [x] Real ledger paths are ignored and documented as mode 0600.
- [x] D05 and all external actions remain unresolved.

## Risks

- The local ledger does not prove a bank payment; the operator must verify the external settlement reference.
- A hash chain detects edits but does not replace off-host backups or access control.
- A successful API response still requires a follow-up user balance and Sub2API balance-history check.
