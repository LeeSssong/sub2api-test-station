# Homepage Full-Width Code Background Design

## Goal

Make the animated code signal background cover the complete homepage hero at every supported viewport width, matching the continuous visual density of the reference site without changing the existing hero content or layout.

## Root Cause

`HeroSignalCanvas` currently draws exactly two copies of each row string. The combined text width is smaller than a wide desktop canvas, so the right side of the hero receives no code glyphs even though the canvas itself fills the viewport.

## Design

- Measure each row string with the active canvas font.
- Generate enough repeated text to cover the viewport width plus one text-width of overflow on both sides.
- Keep each row's existing speed, direction, opacity range, vertical spacing, and signal vocabulary.
- Wrap each row by one measured segment so motion remains seamless without a visible jump.
- Recalculate coverage whenever the hero is resized.
- Preserve the grid, ambient color, content shade, title, navigation, calls to action, and reduced-motion fallback.

## Responsive And Accessibility Behavior

- Wide desktop, standard desktop, tablet, and mobile must all show code across the full hero width.
- Reduced-motion users continue to receive the existing static semantic fallback.
- The code remains decorative and hidden from assistive technology through the existing canvas treatment.

## Verification

- Add a focused canvas unit test that proves row repetition is derived from viewport width rather than fixed at two copies.
- Run the homepage test suite, type checking, and production build.
- Inspect desktop and mobile screenshots in a real browser.
- After deployment, verify the production bundle and capture the live hero at a wide desktop viewport.
