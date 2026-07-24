# ADR 0007: Complete Superadmin Control Plane

Status: Superseded in part by ADR 0008

Date: 2026-07-24

ADR 0008 removes the subscription, entitlement, platform-invoice, and
platform-payment decision. The CPO lifecycle, governance, audit, worker,
notification, overview, PostgreSQL-event, SSE, REST-recovery, and tenant-data
boundaries in this record remain accepted.

## Context

The credential boundary and initial CPO lifecycle APIs are complete, but the
platform has no durable subscription model, operational mail API, platform
notification center, worker-health contract, audit query, or reconnect-safe
realtime delivery.

The legacy CMS mixed superadmin and CPO-admin operations in one route surface
and stored tenant ownership in loosely interpreted administrator identifiers.
It contains no subscription data model worth preserving.

TransEV requires a complete platform plane that does not need to be redesigned
whenever a new CPO is licensed or an operational failure occurs.

## Decision

Implement one modular-monolith platform control plane with explicit modules for:

- CPO lifecycle and administrative recovery;
- platform administrator governance;
- provider-neutral subscription catalog, lifecycle, entitlements, and billing
  records;
- audit and security operations;
- durable mail and notification operations;
- worker heartbeats and recoverable jobs;
- announcements;
- platform overview and non-secret system status;
- durable PostgreSQL platform events with authenticated SSE and REST replay.

A subscription is optional. CPO lifecycle and subscription lifecycle are
separate. Published plan versions and invoices are immutable. Entitlements are
resolved from a subscription snapshot plus explicit expiring overrides.

REST owns commands and authoritative queries. PostgreSQL owns durable truth.
Realtime announces committed facts using at-least-once SSE delivery and cannot
mutate state.

Platform superadmin authority does not grant silent access to tenant business
data or tenant integration-secret plaintext.

## Consequences

- The platform plane gains more tables and APIs, but each has explicit
  ownership and recovery behavior.
- No Redis, NATS, or separate realtime service is required initially.
- Automatic subscription payment collection is not implemented until a
  platform billing provider is separately approved.
- Subscription enforcement must be integrated explicitly into later tenant
  features; no-subscription CPO creation and authentication remain valid.
- Every platform mutation must coordinate durable state, audit, events, and
  applicable mail/notification work in one transaction.
- OpenAPI, interactive documentation, and documentation-route environment
  toggling remain required contract surfaces.
