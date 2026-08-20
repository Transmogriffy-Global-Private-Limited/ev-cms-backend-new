# Customer Authentication Plan

Status: Implemented under ADR 0013; PostgreSQL lifecycle verification pending

## Objective

Provide the complete app-user credential and session boundary after CPO-scoped
signup, while keeping customer sessions separate from platform and CPO-staff
administrative sessions.

## Contract

Public operations under `/api/v1/app/auth`:

- password login start;
- email-OTP login verify and resend;
- refresh-token rotation;
- CPO/customer-scoped password recovery;
- existing signup start, verify, and resend.

Customer-session operations:

- current app-user identity (`me`);
- list and revoke this customer/CPO's sessions;
- current-session logout and customer/CPO-scoped logout-all;
- CPO-local customer password change.

## Invariants

- Customer sessions use dedicated customer auth tables and persist
  `customer_id` plus `cpo_id`; they never reference a global user or carry a
  CPO staff role.
- The app ID must resolve and continue to match the same active CPO, but it is
  public routing metadata rather than authentication.
- Login requires an active CPO-local customer account in that active CPO.
- Access tokens remain signed then encrypted; refresh tokens remain opaque,
  hashed, rotating, and reuse-detecting.
- Customer middleware revalidates the durable session, customer, CPO,
  and current app ID on every request.
- Customer session listing/revocation/logout-all is limited to the current
  customer relationship and does not expose or revoke administrative or
  cross-CPO sessions.
- Password change/reset revokes every session for only the exact CPO-local
  customer account.
- Backend handlers derive customer and CPO identifiers from the
  validated customer principal, never from request bodies or query parameters.
- A current login/reset challenge is unique by `(cpo_id, customer_id, purpose)`;
  its owner row serializes create, resend, verification, and reset paths.
- Customer password mutation locks that same customer row before testing the
  old credential, then updates password, revokes sessions, invalidates active
  challenges, and audits in one transaction.

## Backend Helper Contract

- `customerauth.CurrentPrincipal`
- `customerauth.CurrentUserID` (compatibility alias of `CurrentCustomerID`)
- `customerauth.CurrentCustomerID`
- `customerauth.CurrentCPOID`
- `customerauth.CurrentCPOAppID`
- `customerauth.RequireAppID`

## Implemented Corrective Slice

The forgot response remains generic. Each eligible customer reset email now
carries the opaque recovery ID, code, and expiry required by reset/resend, and
the lifecycle test consumes those recipient-visible values rather than reading
the challenge table.

The focused payload/renderer tests and full Go suite pass. The changed
PostgreSQL lifecycle is not marked verified in this slice because no explicitly
disposable `TEST_DATABASE_URL` was configured.

## Verification

- Token-context and helper unit tests
- Route protection and OpenAPI/runtime drift tests
- PostgreSQL lifecycle covering login OTP, `me`, refresh rotation/reuse,
  customer-scoped session management, password recovery/change, suspension,
  and tenant/app-ID mismatch
- Migration down/up/idempotent-up
- Full documentation, Go test, vet, and whitespace checks

Completed evidence:

- Migration 000005 passed down, up, and idempotent-up execution in PostgreSQL
  17.
- The historical global-customer lifecycle covered mail OTP login, encrypted
  access validation, `me`, refresh rotation/reuse revocation, session
  management, and recovery. The migration-twenty CPO-local lifecycle obtains
  recovery material from encrypted recipient mail but remains pending an
  explicit disposable `TEST_DATABASE_URL`.
- All 40 runtime/OpenAPI operations matched.
- Documentation verification, `go test ./...`, and `go vet ./...` passed.
