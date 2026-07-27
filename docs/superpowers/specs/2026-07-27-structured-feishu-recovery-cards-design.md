# Structured Feishu Recovery Cards Design

## Goal

Turn every Feishu recovery notification into a scannable, structured card that leads with the recovery conclusion and key metrics while retaining the existing operational evidence, redaction, size limits, and action link behavior.

## Scope

This change covers recovery notifications produced by:

- site runtime monitoring;
- upstream account-quality monitoring;
- native Sub2API channel monitoring;
- group availability monitoring;
- upstream usage-session monitoring; and
- synthetic acceptance monitoring.

Alert, command, digest, and pricing-change cards keep their current visual structure. No production deployment is part of this change.

## Information Hierarchy

Every recovery card uses the same four-layer structure:

1. A short recovery conclusion and one supporting sentence.
2. A responsive matrix containing two to four key metrics.
3. A compact evidence block containing the decision basis, data source, and optional follow-up observation.
4. The existing primary action, normally `运维后台`.

The card header stays green and names the recovery domain and affected object. The body does not repeat the full header title.

## Content Rules

- User-facing metric labels and values use plain Chinese business language.
- Internal enum values such as `paused`, `balance_exhausted`, and `active && schedulable` remain available to incident state, evidence hashes, and tests but are not the primary card copy.
- A card renders only metrics that have meaningful values. It never adds empty cells or the placeholder `无` to fill the matrix.
- Evidence timestamps use a compact UTC display in the card. Machine-readable timestamps remain in the incident evidence and test fixtures.
- The data-source description must state whether the source is a native runtime snapshot, a scheduled quality patrol, or another existing read-only source. It must not imply that a new probe or write was performed.
- Existing redaction rules apply to every new structured field.

## Message Model

Add reusable recovery-specific types in `internal/notify`:

```go
type RecoveryMetric struct {
    Label string
    Value string
}

type RecoveryCardView struct {
    Title      string
    Summary    string
    Detail     string
    Metrics    []RecoveryMetric
    Basis      []string
    Source     string
    Focus      string
    Links      []Link
    Suppressed bool
}
```

`RenderRecoveryCard(RecoveryCardView)` is the only function that assembles the structured recovery elements. Existing `RenderRecovery(IncidentView)` remains as a compatibility entry point for callers outside the migrated recovery paths and falls back to the existing prose layout when no structured recovery view is supplied.

`CardElement` gains support for Feishu `div.fields`, represented as structured `CardText` values. The renderer emits:

- one summary `div`;
- one metrics `div` with two to four fields;
- one evidence `div`;
- one existing `action` element when links are usable.

The renderer does not parse semicolon-delimited result strings or infer fields from display text.

## Producer Mapping

Each producer translates its domain values before calling the renderer:

- Site runtime: Chinese metric name, current business state, healthy-window confirmation, observation window, evidence time, and native snapshot source.
- Account quality: multiplier or balance state, healthy-window confirmation, metric name, evidence time, and scheduled-patrol source.
- Native monitor: operational state, monitored model, latency when present, healthy-window confirmation, and native monitor source.
- Group availability: available/total accounts, recovered availability state, healthy-window confirmation, and group snapshot source.
- Usage session: normal session state, resumed cost verification, recovery observation time, and usage-page read source.
- Synthetic acceptance: recovered synthetic state, duplicate suppression result, no-real-service-impact statement, and synthetic acceptance source.

No producer invents a duration, latency, baseline, or timestamp that its source does not provide.

## Compatibility And Failure Handling

- Alert rendering is unchanged.
- `RenderedText` continues to flatten every text field in delivery order so App Bot and webhook fallback text cannot drift from the card.
- Relative and non-HTTPS action-link filtering remains unchanged.
- Card serialization keeps the existing 30 KiB limit.
- Empty or fully redacted metrics are omitted. If no usable structured metrics remain, `RenderRecovery` uses the existing prose recovery body instead of sending a blank matrix.
- Duplicate-suppression information remains visible for synthetic acceptance recovery.

## Testing

Tests must prove:

- the structured card serializes summary, `div.fields`, evidence, and action as separate elements;
- two, three, and four metric layouts contain no blank fields;
- structured fields are included by `RenderedText`;
- sensitive values are redacted in metrics and evidence;
- all six recovery producers supply business-language summaries and the correct source;
- group availability recovery uses the shared structured renderer;
- existing alert-card JSON remains unchanged;
- relative link filtering and the 30 KiB size limit still apply; and
- the full relay-ops Go suite, contract validation, and infrastructure baseline pass.

## Done When

- Every recovery notification path uses the shared structured recovery renderer.
- Recovery cards lead with a clear conclusion and two to four key metrics.
- No recovery card exposes internal enums as its primary user-facing wording.
- All required local verification passes.
- The feature branch is merged into `main` and its worktree is removed.
