# Upstream Intake and Acceptance Implementation Plan

> **For agentic workers:** Execute inline because the user designated the primary AI as the worker. Follow test-driven development and preserve D13.

**Goal:** Build a safe, locally verifiable UP01 intake package without requiring a purchased asset, real credential, recharge, or network request.

**Architecture:** Keep supplier facts in a versioned YAML document and validate them with Ruby standard-library code. Keep real local documents ignored, and use a separate manual checklist for checks that can only run after assets and credentials exist.

**Tech Stack:** YAML, Ruby 2.6 standard library, Minitest, Markdown.

## Global Constraints

- Do not perform real login, payment, recharge, purchase, merchant activation, or upstream calls.
- Do not store API keys, passwords, cookies, OAuth tokens, private keys, or payment credentials.
- Align the Sub2API mapping with v0.1.155 source at commit `41cec0db059ffb82d0efdcfcf07a24ab51fbfe97`.
- Treat servers, domains, upstream balances, and accounts as unpurchased or assumed assets under D13.

---

### Task 1: Define and test the validator contract

**Files:**
- Create: `tests/upstreams/validate_upstream_test.rb`
- Create: `ops/validate-upstream.rb`

**Interfaces:**
- `UpstreamConfigValidator.new(document, live_ready: false).errors` returns field-qualified validation errors.
- `ruby ops/validate-upstream.rb [--live-ready] FILE...` exits 0 only when every file is valid.

- [x] Write tests for valid intake, strict readiness, URLs, host equality, secret rejection, required fields, and model uniqueness.
- [x] Run the tests and confirm they fail because the validator does not exist.
- [x] Implement the smallest standard-library validator that passes the tests.
- [x] Run the tests and confirm they pass without warnings.

### Task 2: Add the safe example and operator documents

**Files:**
- Create: `config/upstreams/UP01.example.yaml`
- Modify: `.gitignore`
- Create: `docs/project/upstream-intake.md`
- Create: `docs/superpowers/checklists/upstream-live-acceptance.md`

- [x] Add a fictional, structurally complete draft example with no credential.
- [x] Ignore `config/upstreams/*.local.yaml`.
- [x] Document copy, fill, validate, and Sub2API admin-entry steps.
- [x] Document the deferred live acceptance sequence and evidence to capture.
- [x] Validate the example normally and confirm strict mode rejects only its draft readiness.

### Task 3: Update durable project state and verify

**Files:**
- Modify: `docs/project/current-state.md`
- Modify: `docs/project/assets-register.md`
- Modify: `docs/superpowers/plans/2026-07-15-commercial-ai-api-relay-implementation-plan.md`
- Create: `docs/superpowers/reports/2026-07-15-upstream-intake-verification.md`

- [x] Record the completed offline package and the still-deferred real checks.
- [x] Run Ruby tests, example validation, Git ignore checks, secret-pattern scans, Markdown fence checks, the infrastructure baseline regression test, and the container cleanup check.
- [x] Review the diff for unsupported Sub2API claims, secrets, payment actions, and scope creep.

## Verification Commands

- `ruby tests/upstreams/validate_upstream_test.rb`
- `ruby ops/validate-upstream.rb config/upstreams/UP01.example.yaml`
- `ruby ops/validate-upstream.rb --live-ready config/upstreams/UP01.example.yaml` (expected failure: draft)
- `git check-ignore config/upstreams/UP01.local.yaml`
- `bash tests/infra/validate-baseline.sh`

## Acceptance

- [x] All automated checks pass except the intentional strict-mode draft check.
- [x] No live external action has occurred.
- [x] A future session can fill UP01 facts without rediscovering the required fields.

## Risks

- Upstream fields may remain unknown until the user can access its dashboard; unknown real-world facts remain unverified rather than being guessed.
- Passing local validation proves document safety and completeness, not upstream availability, resale permission, pricing accuracy, or Sub2API interoperability.
