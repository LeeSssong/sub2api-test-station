# Task 3 independent review request

Please independently review the next Task 3 commit for lifecycle and periodic
integration of account native-billing multiplier synchronization.

## Required checks

1. Eligible active OpenAI API-key accounts trigger exactly one asynchronous,
   forced native billing probe after create, edit, and probe-enable actions.
2. Inactive or ineligible accounts do not cause a lifecycle upstream HTTP
   request; lifecycle errors must not change a successful admin write response.
3. Lifecycle and scheduled probes both use the existing probe snapshot writer,
   so Task 2 persistence, CAS, audit, cache, and scheduler handling stay on
   one path. Verify trigger attribution: lifecycle versus scheduled.
4. Failed probes preserve existing multiplier and billing data.
5. Usage logs snapshot `accounts.rate_multiplier` and do not substitute the
   account's group multiplier.
6. No ordinary request forwarding code makes a billing probe call. No
   production data or configuration change is present.

## Evidence

- Implementer report: `task-3-report.md`
- Brief: `task-3-brief.md`
- Targeted lifecycle, scheduled, failed-preservation, and usage-log tests were
  run locally; the final full package regression output will be supplied with
  the commit.
- `git diff --check` will be run before commit.

## Reviewer constraints

Provide a spec-compliance verdict and task-quality verdict, with file/line
evidence for any finding. Review is read-only; do not edit the worktree or
rerun broad suites.
