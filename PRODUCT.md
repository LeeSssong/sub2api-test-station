# Product

## Register

product

## Users

The primary operator is a non-technical administrator preparing and running a small AI API relay for invited users. They need to understand current upstream health, whether controlled registration may open, and what action is required without interpreting internal IDs, hashes, deployment details, or duplicate configuration systems.

## Product Purpose

Provide a lightweight, reliable relay built on Sub2API. Reuse Sub2API for accounts, upstreams, scheduling, authentication, users, API Keys, usage, balances, and registration wherever those capabilities already exist. Add only the small operational projections, controlled-launch policy, monitoring, and Feishu notifications that Sub2API does not provide.

Success means the administrator can identify current truth and the next safe action quickly, while invited users receive a stable native Sub2API experience.

## Brand Personality

Quiet, precise, trustworthy. The interface should feel like a focused operations tool: plain language, restrained visual treatment, and strong evidence without ceremony.

## Anti-references

- Marketing-style dashboards with decorative metrics, oversized headings, gradients, or card grids.
- Interfaces that expose implementation details such as hashes and policy codes before the business decision.
- Duplicate control planes that ask administrators to re-enter Base URLs, Keys, accounts, groups, or registration state already owned by Sub2API.
- Dense forms for rare or unsupported workflows.
- Security states that reveal privileged routes or explain administrator-only behavior to unauthorized visitors.

## Design Principles

1. Prefer Sub2API native capability and keep one authoritative source of truth.
2. Lead with the business decision and next action; place technical evidence behind progressive disclosure.
3. Make privileged surfaces read-only by default and fail closed without exposing their existence.
4. Remove controls that are redundant, rarely needed, or safer in the native administration system.
5. Keep workflows short enough for a non-technical operator to use repeatedly without a runbook.

## Accessibility & Inclusion

Target WCAG 2.1 AA for contrast, keyboard access, semantic structure, status announcements, and responsive text. Do not rely on color alone. Support reduced motion and layouts that remain usable at 200% zoom and on narrow mobile screens.

