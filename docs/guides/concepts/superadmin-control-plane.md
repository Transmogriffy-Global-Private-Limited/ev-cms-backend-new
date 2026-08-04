# Superadmin Control Plane

## Purpose

The platform superadmin operates the CMS itself. The role provisions CPOs,
controls their access, observes platform health, and performs explicit recovery
or governance actions. It does not silently become a user inside a CPO or gain
access to tenant business data and secrets.

## Manual CPO Access

CPO access is intentionally simple:

```text
create CPO
→ PENDING
→ superadmin activates
→ ACTIVE
→ superadmin may suspend
→ SUSPENDED
```

- `PENDING` retains onboarding state but does not permit CPO administrative
  login.
- `ACTIVE` permits eligible CPO staff authentication and tenant operations.
- `SUSPENDED` blocks new tenant access while preserving the tenant and its
  historical data.

The platform can record manually managed commercial subscription plans and CPO
subscription periods. They are commercial records only: there is no
feature-key catalog, entitlement package, platform invoice, platform payment,
or provider-driven lifecycle. Access never changes automatically because of a
billing event. Activation and suspension are explicit platform-superadmin
decisions and are audited.

The CPO app ID remains separate from lifecycle:

- every new CPO receives a generated dummy app ID;
- activation does not require a live app ID;
- the superadmin may replace it with a real integration ID when the CPO's app
  goes live;
- an app ID is routing metadata, not authority and not commercial access.

## Implemented Operations

The current platform-management surface includes:

- searchable/filterable/cursor-paginated CPO collection and CPO detail;
- CPO creation, business-profile replacement, reasoned activation/suspension,
  and app-ID replacement;
- mandatory GSTIN and complete address registration backed by database
  constraints, normalized unique slug/GSTIN indexes, and an advisory
  authenticated slug-availability preflight;
- primary-administrator visibility, recovery/replacement, credential-free
  onboarding resend, and CPO administrative-session revocation;
- durable platform event replay and authenticated SSE;
- filtered platform audit queries;
- registered worker-health visibility and readiness degradation;
- encrypted mail-outbox observation through worker status;
- CPO-owned Razorpay credential storage without platform plaintext access.

The current source does not yet include platform-admin governance, generic mail
job administration, notification/announcement, or overview/status command
surfaces. Those remain planned in the active superadmin plan.

Administrative forgot/reset keeps its public response generic while the
eligible recipient's encrypted email supplies the recovery ID, code, and
expiry needed by reset. The exact SuperAdmin FE-ready and blocked surface is
recorded in `../../SUPERADMIN_FRONTEND_HANDOFF.md`.

## Realtime and Recovery

PostgreSQL remains authoritative. Platform events announce committed facts and
use an ordered numeric cursor. SSE is a low-latency view invalidation channel,
not durable truth.

```text
committed platform change
→ platform_events row in the same transaction
→ SSE delivery when connected
→ REST replay after reconnect
→ authoritative REST resource refresh
```

Clients deduplicate using the event ID. Missing or expired event history is
recovered by reloading authoritative REST state.

## Security Boundary

- Platform sessions and CPO sessions are different authorization planes.
- Tenant context comes from a verified CPO session, never from a client-chosen
  tenant header.
- A superadmin cannot resolve CPO Razorpay secret plaintext.
- Support or impersonation access is not implemented.
- CPO access changes must remain explicit, reasoned where the endpoint contract
  requires it, and auditable.

## Retired Commercial Prototype

Migrations seven and eight previously introduced tenant subscription and
platform-billing prototypes. They were deployed before the product decision
changed. Migration nine moves those tables, without deleting their data, into
the non-runtime `retired_commercial` schema and disables their worker records.
No route or active application module reads or writes that schema.
