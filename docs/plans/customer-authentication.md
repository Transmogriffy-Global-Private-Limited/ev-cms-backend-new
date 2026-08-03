# Customer Authentication Plan

Status: Implemented

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
- global-identity password change.

## Invariants

- Customer sessions use a distinct `CUSTOMER` scope and persist `customer_id`
  plus `cpo_id`; they never carry a CPO staff role.
- The app ID must resolve and continue to match the same active CPO, but it is
  public routing metadata rather than authentication.
- Login requires an active global identity and an active customer relationship
  in that active CPO.
- Access tokens remain signed then encrypted; refresh tokens remain opaque,
  hashed, rotating, and reuse-detecting.
- Customer middleware revalidates the durable session, user, customer, CPO,
  and current app ID on every request.
- Customer session listing/revocation/logout-all is limited to the current
  customer relationship and does not expose or revoke administrative or
  cross-CPO sessions.
- Password change/reset is global to the identity and therefore revokes every
  session in every plane.
- Backend handlers derive user, customer, and CPO identifiers from the
  validated customer principal, never from request bodies or query parameters.

## Backend Helper Contract

- `customerauth.CurrentPrincipal`
- `customerauth.CurrentUserID`
- `customerauth.CurrentCustomerID`
- `customerauth.CurrentCPOID`
- `customerauth.CurrentCPOAppID`
- `customerauth.RequireAppID`

## Known Incomplete Criterion

Password-reset validation and global session revocation are backend-tested,
but the forgot response and current email omit the challenge ID required by
reset/resend. The customer frontend cannot complete recovery, so this plan
cannot return to `Verified` until challenge delivery and end-to-end client
verification are complete without weakening enumeration safety.

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
- PostgreSQL lifecycle covered mail OTP login, encrypted access validation,
  `me`, refresh rotation/reuse revocation, customer-scoped session management,
  password-reset handling/change, and global session revocation. The test read
  the recovery challenge ID internally and did not verify recipient delivery.
- All 40 runtime/OpenAPI operations matched.
- Documentation verification, `go test ./...`, and `go vet ./...` passed.
