# WI-20260922-rating-discovery-and-history

Status: Complete in source; not deployed.

## Scope and invariants

- Customer and CPO charger lists use the one tenant-scoped grouped relation
  over `customer_ratings` before filtering, ordering, limiting, and keyset
  pagination. Only `overall_rating` rows with a non-null `session_id` count.
- `average_rating` is rounded to two decimals and omitted for unrated chargers;
  `rating_count` is always present. No aggregate is stored or cached.
- Customer `GET /api/v1/app/charging-session-ratings` proves the joined
  charging session belongs to the authenticated CPO-local customer. It is a
  durable database read, never a HAL/live read, and does not require current
  charger visibility.
- CPO customer-rating rows include bounded CPO-scoped session and connector
  context where a session exists. Nullable historical session rows retain no
  fabricated context.

## Cursor contract

The existing `before` + `before_id` pair remains the created-at traversal.
Rating-derived charger sorts and non-created customer rating sorts use
`cursor_value` + `cursor_id`, where the latter is the UUID tie breaker. For
average-rating sorting the literal cursor value `null` represents the final
unrated `NULLS LAST` segment. Hub charger lists intentionally remain unpaged.

## Verification boundary

Source/unit package checks, contract parity, documentation checks, broad Go
checks, vet, build, and diff checks are required before publication. PostgreSQL
integration checks require an explicitly configured disposable
`TEST_DATABASE_URL`; no production/development database is a test target.
