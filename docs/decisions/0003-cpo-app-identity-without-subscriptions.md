# 0003: CPO App Identity Is Independent of Subscription

Status: Accepted

Date: 2026-07-23

## Context

CPO tenants need an application identifier shared by their application and
users on tenant-scoped API requests. They need to be provisioned and tested
before a production application identity exists, and commercial subscription
rules are not yet defined.

Requiring that identifier on login or refresh would create a recovery
dependency: a client could not learn a newly assigned identifier without
already knowing it.

## Decision

- CPO creation has no subscription dependency.
- Every CPO receives a unique server-generated dummy app ID.
- CPO creation transactionally establishes its first `ADMIN` membership.
- New identities receive an encrypted-mail temporary password and a durable
  must-change-password flag; existing identities are attached without password
  reset.
- Temporary passwords have no expiry, but every successful login queues a
  reminder and tenant business APIs remain blocked until change/reset.
- CPO lifecycle and dummy/live app-ID mode are independent.
- Platform superadmins activate/suspend CPOs and assign or rotate live app IDs.
- Tenant business APIs require `X-CPO-App-ID` after authenticated CPO context is
  established.
- The header must match the principal's CPO but never establishes tenant
  context or authorization.
- Authentication, platform, health, and independently authenticated callback
  surfaces are exempt.
- Authentication bootstrap responses return the current CPO app ID.

## Consequences

- A CPO may onboard while active with a dummy app ID and no subscription.
- The first administrator can safely be onboarded without returning password
  plaintext through an API.
- Live-ID rotation invalidates old tenant headers immediately while preserving
  account recovery and session-control endpoints.
- The app ID can be distributed to tenant clients and must not be described or
  handled as a secret.
- Subscription and entitlement behavior can be added later without changing
  tenant identity semantics.
