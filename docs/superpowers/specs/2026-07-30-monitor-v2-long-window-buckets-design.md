# Spec: Monitor V2 Long-Window Buckets

## Problem

Monitor V2 requests a 6-hour bucket for the 7-day window and a 24-hour
bucket for the 30-day window. The ops trend repository accepts only
60-second, 300-second, and 3600-second buckets. It silently normalizes the
two long-window bucket sizes to 60 seconds, producing 10,081 points for a
7-day request. Monitor V2 then rejects the response because its contract
allows at most 64 timeline points.

## Goal

Make the existing throughput and error trend repository paths preserve
21,600-second and 86,400-second bucket sizes so Monitor V2 produces bounded
7-day and 30-day timelines.

## Non-goals

- Do not change the Monitor V2 API contract or its 64-point limit.
- Do not change the frontend fallback behavior.
- Do not change the existing 1-minute, 5-minute, or 1-hour trend behavior.
- Do not deploy or mutate production as part of this code fix.

## Design

Use one repository-level bucket normalization helper for both throughput and
error trends. The supported set becomes 60, 300, 3600, 21600, and 86400
seconds; unsupported values continue to fall back to 60 seconds for backward
compatibility.

For 6-hour and 24-hour buckets, group PostgreSQL timestamps by flooring Unix
epoch seconds to the requested interval. This keeps bucket boundaries aligned
to UTC and reuses the repository's existing fill logic and bucket labels.

PostgreSQL's inclusive `generate_series(start, end, bucket)` can expose one
partial bucket at each edge of an unaligned window, yielding 29 or 31 merged
timeline points. After merging throughput and error trends, trim only the
oldest point when the result is exactly one point above the calculated window
capacity. Keep the newest/current bucket. Do not trim grossly oversized
results, so the existing 64-point guard still catches a future bucket fallback.

## Test Strategy

Exercise the real `opsRepository.GetThroughputTrend` and
`opsRepository.GetErrorTrend` paths with `sqlmock`. Empty database results
must still produce:

- 28 six-hour points for a 7-day range and a `6h` bucket label.
- 30 twenty-four-hour points for a 30-day range and a `24h` bucket label.

The SQL expectations must require the requested bucket size, so silently
falling back to 60 seconds cannot satisfy the tests.

## Acceptance Criteria

- [x] Throughput trends preserve 21,600-second and 86,400-second buckets.
- [x] Error trends preserve 21,600-second and 86,400-second buckets.
- [x] A 7-day empty trend contains 28 points, not 10,081 points.
- [x] A 30-day empty trend contains 30 points.
- [x] Snapshot timelines remove only the single inclusive boundary overflow
  and retain their newest bucket.
- [x] Existing Monitor V2 and repository tests pass.

## Risks

The SQL bucket expressions must use the same interval for grouping, filling,
rate denominators, and response labels. Central normalization and
consumer-level tests keep those values synchronized.
