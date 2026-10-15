# Xingqiao Upstream Source Record

- Repository: `https://github.com/Wei-Shaw/sub2api.git`
- Release tag: `v0.2.8`
- Source commit: `fd80b08c90b55edcad5b00171b53f08721d30da1`
- Annotated tag object: `d7a82d78ca51d42be41cb4daa3510ea401defe9f`
- Imported: `2026-09-23`

This directory is an exported source snapshot without nested Git metadata.
Xingqiao-specific frontend and deployment customizations are maintained in
this snapshot and must be reviewed when importing a newer upstream release.

## Current fork integration — 2026-09-29

- Repository: `https://github.com/ranxi2001/sub2api`
- Branch: `production`
- Source commit: `e0b227cc07dfb7094f1c01889c9fdb9f1d391374`
- Version: `2.9.1`
- Previous full fork baseline: `9ee2041d3d8ab1235cb53725077e7c70ef5cefda`
- Non-billing conflicts prefer upstream. Preserve local billing integration and immutable executed migration history.

## Current fork integration — 2026-10-10

- Repository: `https://github.com/ranxi2001/sub2api`
- Branch: `production`
- Source commit: `ff9198947a70a83c59b8448f5ef13909119cfe7c`
- Version: `2.10.4`
- Previous imported source: `11be589504b482d77827ab383b4b53f777241238` (2.10.1)
- Method: 510 changed files imported from verified Git snapshots with three-way content merges; external history remains outside project main. Local billing/quota/admission, singleton roles, monitor-v4/SLA and user shell retained; Ent and Wire regenerated. Existing migration SQL bytes preserved.
- This release uses the user-authorized exact online migration pair `10a94ee6…` → `e5d79ffa…`; four new migrations have bounded lock/statement timeouts. Backward application recovery keeps schema/data and blocks incompatible active TOTP/Excel work, restart renewal orders and new provider platforms.

## Current fork integration — 2026-10-12

- Repository: `https://github.com/ranxi2001/sub2api`
- Release: `v2.10.5`
- Source commit: `d977e7a5e24e5536db6ce8b8478743e0c4b11f77`
- Previous imported source: `ff9198947a70a83c59b8448f5ef13909119cfe7c` (2.10.4).
- Three-way source snapshot import: 97 changed upstream files; preserve local billing/quota/admission, Fast scheduler capabilities, singleton roles, monitoring and user shell. Migration SQL and runtime dependency inputs unchanged.
- Restore ordinary-user TPS using upstream total-duration throughput, independent of admin-only timing details. Align calculator, hints and boundary tests with this release.
- Production branch commits after v2.10.5 are excluded. Test station is not part of this deployment.
