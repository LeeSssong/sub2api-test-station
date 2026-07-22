# Xingqiao Public Homepage Design

**Date:** 2026-07-22 (Asia/Shanghai)
**Status:** Approved

## Goal

Build a branded public homepage for Xingqiao that faithfully reproduces the visible layout, responsive behavior, and motion of `https://api.shuaiapi.com/` while keeping the existing Sub2API application, relay-ops pages, and API gateway intact.

The first screen must make the product position immediately clear:

```text
星桥
链接世界顶尖模型

韩国首尔节点，国内直连。
兼容 OpenAI 与 Anthropic API。
无需翻墙，只需更改基础 URL。
```

## Source Investigation

- The reference site identifies its bundle as `newapi-web` and links to `QuantumNous/new-api` in its footer.
- `QuantumNous/new-api` is public under AGPLv3, but the target homepage is not present in its six public branches, 675 tags, or 6,216 reachable commits as of this design date.
- The unique reference strings, `MODELOC` integration, `home-brand-reveal` classes, and homepage motion components do not appear in the public repository history or in indexed public forks.
- The reference site's `/api/home_page_content` is empty, so the page is compiled into its customized frontend rather than supplied as editable backend HTML.
- The public production bundle exposes enough runtime behavior to identify section order, scroll ranges, animation timing, responsive fallbacks, and interaction rules, but it is not a reusable source template.

The implementation will therefore be original project code. The downloaded reference bundles are behavior evidence only and must not be vendored, served, or made a runtime dependency.

## Chosen Approach

Use a standalone Vite and React static application for `/`, with Motion and Lenis providing the same classes of animation used by the reference page. Caddy serves the homepage and its assets directly. Every route outside the exact homepage surface continues to use its current owner.

Rejected alternatives:

- Port the New API frontend: its application state, router, backend APIs, and AGPL obligations do not match this Sub2API deployment.
- Start from a generic landing-page template: faster initially, but unable to meet the approved 1:1 visible-motion requirement.
- Serve the reference site's compiled bundle: coupled to another site's New API runtime, branding, configuration, and unpublished code.

## Scope

### Included

- Public homepage at `/`.
- Hashed assets under `/home-assets/*`.
- Xingqiao logo, favicon, copy, and QQ support identity.
- Guest, user, and administrator CTA states.
- Existing `/pricing` and `/monitor` navigation.
- Optional MODELOC reports with an administrator reminder on `/ops`.
- All visible reference-page motion, including desktop and mobile behavior.
- A subtle first-screen scroll cue because the continuation is otherwise easy to miss.
- Reduced-motion behavior and keyboard-accessible interactions.
- Caddy routing, cache policy, health checks, and rollback behavior.

### Excluded

- Replacing or restyling Sub2API login, registration, dashboard, model, usage, or administration pages.
- Replacing `/pricing`, `/monitor`, or `/ops`.
- Adding WeChat, Feishu, email, ticket, or telephone support to the public homepage.
- Creating a MODELOC report or claiming one exists before a real report URL is configured.
- Changing prices, user-group multipliers, upstream routing, registration state, balances, or payment behavior.
- Importing New API or the reference site's production bundle into the application.

## Route Ownership

| Request | Owner | Behavior |
| --- | --- | --- |
| `GET /` | Caddy static homepage | Serve `index.html` only when the validated homepage build exists; otherwise fall through to Sub2API. |
| `GET /home-assets/site-config.json` | Caddy static homepage | Serve runtime public configuration with revalidation/no-cache semantics. |
| `GET /home-assets/*` | Caddy static homepage | Serve hashed homepage JS, CSS, images, and fonts with immutable caching. |
| `/register`, `/login` | Sub2API | Existing public authentication flows. |
| `/dashboard`, `/admin/dashboard` | Sub2API | Existing authenticated destinations and route guards. |
| `/pricing` | relay-ops | Existing public pricing page. |
| `/monitor` | Sub2API | Existing status page and its current authentication behavior. |
| `/ops`, `/relay-ops/api/*` | relay-ops | Existing hidden administrator operations surface. |
| `/api/*`, `/v1/*`, all other routes | Existing services | Preserve current Caddy matchers, streaming settings, and proxy behavior. |

Caddy must use an existence matcher for the homepage file. A missing or invalid deployment must not turn `/` into a permanent 404 or affect any proxy route.

## Information Architecture

The homepage preserves the reference page's section order and composition while substituting Xingqiao content:

1. Hero: Xingqiao identity, domestic direct-connect statement, OpenAI and Anthropic compatibility, role-aware CTA, copyable API origin, and animated protocol path.
2. Core value section: domestic direct connection, transparent price presentation, and real model access.
3. MODELOC third-party report section, rendered only when at least one real report is configured.
4. Transparent pricing section.
5. Real model-channel section.
6. Korea/Asia network section emphasizing Seoul hosting, domestic direct access, and no VPN requirement.
7. QQ group support section.
8. Service-boundary section.
9. Large scroll-driven statement section.
10. “Follow one request” route visualization.
11. OpenAI and Anthropic integration section.
12. Footer and full-screen Xingqiao brand reveal.

