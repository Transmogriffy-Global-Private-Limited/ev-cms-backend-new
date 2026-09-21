# WI-20260921-customer-charger-rating-aggregate

Status: Complete
Owner: Codex
Started: 2026-09-21

## Outcome

Expose the current-CPO average overall session rating and contributing count on
existing full customer charger projections without materializing aggregate
state or widening charger visibility.

## Invariants

- Aggregate only `customer_ratings` rows with a non-null `session_id`, grouped
  by CPO and charger, from mandatory `overall_rating`.
- Select/authorize visible chargers first, then make one bounded grouped query
  for those IDs; map markers remain compact.
- Existing rating identity is immutable after creation. A later PUT changes
  only rating contents and `updated_at`; it does not rewrite charger or hub.

## Non-goals

No aggregate table/cache/counter/trigger/worker, new route, CPO aggregate,
hub aggregate, score distribution, rating sorting/filtering, frontend code,
migration, deployment, or HAL work.

## Verification

- Focused aggregate/projection/rating tests, `./src/customerauth`, `./src/cpo`,
  OpenAPI/runtime route parity, documentation verification, serialized full Go
  tests, vet, build, and whitespace checks passed.
- PostgreSQL aggregate and rating identity coverage is present but skipped in
  this checkout because `TEST_DATABASE_URL` is unset. It must run only against
  an explicitly selected disposable PostgreSQL database.
