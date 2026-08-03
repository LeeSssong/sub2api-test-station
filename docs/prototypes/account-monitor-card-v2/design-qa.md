# Account Monitor Card Prototype V3 Design QA

Final result: passed

## Evidence

- Desktop screenshot: `prototype-v3-desktop.png` from a 1440 x 1000 CSS viewport.
- Mobile top screenshot: `prototype-v3-mobile-top.png` from a 390 x 844 CSS viewport.
- The desktop screenshot shows one seven-field native group summary, with accounts `#113` and `#207` in rank 1 then rank 2 order on one row.
- Account `#113` renders the procurement-cost mode as `¥120.00` and account `#207` renders the upstream-managed multiplier mode as `0.62×`.
- Both cards render tabular current concurrency as `current / max`.

## Layout And Content

- The group summary contains exactly: status, platform, group multiplier, RPM limit, native account count, native active account count, and native rate-limited account count. It is hidden by the all-site tab.
- Desktop uses a stable two-column account grid. At 390px, the cards switch to a single column; the document scroll width and client width were both 390px.
- The V3 scope keeps group multiplier exclusively in the group summary. It does not appear in either account card.
- Real-request failures and independent probe failures remain separately disclosed for each card.

## Interaction Checks

- Priority pencil opened an inline integer editor. Saving `4` updated only the in-page priority value; invalid values retain the editor and show the required validation message. Cancel restores the stored value.
- The multiplier-mode account opened an inline procurement-cost editor. Saving `50` rendered procurement mode with the displayed effective date; confirmation-based clear restored `0.62×` upstream-managed multiplier mode.
- 7-day switching updated scores, ranks, request metrics, cost detail, and each call disclosure from the same account window. Group/all-site tabs updated selected state, labels, and group-summary visibility without resetting saved edits.
- Search and status filtering apply to both cards. Each call disclosure uses its own account-specific `aria-controls`. Per-card and all-card refresh controls update the visible checked-at time only.
- A fresh five-second concurrency check changed `3 / 10` to `4 / 10` and `1 / 8` to `0 / 8`; the measured card width and height did not change.

## Browser Results

- Desktop document scroll width and client width: 1440px / 1440px.
- Mobile document scroll width and client width: 390px / 390px.
- Desktop and mobile score, rank, priority, cost, and concurrency content stayed inside their containers. Inline priority and cost editors were operable at mobile width.
- Browser console: 0 errors, 0 warnings after adding the empty data favicon.

## Findings

- No actionable P0, P1, or P2 visual or interaction findings remain for this standalone V3 prototype.
