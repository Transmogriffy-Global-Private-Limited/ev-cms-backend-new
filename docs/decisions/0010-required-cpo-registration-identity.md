# ADR 0010: Require Complete CPO Registration Identity

Status: Accepted

Date: 2026-08-03

## Context

CPO creation previously allowed a missing GSTIN and blank address, city, state,
and pincode values. Profile replacement could also clear those values. That
made the platform-owned tenant registration record incomplete even though the
SuperAdmin frontend needs the same fields for onboarding and later support.

The schema already has normalized unique indexes for slug and GSTIN. A frontend
availability check is useful for form feedback, but a read followed by a create
cannot reserve a value or remove the concurrency race.

## Decision

- CPO creation and platform profile replacement require nonblank GSTIN,
  address, city, state, and pincode values.
- GSTIN is trimmed, uppercased, exactly 15 alphanumeric characters, and
  globally unique through the existing normalized PostgreSQL index.
- Slug remains trimmed, lowercased, format-validated, and globally unique
  through the existing normalized PostgreSQL index.
- Migration eleven makes GSTIN `NOT NULL`, removes empty-string defaults from
  address fields, and adds nonblank database checks. It fails closed when
  incomplete legacy CPO rows exist instead of inventing legal or address data.
- `GET /api/v1/platform/cpos/slug-availability` gives an authenticated platform
  client a read-only snapshot for a normalized candidate.
- The availability response is advisory and does not reserve the slug. CPO
  creation and the unique index remain authoritative and can still return
  `cpo_conflict`.

## Consequences

SuperAdmin creation and profile forms must always submit the complete
registration snapshot. CPO and tenant-organization responses always contain a
GSTIN plus all address fields. Existing incomplete databases require an
explicit, human-reviewed data correction before migration eleven can apply.

The frontend can debounce and cancel availability requests for responsive
validation, but it must keep final conflict handling. No lock, reservation
table, cache, or background cleanup process is added.

## Rejected Alternatives

- Backfilling placeholder GSTIN/address data: rejected because the backend
  must not fabricate legal or tenant identity data.
- Relying only on HTTP validation: rejected because imports, scripts, and future
  internal callers must preserve the same durable invariant.
- Treating the availability GET as a reservation: rejected because that needs
  expiry, ownership, cleanup, and additional race semantics without a current
  product requirement.
- Case-sensitive uniqueness: rejected because the server normalizes both
  identifiers and equivalent casing must not create distinct CPOs.
