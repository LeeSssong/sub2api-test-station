# T54 Task 1 Report

## Scope

Implemented the normalized per-group scheduler policy contract, preset expansion, legacy JSON compatibility, validation, and settings DTO/API wiring.

## Changed files

- `upstream/sub2api/backend/internal/service/settings_view.go`
- `upstream/sub2api/backend/internal/service/setting_parse.go`
- `upstream/sub2api/backend/internal/service/setting_update.go`
- `upstream/sub2api/backend/internal/handler/dto/settings.go`
- `upstream/sub2api/backend/internal/handler/admin/setting_handler_update.go`
- `upstream/sub2api/backend/internal/handler/admin/setting_handler.go`
- `upstream/sub2api/backend/internal/service/scheduler_fairness_settings_test.go`

The typed policy map is additive to the legacy override map. Fair presets expand to fixed effective values; weighted policies merge only supplied weight keys with global values. Legacy pointer-field JSON is read as `weighted_override` and re-serialized without dropping legacy fields. Unknown modes/presets/fields, unknown groups, non-finite or negative weights, invalid fairness ranges, and zero base-weight totals fail closed.

## Verification

RED was observed before implementation: the new contract tests failed to compile because the policy types and normalization/parser functions were absent.

GREEN command:

`cd upstream/sub2api/backend && go test ./internal/service -run 'Test(OpenAIScheduler|NormalizeOpenAI|ResolveOpenAI|SettingParse)' -count=1`

Result: PASS.

`git diff --check`: PASS.

No migration, configuration schema, production data, deployment, push, or merge was performed. Expected deployment property remains `downtime_required=false`; rollback is the existing blue-green image or clearing the policy map to restore legacy settings.

## Residual risks

Group ID validation is enforced when a known-group set is supplied; settings parsing itself only has the positive-ID boundary because it does not load the group repository. Fairness still uses the existing account-level signals until Task 2 applies policy-aware scheduling.

## Commit

Commit subject: `feat: add validated per-group scheduler policies`
