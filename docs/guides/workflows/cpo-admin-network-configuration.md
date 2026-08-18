# CPO Administrator and Initial Network Configuration

## Purpose

This guide explains the first callable CPO business surface: the one
platform-provisioned CPO administrator can maintain their own identity profile,
create hubs, register CMS charger/connector projections, define GST profiles,
and create tariffs.

Customers use the separate `/api/v1/app/auth/*` session plane and receive no
staff authority. A CPO ADMIN has only a tenant-scoped point lookup at
`GET /api/v1/cpo/users/{user_id}`; there is no customer or staff directory,
role-management, or customer-mutation API.

## Current Authority

Only CPO role `ADMIN` is callable. Platform provisioning creates exactly one
primary ADMIN. There is no tenant staff creation, invitation, assignment, or
role-management API.

The schema retains `OWNER`, `OPERATOR`, and `VIEWER` enum values as dormant
future capacity. They do not currently authenticate a CPO administrative
session or authorize a CPO operation. A later staff-management feature can
activate those values by deliberately adding lifecycle, authorization,
recovery, contracts, and tests; current handlers do not need to be redesigned.

Platform Superadmin and CPO ADMIN remain separate planes:

- Superadmin creates and activates the CPO, manages its organization fields and
  app ID, and can recover the primary administrator.
- CPO ADMIN can read the authenticated tenant's organization details and
  manages only its own identity and tenant operational records.
- Superadmin authority never grants tenant-business access.

## Authentication Sequence

```text
Superadmin provisions and activates CPO
→ primary ADMIN receives onboarding mail
→ ADMIN logs in and verifies mail OTP
→ ADMIN changes temporary password
→ frontend stores current CPO app ID from auth response
→ each CPO operation sends bearer token + X-CPO-App-ID
→ backend derives CPO from durable session
→ app-ID header is compared with that CPO
→ ADMIN-only policy is enforced
```

The frontend must not treat `X-CPO-App-ID` as a tenant selector or secret.
`401 unauthorized` means the authenticated session is unusable.
`403 cpo_app_id_mismatch` means refresh or call `/api/v1/auth/me` and adopt the
current app ID. `403 password_change_required` means complete the password
change first.

## Administrator Identity Profile

`GET /api/v1/cpo/admin/profile` returns the authenticated administrator's
global login identity, including optional phone.

`PATCH /api/v1/cpo/admin/profile` changes only:

- `full_name`;
- `phone`, where blank clears it.

Email changes need a future verified-email workflow and are intentionally not
accepted. Role, CPO organization fields, membership, password, and verification
state are also outside this route. Password changes use
`POST /api/v1/auth/password/change`.

The profile is identity-owned, not CPO-organization-owned. There is no mutable
`/api/v1/cpo/profile`. Superadmin manages CPO organization details through the
platform CPO contract.

## CPO Organization Details

`GET /api/v1/cpo/organization` returns the registration/business fields for
the CPO selected by the authenticated ADMIN session, together with current
lifecycle status and app-ID information. The request does not accept a CPO ID;
the frontend cannot use it to inspect another tenant.

This projection omits the internal Superadmin actor and privileged lifecycle
reason. It is read-only: corrections to business name, company type, GSTIN, or
address still require the Superadmin platform workflow. The current `app_id`
is not a credential; the frontend already needs it as `X-CPO-App-ID`.

## Recommended Configuration Order

### 1. Create a hub

Call `POST /api/v1/cpo/hubs` and retain its generated UUID. Latitude and
longitude are required even when either value is exactly zero. Optional
`sanction_load` is the site's non-negative electrical capacity in kW; the
stored default `0` means that capacity has not been recorded. Optional
`customer_visible` defaults to `false` and must remain false at creation:
`customer_visible:true` returns `409 hub_tariff_root_required`. First create
the Hub, then create its exactly-one enabled unbounded Hub-root tariff, and
only then publish with `PATCH /api/v1/cpo/hubs/{hub_id}`. Only published hubs
and their attached chargers appear in the User App.

### 2. Register a charger and connectors

Call `POST /api/v1/cpo/chargers` with every initial connector. `hub_id` is
optional: include a same-CPO hub UUID to register it there immediately, or omit
it to create independent inventory before the physical site is known.
The server generates:

- CMS charger UUID;
- six-character public charger ID;
- OCPP identity mapping;
- connector UUIDs.

Do not generate those values in the frontend. The entire charger/connector
creation and audit record commit atomically.

The OCPP identity is only a mapping value. This operation does not contact the
HAL, establish an OCPP connection, or prove live charger status.

### 3. Attach or reassign a charger when required

Call `POST /api/v1/cpo/hubs/{hub_id}/chargers` with the charger's CMS UUID.
Both resources must belong to the signed-in CPO. Repeating the same target hub
is side-effect free; a real move records `CHARGER_HUB_REASSIGNED`. A move does
not rewrite tariff targets: charger tariffs remain on the charger and hub
tariffs remain on the hub.

### 4. Create a GST profile

Call `POST /api/v1/cpo/gsts` with all three exact decimal rates. Send decimals
as JSON strings to avoid client floating-point rounding.

### 5. Create a tariff

Create the tariff through its owning scope: `POST
/api/v1/cpo/hubs/{hub_id}/tariffs`, `POST
/api/v1/cpo/chargers/{charger_id}/tariffs`, or `POST
/api/v1/cpo/user-groups/{user_group_id}/tariffs`. The route fixes exactly one
target; target IDs in the body are rejected. Every referenced GST record must
belong to the same CPO. Currency defaults to INR. An active
tariff is a root (omit both effective dates), an open-ended fallback (supply a
start only), or a complete `[start_date, end_date)` bounded override. Enabled
rows for one exact target may have nested bounded overrides but never crossing
or identical bounds; duplicate roots/open starts return
`409 tariff_temporal_conflict`. A customer-visible Hub also requires exactly
one enabled unbounded Hub root; tariff dates never set a charging-session
cutoff.

### 6. Read and update

- Hubs, chargers, GST profiles, and user groups have bounded keyset listing
  plus get/update. Tariffs are listed, retrieved, and updated through their
  hub, charger, or user-group scope.
- Chargers additionally have dependency-safe deletion.
- Hubs and GST profiles do not have delete routes. Scoped tariff `DELETE`
  returns `204` only when no historic charging reference or published-Hub root
  floor prevents removal.
- Connector changes occur only as updates to existing connector UUIDs on a
  charger. Adding/removing connectors is not implemented.
- GST and tariffs are retained by setting `is_active=false`.

The complete bodies, examples, errors, and constraints are in
`../../contracts/api/administrative-http-api.md` and the Swagger/OpenAPI
surface.

## Failure and Recovery

Mutations use PostgreSQL transactions. A failed related-record lookup, database
constraint, or audit write rolls back the business mutation.

Client retries:

- GET operations are safe.
- PATCH operations are replacement-style updates for supplied fields and are
  safe to retry with the same body.
- Repeating `POST /api/v1/cpo/hubs/{hub_id}/chargers` for the charger's current
  hub is safe and side-effect free. A tariff-schedule conflict means no move
  occurred; reconcile the overlapping tariffs before retrying.
- Creates do not currently accept an idempotency key. After a timeout, query the
  available read surface before retrying; uniqueness constraints prevent some
  duplicates but are not a general idempotency guarantee.
- Charger delete is safe to repeat only until the first successful `204`; a
  later retry returns `404 charger_not_found`.

`409 charger_in_use` means durable related records prevent deletion. Do not
work around it with direct SQL or cascading deletion. Retire or remove
dependent records through their owning workflow.

## Realtime and HAL Limits

This surface is REST-only. It does not emit tenant realtime events and the UI
must refresh from REST after its own successful mutation.

Live charger state remains a future CMS/HAL integration concern. HAL owns OCPP
connections, protocol state, raw meters, commands, reconnect recovery, and
exact transaction IDs. See `../../integrations/ocpp-hal-boundary.md`.

## Verification

Run:

```powershell
go test ./src/cpo -count=1
go test ./src/routes -run TestOpenAPIContractMatchesRuntimeRoutesAndServesUI -count=1
.\scripts\verify-docs.ps1
```

With an explicitly disposable PostgreSQL database:

```powershell
$env:TEST_DATABASE_URL = '<test-database-url>'
go test ./src/cpo -run TestCPOAdminProfileAndNetworkConfigurationWithPostgreSQL -count=1
```

The PostgreSQL test creates durable test rows and must not target a valuable or
production database.
