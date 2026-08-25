# ADR 0015: Validate CPO Legal Identity as GSTIN, State, and PIN Invariants

Status: Accepted

Date: 2026-08-25

## Context

ADR 0010 made GSTIN and CPO address data mandatory and preserved normalized
global GSTIN uniqueness. It did not validate the Indian GSTIN checksum, connect
the GSTIN state code to the stored registration state, or restrict pincode to a
six-digit Indian PIN code. As a result, malformed legal identity could be
accepted by the API or written directly through another database path.

## Decision

- CPO create and profile replacement normalize GSTIN uppercase and validate its
  Indian structural form, checksum, and state code.
- The GSTIN state code must match the selected CPO registration state. The
  state-code mapping is explicit in application code and PostgreSQL.
- Pincode is exactly a six-digit Indian PIN code.
- CPO business names/addresses must contain meaningful text; city and
  administrator names must contain a letter. Input whitespace is collapsed for
  consistent display and storage.
- Migration fifty-three preflights existing CPO records and then adds immutable
  PostgreSQL GSTIN/checksum, GSTIN-state, and PIN constraints. It does not
  invent or repair legal data.
- `uq_cpos_gstin_normalized` remains the only CPO GSTIN uniqueness guard. A
  `(gstin, business_name)` uniqueness key is redundant because globally unique
  GSTIN already implies uniqueness of that pair.

## Consequences

Create/profile requests can return `invalid_gstin`,
`invalid_gstin_state_mismatch`, or `invalid_pincode` before persistence. Direct
database writes receive the equivalent durable check-constraint protection.
Databases containing prior malformed CPO identity data must be corrected from
an authoritative source before migration fifty-three can apply.

The platform validates an identifier, not ownership of a legal business name.
Verifying that a GSTIN belongs to the supplied business name requires an
authorized GST registry integration and remains intentionally unsupported.

## Rejected Alternatives

- A redundant compound `(gstin, business_name)` unique index: rejected because
  global GSTIN uniqueness already proves the compound pair unique.
- A business-name equality check from local data: rejected because the CMS has
  no authoritative GST legal-name registry.
- HTTP-only validation: rejected because direct writes and future callers must
  preserve the same durable invariant.
