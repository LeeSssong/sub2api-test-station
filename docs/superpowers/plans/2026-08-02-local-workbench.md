# Local Extraction Payment Workbench Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a local single-user web workstation that accepts 1:1:1 token/CDK rows, runs the documented extraction and payment APIs, persists work in SQLite, and shows live and final task states.

**Architecture:** A Node.js server owns the SQLite database, the persistent upstream Cookie session, and a resumable background job runner. A dependency-free browser frontend provides spreadsheet-style row entry, batch history, and polling-based status updates. The upstream client is isolated behind an injectable interface so API behavior and retry handling can be tested without spending real credentials.

**Tech Stack:** Node.js 26, built-in `node:http`, built-in `node:sqlite`, native `fetch`, browser ES modules, Lucide icons, Node test runner.

**Path Scope:** All implementation paths in this plan are relative to `tools/local-workbench/`; project documentation paths remain relative to the repository root.

## Global Constraints

- Run only as a local single-user application.
- Persist batches and task progress across restarts with SQLite.
- Keep every row's `access_token`, extraction CDK, and payment CDK strictly paired.
- Follow `https://kk.642636.xyz` API limits: 100 extraction rows, 500 payment URLs, and 100 payment status IDs per request.
- Preserve one `helan_client` Cookie across submissions and status polling.
- Retry `429`, `502`, and `503` with bounded exponential backoff and honor `Retry-After`.
- For payment `207`, persist accepted submissions and retry only `remainingPaymentUrls`.

---

### Task 1: Input Parsing And Project Shell

**Files:**
- Create: `package.json`
- Create: `src/input-parser.js`
- Test: `test/input-parser.test.js`

**Interfaces:**
- Produces: `parsePastedRows(text: string): Array<{ accessToken: string, extractionCdk: string, paymentCdk: string }>`
- Produces: `validateRows(rows): { valid: boolean, errors: Array<{ row: number, message: string }> }`

- [ ] Write tests for tab-separated, comma-separated, blank-line, and incomplete-row input.
- [ ] Run `npm test -- test/input-parser.test.js` and verify missing-module failure.
- [ ] Implement normalization and strict three-column validation.
- [ ] Run the focused tests and verify they pass.

### Task 2: SQLite Batch Repository

**Files:**
- Create: `src/database.js`
- Test: `test/database.test.js`

**Interfaces:**
- Produces: `createDatabase(path)` with batch, task, setting, status-update, and resumable-batch methods.
- Consumes: normalized rows from Task 1.

- [ ] Write repository tests for batch creation, row ordering, state updates, settings, and unfinished-batch lookup.
- [ ] Run `npm test -- test/database.test.js` and verify missing-module failure.
- [ ] Implement schema creation and prepared repository operations with `node:sqlite`.
- [ ] Run the focused tests and verify they pass.

### Task 3: Upstream API Client

**Files:**
- Create: `src/upstream-client.js`
- Test: `test/upstream-client.test.js`

**Interfaces:**
- Produces: `createUpstreamClient({ baseUrl, fetchImpl, cookieStore, sleep })`.
- Produces methods `submitExtractions`, `getExtraction`, `submitPayments`, and `getPaymentStatuses`.

- [ ] Write tests proving request payloads, Cookie capture/reuse, text extraction responses, and retriable status handling.
- [ ] Run `npm test -- test/upstream-client.test.js` and verify missing-module failure.
- [ ] Implement bounded backoff, `Retry-After`, JSON/text parsing, and typed upstream errors.
- [ ] Run the focused tests and verify they pass.

### Task 4: Resumable Batch Runner

**Files:**
- Create: `src/batch-runner.js`
- Test: `test/batch-runner.test.js`

**Interfaces:**
- Produces: `createBatchRunner({ database, upstream, sleep, onChange })` with `start(batchId)` and `resume()`.
- Consumes repository methods from Task 2 and upstream methods from Task 3.

- [ ] Write a happy-path test from extraction submission through completed payment.
- [ ] Write failure and resume tests for failed extraction, `207` remaining URLs, and pending payment polling.
- [ ] Run `npm test -- test/batch-runner.test.js` and verify missing-module failure.
- [ ] Implement extraction chunks, polling, payment-CDK grouping, status chunks, terminal-state calculation, and single-run locking.
- [ ] Run the focused tests and verify they pass.

### Task 5: Local HTTP API And Static App

**Files:**
- Create: `src/server.js`
- Create: `public/index.html`
- Create: `public/styles.css`
- Create: `public/app.js`
- Create: `public/vendor/lucide.min.js`
- Test: `test/server.test.js`

**Interfaces:**
- Produces HTTP endpoints `GET /api/batches`, `GET /api/batches/:id`, and `POST /api/batches`.
- Produces a responsive workstation with editable rows, Excel paste, start action, current detail, and history selection.

- [ ] Write HTTP tests for validation, batch creation, list, detail, and not-found responses.
- [ ] Run `npm test -- test/server.test.js` and verify missing-module failure.
- [ ] Implement JSON routing, static serving, startup resume, and graceful shutdown.
- [ ] Implement the approved two-pane workstation and responsive mobile layout.
- [ ] Run server tests and browser syntax checks.

### Task 6: End-To-End Verification And Runbook

**Files:**
- Create: `README.md`

**Interfaces:**
- Documents local installation, startup, data location, and workflow.

- [ ] Run the complete `npm test` suite and confirm zero failures.
- [ ] Start the app with a temporary database and verify health/list/create validation manually.
- [ ] Open desktop and mobile views, capture screenshots, and check layout, controls, and non-overlap.
- [ ] Confirm restart recovery against a seeded unfinished batch without making real upstream submissions.
- [ ] Document `npm start`, local URL, SQLite path, and API source.