When MODELOC is hidden, the adjacent sections close the space completely; no empty heading, card, divider, or skeleton remains.

## Navigation

The primary navigation is:

| Label | Destination |
| --- | --- |
| 首页 | `/` |
| 控制台 | Role-aware destination described below |
| 模型 | `/pricing` |
| 状态 | `/monitor` |
| 文档 | Homepage integration-section anchor |
| 关于 | Homepage service-boundary anchor |

Anchor navigation uses smooth scrolling when motion is allowed and immediate focus-preserving navigation when reduced motion is requested. The mobile navigation contains the same destinations and closes after selection.

## Hero And CTA States

The hero title and supporting copy use the exact approved text in the Goal section.

The API origin defaults to `window.location.origin` and is copyable. The adjacent terminal path cycles between:

```text
/v1/chat/completions
/v1/messages
```

CTA resolution:

| Session state | Label | Destination |
| --- | --- | --- |
| Guest | `立即获取密钥` | `/register` |
| Authenticated user | `前往控制台` | `/dashboard` |
| Authenticated administrator | `前往控制台` | `/admin/dashboard` |

The first viewport also contains a small animated downward arrow and short continuation cue. A controlled portion of the next section remains visible at common desktop and mobile viewport heights. The cue fades once the user begins scrolling and is not a second primary CTA.

## Authentication Data Flow

The static homepage interoperates with Sub2API's existing browser session rather than creating another login system.

1. Read the existing `auth_token`, `refresh_token`, and `auth_user` local-storage entries for an immediate cached state.
2. Validate the access token with `GET /api/v1/auth/me` using the existing Bearer token contract.
3. On a single 401 response, call `POST /api/v1/auth/refresh` once when a refresh token exists, update the same Sub2API storage keys, and retry `/auth/me` once.
4. Resolve the CTA from the validated `role: "admin" | "user"` response.
5. With no token or a definitive authentication failure, resolve to guest.
6. On a transient network failure, retain the cached state. The destination remains protected by Sub2API's native route guards.

Tokens must never appear in query strings, HTML, logs, analytics, error messages, or homepage configuration. The homepage exposes no privileged data; authentication only selects a label and navigation destination.

## Public Configuration

`/home-assets/site-config.json` is a runtime public configuration file with a versioned schema. It contains no secrets.

```json
{
  "version": 1,
  "apiOrigin": "",
  "support": {
    "qqGroup": "1080152144"
  },
  "thirdPartyReports": []
}
```

- Empty `apiOrigin` means the current browser origin.
- The QQ group renders as `1080152144` with a copy action and success feedback. The page does not depend on a temporary QQ join URL.
- Every report entry requires a stable ID, provider, title, HTTPS URL, and status. Description is optional.
- Invalid, non-HTTPS, or incomplete report entries are ignored.
- A missing or malformed file falls back to the built-in origin, QQ group, and empty report list.

## MODELOC Lifecycle And Reminder

The initial report list is intentionally empty, so the public MODELOC section is hidden.

`/ops` reads the same public configuration and displays a persistent, non-blocking reminder when no valid MODELOC report exists. The reminder must say that public evidence is not yet configured and provide the exact configuration filename or operational update instruction without exposing a filesystem secret path.

Once a valid MODELOC report is added:

- the homepage displays the third-party report section on its next configuration load;
- the `/ops` reminder disappears automatically;
- no source-code change is required;
- the report opens in a new tab with `noopener noreferrer`.

Failure to load MODELOC itself never blocks the homepage because the site only links to the report and does not embed remote scripts or content.

## Pricing Copy

The pricing section displays this approved fixed public copy:

```text
官方价格　100%
星桥价格　官方价格的 0.1–0.3 倍
额度换算　1 元 = 1 美元额度
```

This copy is deliberately static and is not derived from the current Sub2API user-group multiplier. The production user groups are currently recorded as `1.0x`; the user explicitly approved showing `0.1–0.3 倍` without a “future price” qualifier. This homepage increment does not alter billing configuration.

## Product Claims And Service Boundaries

The domestic-access section states that the service uses a Seoul, South Korea node, supports domestic direct access, and does not require a VPN. It must not claim a mainland-China server or a guaranteed latency value.

The service-boundary section contains three groups:

### Capacity protection

- Under high load, Xingqiao may lower concurrency, add upstream capacity, pause registration, or move to invitation-only access.
- Existing-user stability takes precedence over unlimited enrollment or throughput.

### Reasonable use

