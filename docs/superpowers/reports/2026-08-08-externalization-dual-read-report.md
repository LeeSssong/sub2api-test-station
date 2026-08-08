# Externalization Dual-Read Report

Status: local rehearsal gate only. No production traffic was switched.

The cutover contract is represented by per-page modes: `legacy_only`,
`shadow_building`, `dual_read_comparing`, `external_primary`, and
`legacy_retired`. A page may enter `external_primary` only when decimal
amounts, counts, permissions, exports, freshness and rollback evidence all
pass. A degraded control-plane response always keeps the legacy page/data
source available.

Required comparison windows are minimum, default, and maximum administrator
time ranges. The report schema records counts, decimal amounts, rate and
calculation versions, freshness, permission/export checks and rollback result.

This report is a design and local-contract artifact; it is not production
deployment or online acceptance evidence.
