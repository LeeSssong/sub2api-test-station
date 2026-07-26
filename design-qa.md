# Design QA

- Source visual truth: `/var/folders/26/3qc7y_lx2s11df_9sh7dqg_40000gn/T/codex-clipboard-6193e098-4916-4681-ba94-6937b60505bb.png`
- Desktop implementation: `/Users/gongtengxinwen/Documents/sub2api搭建/.worktrees/homepage-screenshot-adjustments/output/playwright/homepage-final-desktop.png`
- Mobile implementation: `/Users/gongtengxinwen/Documents/sub2api搭建/.worktrees/homepage-screenshot-adjustments/output/playwright/homepage-final-mobile.png`
- Combined comparison: `/Users/gongtengxinwen/Documents/sub2api搭建/.worktrees/homepage-screenshot-adjustments/output/playwright/homepage-final-comparison.png`
- Desktop viewport: 1812 x 1050 CSS px, device pixel ratio 1
- Desktop pixels: source 2858 x 1346; implementation capture 1797 x 1041
- Mobile viewport: 390 x 844 CSS px, device pixel ratio 1
- Mobile pixels: implementation capture 375 x 812
- State: hero entry animation completed, dark theme, guest session

## Full-View Comparison

The annotated source asked for the title group to move substantially upward while the pitch and CTA remain toward the lower right. The implementation places the title at `y=441.75` and the pitch at `y=766.81`, producing a clear 325 px vertical diagonal. The previous desktop composition kept both groups near the lower edge.

The desktop hero has no horizontal overflow. The mobile layout resets the title padding to `0px`, keeps all hero controls within the viewport, and has no horizontal overflow.

## Focused Evidence

A separate crop was not required because the requested change affects the full hero composition and is readable at full-view scale. DOM measurements confirm:

- Desktop title padding: `288px`
- Desktop grid contract: `data-composition="raised-diagonal"`
- Mobile title padding: `0px`
- Desktop and mobile console errors: none
- Codex installer link: `https://codexapp.agentsmirror.com/`, `_blank`, `noopener noreferrer`

## Fidelity Review

- Fonts and typography: Existing font stack, weights, line heights, letter spacing, and wrapping are unchanged. The title remains readable and does not collide with the pitch.
- Spacing and layout rhythm: The desktop title is visibly raised; the right pitch remains anchored low. Mobile returns to the existing stacked rhythm.
- Colors and visual tokens: Existing dark palette, cyan status accent, and background signal treatment are unchanged.
- Image quality and assets: Existing logo and canvas signal rendering remain sharp and correctly framed.
- Copy and content: Hero copy is unchanged. The Codex installation label and destination now match the requested website.

## Comparison History

- Earlier P1: The title and pitch were both concentrated near the bottom, so the intended diagonal composition was too weak.
- Fix: Increased desktop `.hero-title` bottom padding from `clamp(4.5rem, 8vh, 6.5rem)` to `clamp(12rem, 28vh, 18rem)` while preserving the `980px` mobile reset.
- Post-fix evidence: Desktop title and pitch have a 325 px top-position separation; mobile has no overflow or overlap.

## Findings

No actionable P0, P1, or P2 findings remain.

## Primary Interactions

- Hero pointer entry animation reaches the visible final state.
- Homepage navigation and CTA links render with valid destinations.
- Documentation installer link renders with the requested destination and safe new-tab attributes.

final result: passed
