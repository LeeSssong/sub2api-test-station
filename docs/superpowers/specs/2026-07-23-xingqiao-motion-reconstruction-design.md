# Xingqiao Homepage Motion Reconstruction Design

**Date:** 2026-07-23 (Asia/Shanghai)
**Status:** Approved for direct implementation

## Goal

Rebuild the Xingqiao public homepage motion system so it delivers the same visible categories, pacing, and responsive behavior as the observed Shuai API homepage while using original React, Motion, canvas, copy, and Xingqiao brand assets.

## Evidence Boundary

The public reference's custom homepage bundle and live page were inspected for behavior. Its compiled JavaScript, CSS, logos, and WebP animation assets are evidence only. No reference source, binary asset, class names, or brand material may ship in Xingqiao.

The observed reference has five coordinated motion layers:

1. A first-view canvas signal field and pointer-origin entrance reveal.
2. Reusable viewport reveals, a blinking endpoint cursor, and a scroll-revealed routing mini-flow.
3. A word-level statement driven by scroll progress.
4. A desktop-only, three-phase sticky request sequence with line drawing, traveling dots, node activation, and count-up metrics.
5. A footer canvas whose image cells disperse in response to pointer movement and decay at rest.

## Architecture

`HeroSignalCanvas` owns the live first-view background. It creates original API-themed rows from a bounded local alphabet, advances them by per-row velocity, applies pointer proximity as an opacity/speed disturbance, and pauses with `IntersectionObserver`. It renders a static text fallback whenever reduced motion is requested.

`useHeroEntry` owns the first-view choreography. It starts once from pointer/focus/keyboard/scroll intent, stores the local pointer origin in CSS custom properties, and exposes a deterministic final state for reduced motion. CSS renders the ambient/grid reveal, circular ripple, and staggered content entry.

`Reveal` becomes an animation-variant observer rather than one fixed fade. A shared contract accepts `animation`, `delay`, `threshold`, and `once`, supports `fade-up`, `fade-in`, `scale-in`, `mask-rise`, and `rule-line`, and uses `rootMargin: 0px 0px -40px 0px`.

The existing Motion dependency drives scroll-linked sections. `RequestJourney` remains its own component, but its desktop layout uses a 340svh root and 100svh sticky stage. Each phase is interpolated directly from `scrollYProgress`; React state is used only for accessible phase labels, never to animate positions. The mobile and reduced-motion layouts retain readable sequential content and static final metrics.

## Component Contract

| Unit | Responsibility | Key contract |
| --- | --- | --- |
| `HeroSignalCanvas` | Canvas lifecycle, DPR sizing, pointer and visibility pause | `active` status exposed with `data-canvas-active` |
| `useHeroEntry` | One-time hero start state and pointer origin | `{ started, origin, start }` |
| `HeroSection` | Hero composition and page-scroll parallax | preserves current copy and CTA behavior |
| `EndpointDemo` | Endpoint type cycle and terminal cursor | endpoint remains copyable and accessible |
| `Reveal` | Section-level entry variants | exact `animation` union and observer options |
| `ModelFlow` | Scroll fill, dashed energy, staggered model chips | preserves six provider labels |
| `RequestJourney` | Scroll narrative | deterministic desktop/mobile/reduced-motion branches |
| `BrandReveal` | Original brand-cell canvas | retains static `星桥` heading fallback |

## Motion Specification

### Hero

- Initial state: dark ambient and grid layers are masked; content is hidden 28px below final position.
- Start: after 1.4s on fine pointers, 180ms on coarse pointers, or immediately after an intentional pointer/focus/keyboard/scroll event.
- Sequence: header drops over 560ms, ambient/grid layers reveal over 880ms, ring expands/fades over 920ms, and endpoint/headline/copy/CTA rise over 680ms with 120ms steps.
- While the hero scrolls away: content translates upward by up to 96px, fades to zero near 72% of the hero range, background moves up to 120px, and content scales down to 0.94.
- The live text canvas stays background-only, renders no user data, caps DPR at 2, and stops requestAnimationFrame when out of view.

### Surface And Flow

- Status dot pulses every second.
- Endpoint paths type/erase in a 5.056s cycle and retain a 1s blinking cursor.
- Model-flow fill uses local section progress (`start 0.92`, `end 0.45`); its dash travels every 2.8s and its six chips pulse over 3.6s with 450ms stagger.
- Cards enter once using the shared reveal variants. Hover behavior remains small: border/color response and at most a 2px lift.

### Scroll Scenes

- The statement exposes individual phrase tokens and maps each to a 20% local scroll window, opacity `0.15 -> 1`, y `18 -> 0`.
- Desktop request journey uses section progress to sequence outbound line 6-26%, outbound dot 7-26%, route line 36-52%, provider lighting 52/58/64%, return line 68-78%, and final metrics 70-92%. It remains sticky for 340svh.
- Mobile below 680px and reduced motion show a static, readable sequence with all providers and completed metrics. No sticky scroll trap is used.
- Brand cells derive only from an offscreen canvas rendering `星桥`; pointer velocity displaces nearby cells and decays by 0.88 per frame. Animation pauses while offscreen.

## Accessibility And Performance

- `prefers-reduced-motion` removes automatic animation, canvas loops, sticky presentation, smooth scrolling, and transient hidden content.
- Canvas elements are `aria-hidden`; semantic headings and text remain visible in DOM.
- Touch devices avoid pointer-dependent critical behavior.
- Every requestAnimationFrame, timer, listener, and observer is cleaned up.
- Canvas memory is bounded by its element dimensions and a maximum DPR of 2.

## Validation

- Component tests assert final static/reduced states, entry data attributes, canvas fallback, exact provider labels, and request phases.
- Browser checks verify the desktop hero enters, its text field changes between frames, model flow advances, the request scene responds to fixed scroll positions, and mobile avoids sticky desktop framing.
- `npm run test:run`, `npm run typecheck`, and `npm run build` must pass before handoff.
