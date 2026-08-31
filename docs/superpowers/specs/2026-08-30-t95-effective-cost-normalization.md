# Spec: T95 账号有效成本归一化与官方利润保护

## Goal

让官方利润保护使用统一的有效成本倍率 `U`，覆盖 API Key 直接倍率、API Key 比例型上游和 OAuth 自购账号，同时保留官方可用性优先/fail-open 语义。

## Existing Native Capability

- Groups 原生字段 `profit_control_enabled`、`profit_min_margin`、`profit_safety_buffer` 继续作为唯一分组利润配置入口。
- 官方利润门、并发、粘性、重试和故障切换保持不变；仅替换账号成本读取为统一 provider。
- `accounts.rate_multiplier` 继续兼容 API Key 直接倍率和上游返回倍率 `R`。
- OAuth 采购成本字段继续复用现有采购入口；本切片将其纳入利润门计算。

## Cost Models

| Account | Model | Inputs | Formula |
| --- | --- | --- | --- |
| API Key | `direct_multiplier` (default) | `rate_multiplier` (`R`) | `A=1; U=R` |
| API Key | `ratio_based_upstream` | actual cost, obtained quota, `rate_multiplier` (`R`) | `A=actual_cost/obtained_quota; U=A*R` |
| OAuth | `self_owned` (locked) | procurement cost, estimated usable quota | `A=procurement_cost/estimated_quota; U=A` |

The self-owned model treats the stored cost and quota as the same 1:1 settlement unit. It has no upstream `R` input. `U` is computed server-side and is never accepted as an administrator input.

## Data and API

- Store `effective_cost_model`, `upstream_actual_cost`, and `upstream_obtained_quota` in the existing `accounts.extra` JSONB extension; no new SQL columns are required.
- Both ratio inputs are required together for the ratio model and use the same settlement unit.
- Existing `rate_multiplier` is the upstream-returned multiplier for API Key models. Existing procurement fields remain the self-owned inputs.
- Account create/update API accepts `effective_cost_model`, `upstream_actual_cost`, and `upstream_obtained_quota`. OAuth requests reject non-`self_owned` models and any ratio inputs. API Key requests default a missing model to `direct_multiplier`.
- Account responses include model, ratio inputs, `effective_cost_a`, `effective_cost_r`, `effective_cost_u`, and `effective_cost_status` for administrators.

## Runtime

`EffectiveCostProvider` computes a finite non-negative `U` from the account type/model. Invalid or incomplete input returns `unknown` rather than zero. The official profit veto consumes `U`; provider failure/unknown remains fail-open for availability, while the observer records the existing invalid-rate reason.

## Acceptance Criteria

- [ ] Direct API Key defaults to `direct_multiplier`, computes `U=rate_multiplier`.
- [ ] Ratio API Key computes `U=(actual/obtained)*rate_multiplier` and rejects incomplete/invalid values.
- [ ] OAuth always computes `U=procurement/estimated_quota`, ignoring legacy `rate_multiplier` and upstream ratio fields.
- [ ] Official profit control gates on `U` without a second profit gate.
- [ ] Existing Groups profit fields and fail-open behavior remain unchanged.
- [ ] Admin cost dialog can choose the API Key model and enter ratio parameters; OAuth remains self-owned.
- [ ] No history rows, production data, SQL migration, or deployment are changed in this task.

## Risks

- Existing `rate_multiplier` also feeds billing and scheduler scoring; this slice keeps it unchanged and only normalizes the profit-admission read path.
- The JSONB extension avoids generated Ent schema churn; existing `extra` merge/preservation paths must retain the three managed keys.
