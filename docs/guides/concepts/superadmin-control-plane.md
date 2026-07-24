# Superadmin Control Plane

## What It Owns

The superadmin plane owns TransEV platform administration:

- CPO provisioning and lifecycle;
- platform administrator governance;
- software plans, subscriptions, entitlements, and platform billing records;
- operational audit and security recovery;
- durable mail and notification operations;
- worker visibility;
- platform announcements;
- platform-level overview and non-secret status;
- realtime facts for superadmin clients.

It does not own CPO charger inventory, customers, wallets, charging sessions,
tenant payment operations, or tenant integration-secret plaintext.

## Why Legacy Admin Routes Are Not the Design

The legacy CMS was inspected only as a coverage inventory. Its admin route
group mixed platform creation of administrators with tenant charger, wallet,
vehicle, support, payment, and charging operations. Loose
`associatedadminid` fields were used as several different ownership concepts.

The new CMS does not preserve those route names, trust assumptions, or data
relationships. Each legacy data area is placed under its actual owner:

- platform licensing and CPO lifecycle belong to the superadmin plane;
- tenant business operations belong to the authenticated CPO;
- protocol state and OCPP commands belong to the separate HAL;
- customer authentication and customer-owned state remain CPO-scoped.

## Command and Realtime Flow

```text
superadmin REST command
→ encrypted platform session validation
→ platform authorization
→ request validation
→ PostgreSQL transaction
→ durable state + audit + platform event + applicable queued delivery
→ commit
→ SSE announces the committed fact
→ frontend refreshes authoritative REST state
```

Retries are safe only where the command contract defines idempotency. Realtime
may duplicate an event, so clients deduplicate using the event ID.

## Subscription Boundary

A CPO can exist without a subscription. The subscription controls licensed
product capabilities, not tenant identity or ownership. Expiry never deletes
tenant data and never prevents completion or reconciliation of active charging
operations.

The platform subscription design is provider-neutral. CPO-owned Razorpay
credentials remain dedicated to that CPO's own payment operations and are not
reused for TransEV subscription collection.

## Operational Recovery

Superadmins may inspect safe metadata and recover failed durable work through
explicit APIs. They do not receive:

- arbitrary email execution;
- raw queue-payload editing;
- worker process kill/restart endpoints;
- direct password assignment;
- tenant secret decryption;
- silent impersonation.

Every recovery action requires an authenticated platform actor and becomes an
audit/event fact.

## Canonical References

- implementation plan:
  `docs/plans/superadmin-control-plane.md`;
- architecture decision:
  `docs/decisions/0007-complete-superadmin-control-plane.md`;
- HTTP contract:
  `docs/contracts/api/administrative-http-api.md`;
- realtime contract:
  `docs/contracts/realtime/platform-events.md`;
- machine-readable contract:
  `docs/contracts/openapi/openapi.yaml`.
