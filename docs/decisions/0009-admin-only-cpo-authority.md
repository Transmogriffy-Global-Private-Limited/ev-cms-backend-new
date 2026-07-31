# ADR 0009: Begin CPO Operations with One Administrator Authority

Status: Accepted

Date: 2026-07-31

## Context

The initial schema deliberately retained four fixed CPO membership values:
`OWNER`, `ADMIN`, `OPERATOR`, and `VIEWER`. Only the first administrator is
actually provisioned, and no staff invitation, creation, role assignment,
removal, or recovery workflow exists.

Exposing speculative role behavior in the first charging-network endpoints
would create an authorization contract before the staff lifecycle and its
frontend requirements are understood.

The CPO organization already stores business/company details managed by the
platform. A tenant-side organization "profile" would duplicate that authority.
The human administrator still needs a safe personal identity profile.

## Decision

- Current CPO authentication accepts only an active `ADMIN` membership on an
  active CPO.
- Every callable CPO business and integration operation requires `ADMIN`.
- Platform provisioning and primary-admin recovery normalize the selected
  primary membership to `ADMIN`.
- `OWNER`, `OPERATOR`, and `VIEWER` remain valid persisted enum values but are
  dormant: no API creates them and they grant no current CPO authority.
- There is no tenant-side CPO organization profile route.
- `GET/PATCH /api/v1/cpo/admin/profile` exposes only the authenticated global
  administrator identity. Email and authority are immutable there.
- `/api/v1/auth/me` remains the canonical session/bootstrap response.

## Consequences

The first frontend has one unambiguous CPO persona and cannot accidentally
depend on unimplemented role semantics. A later staff-management feature can
activate the dormant values without changing membership keys, tenant IDs,
charger records, or customer records, but it must deliberately define:

- invitation and acceptance;
- role transitions and last-admin protection;
- suspension/removal and session revocation;
- per-operation authorization;
- audit and mail behavior;
- frontend contracts and recovery;
- migration/backfill requirements, if any.

An identity profile is global to the login user. If future requirements need
different display data per CPO membership, that feature must add an explicit
membership-owned profile rather than silently changing the meaning of the
current endpoint.

## Rejected Alternatives

- Activating all four roles immediately: rejected because no staff lifecycle or
  approved capability matrix exists.
- Removing dormant enum values from migration one: rejected because the
  migration is already deployed and the values provide harmless future storage
  capacity while application authorization remains closed.
- Adding `/api/v1/cpo/profile` for organization fields: rejected because it
  duplicates the platform-managed CPO record and blurs control-plane ownership.