- Normal interactive coding and model usage is supported.
- Automated quota consumption, artificial load tests, and abusive scripted concurrency may be throttled, reduced, or suspended.
- Enforcement is progressive and communicated through the available support channel; it is not described as an invisible ban.

### Incidents and responsibility

- Official outages, policy changes, and upstream account actions are outside Xingqiao's direct control.
- Xingqiao will switch available capacity and communicate known incidents as quickly as practical.
- The homepage does not promise refunds, compensation, service credits, or an absolute uptime guarantee.

Only QQ group support is presented publicly.

## Brand Assets

Use the previously approved Xingqiao assets:

- `output/imagegen/xingqiao-brand/xingqiao-logo-80.png` for the header and small icon contexts.
- `output/imagegen/xingqiao-brand/xingqiao-logo-master.png` as the high-resolution source for derived web sizes.
- The site icon set is derived from the approved logo and includes browser favicon sizes and an Apple touch icon.
- The QQ avatar files are retained as community assets but are not substituted for the site logo.

The reference site's SHUAI/HLOOL branding, labels, and logo-motion media must not appear in the Xingqiao build.

## Motion Specification

All visible reference-page motion is a hard acceptance target, not an optional polish pass.

### Required motion inventory

- Initial hero overlay and reveal sequence.
- Masked and staggered hero typography, support copy, CTA, and endpoint entry.
- OpenAI/Anthropic terminal typing cycle and cursor blink.
- Lenis-style smooth scrolling.
- Intersection-driven fade, fade-up, fade-in, mask-rise, and staggered card entrances.
- Hover elevation, border/color response, icon movement, navigation response, and copy-success feedback.
- Scroll-driven word-by-word large statement reveal.
- Desktop sticky “Follow one request” scene across an extended scroll range.
- Request-line drawing, outbound and return dots, gateway emphasis, provider-node activation, descriptive phase changes, and latency/token count-up.
- Integration flow lines and staggered model chips.
- Full-screen footer brand reveal with the word `星桥`.
- Canvas cell displacement, pointer-driven disturbance, decay, and return-to-origin behavior in the brand reveal.
- Scroll-linked skew/settling behavior around the brand reveal.
- Logo hover motion using Xingqiao-compatible media or an original equivalent derived from the approved mark.
- Added first-screen continuation cue and its scroll-triggered dismissal.

Animation ranges, easing, delays, and responsive thresholds are reconstructed from observed reference behavior and its runtime evidence. They are validated at fixed scroll progress rather than judged only by freehand viewing.

### Responsive motion

- Desktop retains sticky long-scroll sequences when the viewport and motion preference allow them.
- Mobile follows the reference site's touch-friendly fallback: the request phases become a readable sequential presentation instead of forcing the desktop sticky timeline.
- Pointer-only effects do not become required interactions on touch devices.
- Fixed-format diagrams and counters have stable dimensions so animated content cannot reflow surrounding sections.

### Reduced motion

When `prefers-reduced-motion: reduce` is active:

- all content is immediately visible in its final state;
- smooth scrolling, typing loops, sticky progress timelines, canvas displacement, and continuous decorative motion are disabled;
- copy, links, buttons, focus behavior, and section order remain complete;
- no blank opacity-zero content waits for an observer or animation event.

## Visual And Responsive Requirements

- The page must preserve the reference layout, section rhythm, card geometry, borders, typography hierarchy, and light/dark behavior while replacing brand-specific text and imagery.
- Intentional brand and copy substitutions are excluded from pixel-equivalence comparisons; their containers, alignment, and responsive behavior are not.
- The hero always leaves a visible hint of the next section on common mobile and desktop viewports.
- No text overlaps, clipped controls, unintended horizontal scroll, viewport-width font scaling, or animation-driven layout shift is accepted.
- Interactive icons have accessible names or tooltips; copy actions announce success without moving the layout.
- Keyboard focus is visible and section anchors transfer focus sensibly.
- Semantic headings, landmarks, link destinations, contrast, and form-free navigation remain usable without animation.

## Failure Handling

- Missing homepage build: Caddy falls through to the existing Sub2API root.
- Missing hashed asset: return an asset error without changing API or application routing.
- Missing or malformed public configuration: render built-in core content, show QQ group `1080152144`, and hide third-party reports.
- `/auth/me` unavailable: keep the cached CTA state, or guest when no cache exists.
- Refresh failure: clear only invalid homepage session assumptions and show the guest CTA; native application routing remains authoritative.
- Clipboard API unavailable: select or expose the value for manual copying without hiding it.
- MODELOC unavailable: the external report link may fail independently; homepage rendering is unaffected.
- Canvas unavailable: render the static Xingqiao brand-reveal word and background.
- IntersectionObserver unavailable: render content in final visible states.

## Security And Privacy

