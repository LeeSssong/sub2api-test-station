# Multiplier Alert Copy Design

**Date:** 2026-07-28 (Asia/Shanghai)
**Status:** Approved

## Goal

Make account multiplier-change alerts describe the event accurately. A
multiplier is pricing/billing evidence, and the stored comparison value is the
previous observation rather than a health baseline.

## Behavior

- Title multiplier incidents as account billing multiplier changes, not
  upstream account quality alerts.
- Label the comparison value as the previous record instead of a baseline.
- Explain that the system only detected a change and did not change routing or
  account configuration.
- Direct the administrator to confirm whether the change matches upstream
  pricing or account configuration.
- Do not render the generic `/ops` action for multiplier incidents because the
  page has no multiplier evidence drill-down.
- Leave non-multiplier incident cards unchanged.

## Scope

Change only relay-ops multiplier incident rendering and its tests. Do not
change multiplier collection, comparison, incident state, notification
delivery, account scheduling, routing, pricing, or credentials.

## Validation

Tests must prove that a multiplier incident:

- uses the billing-multiplier title and plain-language labels;
- identifies the previous and current values;
- states that no automatic configuration change occurred;
- does not claim account-quality degradation;
- does not include the generic operations-console action.

Existing tests must continue to prove that other incident domains retain their
current copy and actions.
