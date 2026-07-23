# 0006: Separate Customer Session Plane

Status: Accepted

Date: 2026-07-23

## Context

Customers and CPO staff may share one global login identity, but their
authority, data access, and session-management expectations differ. Reusing a
CPO administrative session for a charging app would blur tenant customer
ownership and could expose or revoke unrelated sessions.

## Decision

- Add a distinct `CUSTOMER` authentication scope.
- Bind each customer session durably to one `user_id`, `customer_id`, and
  `cpo_id`, with no CPO staff role.
- Keep the existing signed/encrypted access-token and rotating opaque refresh
  design, while revalidating customer, CPO, user, session, wallet, and app
  identity from PostgreSQL.
- Require the current CPO app ID on public and protected app authentication
  routes as routing metadata, not as a secret.
- Scope customer session listing, revocation, and logout-all to the exact
  customer/CPO relationship.
- Treat password changes and resets as global identity operations that revoke
  sessions in every authentication plane.
- Expose trusted `customerauth.Current*` helpers for backend app handlers.

## Consequences

- Customer access tokens cannot authorize platform or CPO-staff endpoints.
- A single identity may hold independent administrative and customer sessions.
- Customer logout-all cannot disrupt another CPO or an administrative session.
- A password change/reset intentionally revokes all of those sessions because
  the password belongs to the global identity.
- App handlers can build a `me` response and tenant queries without trusting
  client-supplied customer or CPO identifiers.