- Homepage assets contain no upstream credentials, administrator keys, report-service secrets, or private operations data.
- No third-party script, tracker, iframe, or remote font is required for initial rendering.
- External links use safe target attributes.
- Runtime configuration is parsed as data, never inserted as unsanitized HTML.
- Existing Caddy security headers remain in effect; homepage-specific content security policy must allow only the resources actually used.
- The homepage does not weaken Sub2API authorization or add a parallel user/session store.

## Test Strategy

### Unit tests

- Public configuration parsing and fallback behavior.
- Report validation and empty-list hiding.
- Guest, user, administrator, token-expiry, single-refresh, and refresh-failure state resolution.
- Role-aware CTA labels and destinations.
- API origin resolution and both protocol paths.
- QQ copy behavior and clipboard fallback.
- Reduced-motion state selection.

### Browser flow tests

- Guest CTA reaches `/register`.
- User CTA reaches `/dashboard`.
- Administrator CTA reaches `/admin/dashboard`.
- Navigation reaches `/pricing`, `/monitor`, documentation, and about destinations.
- MODELOC absence leaves no visual gap; a configured fixture renders a safe external report link.
- Desktop and mobile navigation, copy feedback, anchor focus, and reduced-motion flows work without console errors.

### Caddy route-contract tests

- `/` resolves to the static homepage only when the validated build exists.
- `/home-assets/*` uses the expected cache behavior.
- Authentication, dashboard, pricing, monitor, ops, API, SSE, and all existing reverse-proxy routes retain their owner and response semantics.
- The exact homepage match does not intercept `/home`, `/login`, `/register`, or arbitrary SPA paths.

### Visual and motion tests

- Capture the reference and Xingqiao pages at `1440x900`, `1920x1080`, and `390x844` plus any breakpoint-edge viewports found during implementation.
- Compare full-page and first-viewport screenshots with intentional logo/copy regions masked only for content, not geometry.
- Capture fixed frames for initial entry and predetermined scroll progress through every scroll-driven scene.
- Record complete desktop and mobile scroll runs to inspect continuity, sticky release points, and transition ordering.
- Perform canvas pixel checks so the brand reveal cannot pass while blank.
- Exercise pointer disturbance and verify visible decay back to the undisturbed word.
- Check bounding boxes and document scroll width to reject overlap and horizontal overflow.

### Performance checks

- Production build succeeds with no missing assets.
- Desktop primary motion has no persistent visible jank under the standard test machine and browser profile.
- Mobile does not use the desktop long-scroll scene when its fallback is required.
- Below-the-fold work is delayed until needed, while the hero and its fonts/logo render without a prolonged blank state.

## Deployment And Rollback

1. Build and test the homepage independently.
2. Validate that `index.html`, hashed assets, logo derivatives, and `site-config.json` exist.
3. Run the route-contract and local-browser acceptance suites before reloading Caddy.
4. Preserve the last known-good static build.
5. Deploy the new versioned asset directory, then update the homepage entry atomically.
6. Reload Caddy only after configuration validation.
7. Run public smoke tests for `/`, `/register`, `/dashboard`, `/pricing`, `/monitor`, `/ops`, `/health`, and representative API/SSE routes.
8. If homepage smoke tests fail, restore the previous entry/build. If the homepage build is absent, Caddy falls through to Sub2API by design.

Deployment of this homepage must not mutate registration mode, account scheduling, prices, multipliers, balances, keys, users, routing, or relay-ops evidence.

## Acceptance Criteria

- The first viewport reads `星桥 / 链接世界顶尖模型` and explicitly states Seoul hosting, domestic direct connection, OpenAI and Anthropic compatibility, and no VPN requirement.
- Guest, user, and administrator CTAs display and route exactly as approved.
- The API origin copies successfully and the path visibly cycles between `/v1/chat/completions` and `/v1/messages`.
- The navigation destinations and homepage anchors work on desktop and mobile.
- The first-screen cue and visible next-section edge make continuation discoverable without competing with the primary CTA.
- The page displays the fixed `100% / 0.1–0.3 倍 / 1 元 = 1 美元额度` pricing copy.
- Only QQ support is shown, using group `1080152144` with a working copy action.
- Empty MODELOC configuration hides the entire public module and produces the `/ops` reminder; a valid report reverses both outcomes automatically.
- All required reference motions are implemented and pass fixed-progress visual review on desktop and the approved mobile fallback.
- The Xingqiao Canvas brand reveal is nonblank, interactive on pointer devices, and stable after decay.
- Reduced-motion users receive all content without continuous or scroll-scrubbed animation.
- No tested viewport has incoherent overlap, clipped controls, unintended horizontal overflow, or animation-driven layout shift.
- Homepage failure cannot break Sub2API authentication, dashboards, API traffic, pricing, monitoring, or operations routes.
- No reference-site brand asset, downloaded production bundle, or unpublished runtime code ships with the Xingqiao homepage.
