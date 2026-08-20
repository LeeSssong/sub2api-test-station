# T47 Monitor Visual Redesign Implementation Plan

Goal: Match the supplied competitor visual language on the custom performance monitor while preserving Monitor V2 data behavior.

Architecture: Keep MonitorV2View as the route-level composition and refactor only its visual shell, MonitorV2GroupCard, MonitorV2Timeline, and the sidebar virtual menu icon. No API or state changes.

Tech Stack: Vue 3, TypeScript, Tailwind utility classes, Vitest, Vue Test Utils.

Spec: docs/superpowers/specs/2026-08-21-t47-monitor-visual-redesign.md

Global constraints:
- Preserve Monitor V2 contract version 7 and 24h|7d|30d windows.
- Preserve optimistic status and stale-snapshot refresh behavior.
- No migration, API, config, or production data changes.
- Keep timeline inner scrolling only on narrow screens.
- Retain keyboard focus and ARIA labels.

Tasks:
1. Add the sidebar pulse-monitor icon and compact page shell, with failing tests first.
2. Rework the service-line card into compact left metrics and right timeline, with a component test.
3. Rework the timeline into dense vertical bars while preserving tooltip and accessibility behavior.
4. Run focused tests, typecheck, build, diff-check, visual smoke checks, and write the handoff.
