# Narra Test Station Implementation Plan

> Execute inline with executing-plans and test-driven-development. User already authorized implementation, merge, test-station push and deployment.

**Goal:** Mount Narra image workstation on the isolated test station, six-hour ephemeral sessions and admin access to uncleared images.
**Architecture:** Vendor pinned upstream Next/Go worker; site identity bridge; fixed site image gateway; dedicated queue database; interchangeable temporary file storage. Test-station-only release source and blue/green route promotion.
**Tech Stack:** Next.js, Prisma/PostgreSQL, Go, Sub2API Vue, Docker/Caddy.
**Spec:** docs/superpowers/specs/2026-10-10-narra-test-station-design.md

## Global Constraints
- No production server writes or origin pushes. Preserve root main.
- Sessions 6h; no history restored; admin can inspect live images.
- No Narra credits, auth accounts, custom gateways or public galleries.
- Reuse site billing unchanged; never retry uncertain generation submissions.
- Server temporary files with capacity bound, storage adapter and expiry cleanup.

## Tasks
- [ ] Vendor pinned source with license and provenance; verify upstream baseline relevant tests.
- [ ] Test site identity bridge and fixed-provider ingress rejection, then implement bridge and remove legacy routes.
- [ ] Test six-hour ownership/expiry/capacity behavior, then implement temporary storage and cleaner; admin live-image listing.
- [ ] Adapt Worker and UI for fixed gateway, ephemeral session and no credits/history; test generation and expiration.
- [ ] Add native authenticated workstation host page and isolated deployment, validate build and direct tests.
- [ ] Verify migration compatibility of merged base, push clean test-station main, deploy with readiness and rollback; verify public UI and drain.
