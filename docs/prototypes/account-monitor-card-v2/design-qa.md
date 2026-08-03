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
- Clearing #113 now restores its dormant native `0.58×` multiplier while retaining its native `2026-09-01` expiry; this transition completed without a runtime error.
- Saving procurement cost for #207 preserves its missing native expiry and renders `有效期缺失 · 无法计算等效倍率`, without a fabricated effective multiplier.
- 7-day switching updated scores, ranks, request metrics, cost detail, and each call disclosure from the same account window. Group/all-site tabs updated selected state, labels, and group-summary visibility without resetting saved edits.
- Search and status filtering apply to both cards. Each call disclosure uses its own account-specific `aria-controls`. Per-card and all-card refresh controls update the visible checked-at time only.
- The non-functional `查看探测历史` footer control was removed; the remaining footer refresh control updates the checked-at timestamp.
- A fresh five-second concurrency check changed `3 / 10` to `4 / 10` and `1 / 8` to `0 / 8`; the measured card width and height did not change.

## Browser Results

- Desktop document scroll width and client width: 1440px / 1440px.
- Mobile document scroll width and client width: 390px / 390px.
- Desktop and mobile score, rank, priority, cost, and concurrency content stayed inside their containers. Inline priority and cost editors were operable at mobile width.
- Browser console: 0 errors, 0 warnings after adding the empty data favicon.

## Findings

- No actionable P0, P1, or P2 visual or interaction findings remain for this standalone V3 prototype.

## Final Review Fix Verification

- An injected unranked window now renders exactly `未排名`; it does not render an ordinal, `null`, or a rank denominator. Ranked account fixtures remain unchanged.
- At the 390 x 844 CSS viewport, the priority editor wraps and both save/cancel controls retain at least 31 x 31 CSS-pixel hit targets without horizontal document overflow.
- Operational helper text now meets the 4.5:1 WCAG AA contrast threshold on white plus every metric pastel surface.
- Rejected priority and procurement-cost drafts remain in their inputs. The edited input is refocused after validation, and its stable error element is associated through `aria-describedby` and announced with `role="alert"`.
- The selected time range exposes `aria-pressed="true"` on exactly one control. Switching to 7 days updates the score, request count, and failure count from that window.
- Fresh independent browser verification on 2026-08-04 recaptured `prototype-v3-desktop.png` at 1440 x 1000 and `prototype-v3-mobile-top.png` at 390 x 844. Both viewports had matching document scroll/client widths (1440 / 1440 and 390 / 390), desktop retained exactly two ranked cards in score order, mobile used one card column, and the browser console reported 0 errors and 0 warnings.
