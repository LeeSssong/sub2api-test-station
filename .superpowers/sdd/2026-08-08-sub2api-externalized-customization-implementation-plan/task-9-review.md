# Task 9 Review

### Spec Compliance

**Verdict: FAIL.** The implementation provides useful comparison primitives and
fail-closed UI fallbacks, but it does not implement a trustworthy, persisted
promotion authority. The frontend can accept self-authored evidence from the
same response whose data it is deciding to trust, while the actual control-plane
response has no connection to the persisted Go reports. The balance exception
also permits arbitrary value drift. In addition, window coherence, required
metric dimensions, ordered four-page cutover, one-step runtime rollback, the
literal rollback rehearsal, and the required evidence report remain incomplete.

### Strengths

- The five requested read-state spellings are centralized and unknown values
  fail closed to `legacy_only`
  (`upstream/sub2api/frontend/src/config/externalizationFlags.ts:1`,
  `upstream/sub2api/frontend/src/config/externalizationFlags.ts:36`).
- Decimal parsing uses `shopspring/decimal`, and account/request/bill identifiers
  are compared as sorted exact multisets
  (`relay-ops-service/internal/compare/service.go:350`,
  `relay-ops-service/internal/compare/service.go:361`).
- Both passed and failed comparisons are written to a regular `0600` JSONL file,
  followed by `Sync`, and repository errors prevent returning an unpersisted
  report from `CompareAndPersist`
  (`relay-ops-service/internal/compare/service.go:237`,
  `relay-ops-service/internal/compare/service.go:249`,
  `relay-ops-service/internal/compare/service.go:420`).
- Shadow and dual-read modes retain the legacy source; monitor/profitability
  locally reject malformed replacement payloads; Usage is conservatively kept
  on legacy while its contract is incomplete
  (`upstream/sub2api/frontend/src/config/externalizationFlags.ts:67`,
  `upstream/sub2api/frontend/src/views/admin/AccountMonitorView.vue:430`,
  `upstream/sub2api/frontend/src/views/admin/AccountProfitabilityView.vue:326`,
  `upstream/sub2api/frontend/src/views/admin/UsageView.vue:465`).
- The report is candid that the literal rehearsal failed, that accounting and
  reconciliation remain legacy-primary, and that the project remains in
  progress rather than claiming online completion
  (`.superpowers/sdd/2026-08-08-sub2api-externalized-customization-implementation-plan/task-9-report.md:128`,
  `.superpowers/sdd/2026-08-08-sub2api-externalized-customization-implementation-plan/task-9-report.md:167`).

### Issues

#### Critical

1. **The production-facing frontend gate trusts response-authored evidence and
   is disconnected from the persisted comparator.** `CutoverEvidence` is only a
   set of client-visible booleans, window names, a deadline, and an arbitrary
   string reference; it contains no persisted report identity set, operator,
   comparison timestamps, or verifiable authorization. `evidencePasses` trusts
   those fields directly, and both promotable pages pass `response.cutover` from
   the external response into that decision
   (`upstream/sub2api/frontend/src/config/externalizationFlags.ts:12`,
   `upstream/sub2api/frontend/src/config/externalizationFlags.ts:42`,
   `upstream/sub2api/frontend/src/views/admin/AccountMonitorView.vue:427`,
   `upstream/sub2api/frontend/src/views/admin/AccountProfitabilityView.vue:324`).
   Conversely, the actual Go control-plane `ReadModel` has no cutover field or
   report-repository dependency at all
   (`relay-ops-service/internal/controlplane/types.go:16`,
   `relay-ops-service/internal/controlplane/server.go:83`). Thus the shipped
   server cannot produce the claimed gate, while any future service/proxy or
   mocked client response that supplies the self-attested envelope can select
   `external_primary`; the same problem reaches `legacy_retired` through an
   equally self-attested nested object
   (`upstream/sub2api/frontend/src/config/externalizationFlags.ts:52`). The tests
   reproduce this weakness by constructing the passing envelope themselves
   (`upstream/sub2api/frontend/src/config/__tests__/externalizationFlags.spec.ts:5`,
   `upstream/sub2api/frontend/src/config/__tests__/externalizationFlags.spec.ts:35`).
   Promotion must be decided at the trusted server/release boundary from reports
   loaded from the durable repository, with authenticated operator and immutable
   report references; the browser must not turn booleans authored by the data
   source into promotion authority.

2. **Any balance mismatch can be declared explained merely by changing the
   timestamp and supplying two nonempty strings.** `balanceGapExplained` imposes
   no bound on collection-time skew, no link between the references and the
   snapshots, and no acceptable variance/reconciliation rule
   (`relay-ops-service/internal/compare/service.go:473`). As a result, a balance
   of `81.5000` versus `80.50` is deliberately asserted to pass after only a
   one-minute timestamp change and a new string reference
   (`relay-ops-service/internal/compare/service_test.go:131`,
   `relay-ops-service/internal/compare/service_test.go:149`,
   `relay-ops-service/internal/compare/service_test.go:155`). This violates the
   requirement for explicit collection-time/source evidence by treating the
   existence of metadata as proof that an arbitrary financial discrepancy is
   caused by collection timing. Such a report can authorize a false cutover.

