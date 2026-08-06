# Account Monitor Card HTML Prototype Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a standalone minimum-sample HTML prototype for reviewing the narrowed account-monitor card before production implementation.

**Architecture:** Keep the prototype isolated under `docs/prototypes/account-monitor-card-v2/`. Store all sample data in the page, switch time windows client-side, and make no production API calls.

**Tech Stack:** Semantic HTML, CSS, vanilla JavaScript, Lucide browser icons.

## Global Constraints

- Do not modify the production Vue or Go implementation in this task.
- Do not show group rate multiplier or any revenue, cost, profit, reconciliation, or accounting UI.
- Use the same selected time window for requests, failures, performance metrics, score, and rank.
- Keep probe results separate from real request counts.
- Missing account multiplier contributes zero cost points but never removes the account from ranking.

---

### Task 1: Standalone account card prototype

**Files:**
- Create: `docs/prototypes/account-monitor-card-v2/index.html`
- Create: `docs/prototypes/account-monitor-card-v2/design-qa.md`

**Interfaces:**
- Consumes: the approved card hierarchy and the supplied account-monitor screenshots.
- Produces: a directly openable HTML file with working group tabs, time-window controls, and request-detail disclosure.

- [x] **Step 1: Create the semantic page structure and minimum sample data.**
- [x] **Step 2: Implement the approved visual hierarchy and responsive layout.**
- [x] **Step 3: Add working 24-hour, 7-day, and 30-day switching.**
- [x] **Step 4: Verify request/error and probe counts remain separate.**
- [x] **Step 5: Render desktop and mobile screenshots and complete design QA.**
