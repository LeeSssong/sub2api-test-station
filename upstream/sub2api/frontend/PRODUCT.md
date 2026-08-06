# Product

## Register

product

## Users

This is an internal operations console for the Xingqiao API relay. Its primary user is the operator who monitors the whole account pool, compares real service and financial performance, adjusts scheduling, and investigates reconciliation exceptions during frequent production iterations.

## Product Purpose

The console must make the real state of the full site, real `/admin/groups` groups, and every upstream account directly observable and actionable. Success means the operator can distinguish operating data from service data, understand account scoring without internal jargon, find every account without hidden filtering, and complete monitoring, history, reconciliation, pricing, and alerting workflows from the native admin product.

## Brand Personality

Quiet, direct, evidence-based. The interface should feel like a dependable operations tool: compact enough for repeated scanning, explicit about data coverage and sample size, and restrained about secondary controls.

## Anti-references

- Marketing-style dashboards with oversized hero metrics or decorative cards.
- Dense multi-column account walls that make individual accounts hard to inspect.
- Separate relay-ops pages that duplicate the native admin workflow.
- English database or engineering terms exposed to Chinese-speaking operators.
- Scores or statuses presented without their calculation scope, sample count, or data freshness.

## Design Principles

1. Use authoritative business data. Group navigation comes from `/admin/groups`; monitoring projections enrich those groups but never redefine them.
2. Separate operating truth from service quality. Revenue, cost, account profit, and reconciliation belong together; availability, latency, probes, and scores form a distinct service view.
3. Optimize for operational scanning. Account cards use at most two columns, primary evidence stays visible, and secondary controls remain compact or collapsed.
4. Explain evidence in plain Chinese. User-facing labels avoid internal terminology and state the number of calls or probes behind a metric.
5. Preserve complete workflows. Every visible action must have loading, empty, failure, and successful states and must be verified in the production UI.

## Accessibility & Inclusion

Maintain clear keyboard focus, semantic buttons and dialogs, readable contrast, reduced-motion alternatives, and text labels in addition to color for status. Layout and text must remain usable on mobile and desktop without overlap or truncating critical values.