#### Important

1. **The three reports are not required to describe a coherent page/data
   window or evidence run.** `EvaluatePageCutover` overwrites a map entry with
   whichever report for a window appears last, then checks each label in
   isolation; it never verifies compatible start/end ranges, common operator,
   comparison run/evidence lineage, or persisted ordering
   (`relay-ops-service/internal/compare/service.go:442`,
   `relay-ops-service/internal/compare/service.go:448`). The nominal
   minimum/default/maximum test actually gives every label the same one-hour
   start/end interval
   (`relay-ops-service/internal/compare/service_test.go:23`,
   `relay-ops-service/internal/compare/service_test.go:92`). Freshness is also
   weak: `GeneratedAt` need only be nonzero, so ancient or future source data can
   pass when a caller supplies a future `FreshUntil`
   (`relay-ops-service/internal/compare/service.go:479`). Add explicit window
   definitions/run identity, select the latest valid persisted set, and validate
   source generation time against the comparison and freshness bounds.

2. **The comparison schema does not faithfully represent all required business
   metrics.** Rank is modeled as one integer in the count map rather than exact
   ranked identifiers/positions, reconciliation is reduced to exception count
   and IDs, USD and CNY have no currency dimension, and only two global version
   strings exist for rate, margin, score, and rank
   (`relay-ops-service/internal/compare/report.go:32`,
   `relay-ops-service/internal/compare/report.go:55`,
   `relay-ops-service/internal/compare/report.go:64`,
   `relay-ops-service/internal/compare/report.go:71`,
   `relay-ops-service/internal/compare/report.go:82`). The test's single
   `MetricRank: 1` cannot detect reordered or per-entity rank drift
   (`relay-ops-service/internal/compare/service_test.go:29`). Use currency-keyed
   decimal values, exact rank entries tied to entity IDs, the required
   reconciliation fields, and calculation/rate version evidence associated with
   each derived metric.

3. **The ordered four-page cutover is not implemented.** The four build-time
   flags are independent, so profitability/accounting/reconciliation can be
   requested without a passed predecessor
   (`upstream/sub2api/frontend/src/config/externalizationFlags.ts:81`). Accounting
   explicitly never uses the external result
   (`upstream/sub2api/frontend/src/views/admin/UsageView.vue:465`), and the
   required report admits reconciliation has no replacement path at all
   (`docs/superpowers/reports/2026-08-08-externalization-dual-read-report.md:60`).
   This does not satisfy the specified monitor -> profitability -> accounting ->
   reconciliation rollout or the Task 9 feature-cutover step
   (`.superpowers/sdd/2026-08-08-sub2api-externalized-customization-implementation-plan/task-9-brief.md:25`).

4. **The claimed one-step rollback is a compile-time frontend setting, not an
   operational rollback control.** Each mode is captured from `import.meta.env`
   into a module-level object
   (`upstream/sub2api/frontend/src/config/externalizationFlags.ts:79`). In a Vite
   production bundle, changing this requires a new build/deployment, so the
   report's claim that setting `legacy_only` "immediately" rolls a page back is
   unsupported
   (`docs/superpowers/reports/2026-08-08-externalization-dual-read-report.md:68`).
   The one-step per-page rollback flag needs a trusted runtime configuration or
   server-side switch, and its actual result must be persisted as evidence.

5. **Task 9's required rehearsal and evidence report are still open acceptance
   criteria.** The brief requires the literal command to pass and the report to
   contain every window, flags, operator, timestamps, and rollback result
   (`.superpowers/sdd/2026-08-08-sub2api-externalized-customization-implementation-plan/task-9-brief.md:29`).
   The command exited 1 and no real rollback occurred
   (`.superpowers/sdd/2026-08-08-sub2api-externalized-customization-implementation-plan/task-9-report.md:130`).
   The required dual-read report is a contract/test narrative and explicitly
   says it contains no authorizing page records; it has no actual per-window
   values, active flag snapshot, operator, comparison/persistence timestamps, or
   rollback result
   (`docs/superpowers/reports/2026-08-08-externalization-dual-read-report.md:1`,
   `docs/superpowers/reports/2026-08-08-externalization-dual-read-report.md:71`,
   `docs/superpowers/reports/2026-08-08-externalization-dual-read-report.md:93`).
   The unavailable environment is documented honestly, but it is a missing
   acceptance criterion, not a pass; Task 9 must remain open until an authorized
   rehearsal and real report are produced.

#### Minor

None.

### Assessment

Task quality: Needs fixes
