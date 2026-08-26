# T77 Licensed v4.1.1 Detector Reset Handoff

## Candidate

- Baseline: `main@d704e85f75ba632f9ab7341d9ac238b1aefe8365`.
- Branch: `codex/t77-licensed-v411-detector-reset`.
- Scope: replace the `native-1` detector executable with the commercially authorized `chen-006/gpt56_api_detector` v4.1.1 release adapter and provide a guarded production purge for incompatible detection history.
- No database migration, UI redesign, scheduler eligibility change, billing change, or GitHub Actions change.

## Licensed Artifact

- URL: `https://github.com/chen-006/gpt56_api_detector/releases/download/v4.1.1/gpt56_api_detector_github_upload.zip`.
- SHA-256: `70c0c2f092e66cd219f2384e08872e5bedb4559e427c2e320d0070186376f865`.
- The Docker build verifies the checksum before unpacking and retains `LICENSE`, `NONCOMMERCIAL_NOTICE_CN.md`, and the complete release runtime below `/app/vendor/gpt56-v411`.
- The private wrapper exposes only the existing `/healthz`, token-gated `/v1/catalog`, and token-gated `/v1/detect` contracts. It creates a temporary per-run directory, disables upstream retention, and returns bounded Juice/fingerprint summaries only.

## Validation

- `python3 upstream/sub2api/deploy/model-detector-v411-adapter_test.py`: passed.
- `bash tests/operations/purge_account_model_detection_runs_test.sh`: passed.
- `bash tests/operations/model_detector_compose_contract_test.sh`: passed.
- `go test ./internal/service -run 'TestHTTPAccountModelDetectionSidecar|Test.*AccountModelDetection.*Sidecar' -count=1`: passed.
- `go build ./cmd/server`, `gofmt -d`, `bash -n`, Python compile, and `git diff --check`: passed.
- Account-monitor frontend tests, `pnpm typecheck`, and `pnpm build`: passed.
- `bash tests/operations/deploy_sub2api_blue_green_host_test.sh`: passed, including maintenance gates, two-slot rehearsal, rollback, lock concurrency, and immutable release-ID recovery.
- Local Docker image `sub2api-t77-v411:local` built as `sha256:00db07923eaa83e1dd85492e18c453a34cb06b3888ab945a8b02fcce98c7a1ec`; its v4.1.1 modules imported successfully, release notices were present, and the adapter returned health, authenticated catalog, unauthenticated rejection, and bounded invalid-request responses as contracted.

## Production Sequence

1. Refresh the candidate from current clean `main`, then repeat the direct checks affected by the refresh.
2. Merge to root `main`, build the qualified candidate image, and run the release preflight. Stop if it reports `downtime_required=true`.
3. Before promotion, invoke `ops/purge-account-model-detection-runs.sh` on the production host with `T77_EXPECTED_ROWS=3676` and a new safe absolute `T77_BACKUP_DIR`.
4. The guard creates a mode-600 `pg_dump` archive and SHA-256, checks no foreign keys reference the table, locks only `account_model_detection_runs`, deletes only `detector_version IS DISTINCT FROM '4.1.1'`, and verifies that exactly 3,676 rows were deleted with zero non-v4.1.1 rows remaining.
5. Promote through the existing blue-green chain and verify public health, sidecar catalog version `4.1.1`, a bounded real run with real fingerprint evidence, and zero legacy rows.

## Rollback and Risks

- Image rollback uses the preceding verified blue-green image. The cleanup is intentionally not automatic to reverse; restoring the table export requires a separate explicit production operation.
- A real upstream detection cannot be performed locally without an in-scope account and temporary credential. Production validation must confirm it yields real non-empty fingerprint evidence rather than a synthetic result.
