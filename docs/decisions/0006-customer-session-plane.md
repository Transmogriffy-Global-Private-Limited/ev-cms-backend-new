# 0006: Separate Customer Session Plane

Status: Superseded in part by ADR 0013

Date: 2026-07-23

## Context

At the time of this decision, customers and CPO staff could share one global
login identity, but their
authority, data access, and session-management expectations differ. Reusing a
CPO administrative session for a charging app would blur tenant customer
ownership and could expose or revoke unrelated sessions.

## Decision

- Add a distinct `CUSTOMER` authentication scope.
- Historical decision, superseded: bind customer sessions to global `user_id`.
  ADR 0013 binds dedicated customer sessions to `customer_id` and `cpo_id`.
- Keep the existing signed/encrypted access-token and rotating opaque refresh
  design, while revalidating customer, CPO, user, session, wallet, and app
  identity from PostgreSQL.
- Require the current CPO app ID on public and protected app authentication
  routes as routing metadata, not as a secret.
- Scope customer session listing, revocation, and logout-all to the exact
  customer/CPO relationship.
- ADR 0013 makes password changes/resets CPO-local and revokes only the exact
  customer account's sessions.
- Lock the CPO-local customer row before authorizing a current-password change;
  an already-committed replacement makes the old password stale for any waiter.
- Expose trusted `customerauth.Current*` helpers for backend app handlers.

## Consequences

- Customer access tokens cannot authorize platform or CPO-staff endpoints.
- A single identity may hold independent administrative and customer sessions.
- Customer logout-all cannot disrupt another CPO or an administrative session.
- Customer passwords no longer belong to the administrative identity plane.
- App handlers can build a `me` response and tenant queries without trusting
  client-supplied customer or CPO identifiers.
