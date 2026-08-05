# Customer App Experience Plan

Status: In Progress — CPO-scoped customer accounts implemented; DB lifecycle pending

## Objective

Build the complete customer-facing CMS surface in dependency order, without
duplicating CPO administration and without implying that CMS data is live OCPP
truth. The app user is a CPO-scoped customer account. Every customer API is
scoped to that account and its CPO.

The CPO ADMIN team owns network configuration. This work consumes published
CPO-owned network/pricing data through customer-safe read models; it does not
add CPO staff features, change CPO organization ownership, or embed the HAL.

## Existing Baseline

Implemented customer routes are confined to `/api/v1/app/auth`:

- email-verified signup and resend;
- password plus mail-OTP login and resend;
- rotating refresh tokens;
- `me` bootstrap;
- customer-scoped session list/revoke/logout;
- CPO-local password recovery, reset, and change.

There is no customer profile mutation, station discovery, favorites, tariff
quote, charging-session, wallet-ledger, payment, notification, or reporting
API today. Existing tables alone do not imply that those APIs exist.

## Permanent User-Surface Rules

- The current CPO app ID is routing metadata, never a credential or authority
  source.
- Protected app handlers derive user, customer, CPO, and session IDs from the
  validated `CUSTOMER` principal, never from a body, query parameter, or
  client-provided customer ID.
- Every app request carries the current `X-CPO-App-ID`. It must match the
  authenticated customer principal where a bearer token is present.
- Customer email, password, full name, phone, verification, lockout, recovery,
  sessions, and future charging data are CPO-scoped; they are not global staff
  identity data. The same email/password combination is allowed under separate
  CPOs and later credential changes stay in that one CPO account.
- Customer APIs never expose CPO integration credentials, internal OCPP
  identity mappings, audit internals, other customers, or staff data.
- Static CMS inventory is not live charger availability. HAL-backed status,
  command acknowledgment, and meter values must be explicitly labelled and
  implemented later.
- Money is exact decimal strings; billable energy is integer Wh. The frontend
  never recreates billing policy from raw tariff rows.
- REST is authoritative. Future realtime messages only tell clients what to
  re-fetch and must have a REST recovery path.

## Dependency Map

```text
Customer-account identity migration
  -> self-service identity profile
  -> published network read model
  -> favorites
  -> tariff resolver and price display
  -> customer access credentials and group policy
  -> CMS/HAL charging lifecycle
  -> wallet ledger, billing, receipts, history
  -> customer notifications and realtime refinements
```

The CPO team is an input to the published-network and pricing slices only.
The customer-account migration starts first.

## Slice 1 — CPO-Scoped Customer Account Migration

Status: Implemented; PostgreSQL lifecycle verification pending

Migration twenty adds the CPO-owned account fields and customer-only
challenge/session/refresh lineage. Signup, login, recovery, password mutation,
tokens, principal helpers, audit attribution, session management, refresh
reuse detection, and CPO-suspension revocation now use it. With no existing
customers, migration preflight fails rather than silently rewriting data if
the deployment assumption changes. Profile mutation remains Slice 2.

## Slice 2 — Customer Self-Service Profile

Status: Implemented; PostgreSQL lifecycle verification pending

### Goal

Let a signed-in customer update their CPO-scoped account display identity using
the customer principal and current `X-CPO-App-ID` request contract.

### Endpoints

- Keep `GET /api/v1/app/auth/me` as the authoritative bootstrap response.
- Add `PATCH /api/v1/app/auth/profile` with only:

```json
{"full_name":"Asha Das","phone":"+919876543210"}
```

### Rules

- `full_name` is required, trimmed, and bounded to 255 characters.
- `phone` is optional; when supplied it uses the existing normalized 7–15 digit
  format. An explicit JSON `null` clears it.
- Email is not editable here. It is the CPO-account login identifier and
  requires a separate verified/re-authenticated flow.
- Password remains owned by the existing password endpoints.
- The route may not mutate customer status, group, wallet, CPO, sessions, or
  any identifier.
- The update is visible only in this CPO account. A CPO-scoped audit record
  records the actor, customer CPO, changed field names, and no old/new PII
  values.

### Acceptance and Verification

- A customer can update only their own user record.
- Cross-CPO/customer/body-ID attempts cannot redirect the update.
- Response uses the canonical user projection and `Cache-Control: no-store`.
- Add handler/service tests, PostgreSQL lifecycle proof, route/OpenAPI parity,
  exhaustive contract and FE workflow updates. Source implementation and
  focused contract verification are complete; the lifecycle proof remains
  pending until an explicitly disposable `TEST_DATABASE_URL` is available.

## Slice 3 — Published Customer Network Read Model

### Goal

Allow a customer to browse only customer-visible hubs and their attached
chargers/connectors in their CPO.

### Required CPO-Team Dependency

The current network schema has no publish/visibility state. Before discovery,
the CPO-owned hub model needs an explicit `customer_visible` boolean, default
`false`, editable only by CPO ADMIN. It must be represented in the migration,
CPO create/update contracts, CPO guide, OpenAPI, and CPO tests. Default false
prevents accidental publication of test or unfinished infrastructure.

Only customer-visible hubs are discoverable. Independent/unassigned chargers
never appear in the customer app. A charger can appear only when attached to a
visible hub in the same CPO.

### Proposed Endpoints

- `GET /api/v1/app/hubs?limit=&before=&before_id=&q=`
- `GET /api/v1/app/hubs/{hub_id}`
- `GET /api/v1/app/chargers/{charger_id}`

All require CUSTOMER authentication and matching `X-CPO-App-ID`. Collections use
bounded, stable keyset pagination. Query/filter changes discard old cursors.

### Customer Projection

- Hub: ID, name, address, latitude, longitude, 24-hour flag, customer-visible
  metadata, and a customer-owned favorite flag.
- Charger: ID, public charger ID, vendor/model only if approved for display,
  maximum power, OCPP version, attached connector summaries, and favorite flag.
- Connector: ID, number, connector type, max current, max voltage.

Never expose OCPP identity, serial number, sanctioned load, CPO-admin notes,
or raw audit data. Do not expose raw stored status as current availability.
Until HAL integration, the response explicitly uses
`availability: "UNKNOWN"` and includes no false live/online claim.

### Verification

- Tenant, published/unpublished, attached/unassigned, and missing-resource
  tests.
- Cursor/filter stability and no cross-CPO enumeration.
- CPO publication lifecycle PostgreSQL test jointly owned with its CPO slice.
- App-route auth/header/OpenAPI and human/FE contract coverage.

## Slice 4 — Customer Favorites

### Goal

Use the existing customer-favorite tables for an authenticated customer’s saved
hubs and chargers.

### Proposed Endpoints

- `GET /api/v1/app/favorites`
- `PUT /api/v1/app/favorite-hubs/{hub_id}` / `DELETE ...`
- `PUT /api/v1/app/favorite-chargers/{charger_id}` / `DELETE ...`

Create is idempotent; delete is idempotent. A favorite must be in the current
CPO and customer-visible at the time of addition. Listing returns the current
safe discovery projection, so a later-unpublished station is not leaked.

### Verification

Prove composite tenant ownership, duplicate-safe behavior, unpublish behavior,
cross-CPO UUID non-disclosure, and pagination where lists can grow.

## Slice 5 — Effective Tariff Resolver and Customer Price Display

### Goal

Give the app one server-calculated current price display rather than making the
frontend guess from CPO tariff rows.

### Resolver Rules

At a supplied server timestamp, select active, effective tariffs scoped to the
customer’s CPO and hub. Use this deterministic precedence:

1. charger + customer user group;
2. charger + generic customer;
3. hub + customer user group;
4. hub + generic customer.

The resolver loads the active GST profile referenced by the tariff and returns
exact decimal strings. A missing eligible tariff returns an explicit
`price_unavailable` state, never a zero price. The result is informational and
not a charge commitment; a charging session later snapshots the selected tariff
and tax atomically.

### Proposed Endpoints

- `GET /api/v1/app/hubs/{hub_id}/price`
- `GET /api/v1/app/chargers/{charger_id}/price`

The exact route shape may be folded into discovery detail only if the response
remains independently cacheable and its server-calculated semantics are clear.

### Dependencies

- Published network read model.
- CPO tariff effective-date enforcement.
- Customer-group assignment lifecycle, if group-specific tariffs are activated.

Do not activate group-specific access/price behavior until the CPO team has an
approved customer/group management API. Generic tariffs can be supported first.

## Slice 6 — Customer Access Credentials and Group Policy

### Goal

Define app-user RFID/idTag and group access only after CPO administration has a
safe customer-directory, assignment, block/unblock, and group-management
surface.

### Required Design Before Code

- credential ownership, uniqueness, format, and secure storage;
- assignment/revocation lifecycle and audit requirements;
- charger/hub group policy precedence;
- HAL authorization request/response contract;
- offline/timeout/retry behavior and post-reconnect reconciliation.

This is not created from existing `user_groups` tables alone.

## Slice 7 — CMS/HAL Charging Lifecycle

### Goal

Implement the first live customer behavior only after the separate HAL contract
is approved: eligibility check, start/stop intent, durable callback ingestion,
session projection, and recovery.

### Required Contract

- mutually authenticated CMS/HAL API boundary with idempotency keys;
- immutable CMS-to-HAL charger mapping and explicit connector mapping;
- remote-start/stop intent states, timeout, retry, deduplication, and user
  cancellation semantics;
- HAL-issued OCPP transaction ID preservation;
- callback ordering/duplicate handling, session reconciliation, and restart
  recovery;
- live connector/session meter events plus REST snapshot/replay fallback.

The customer app may show `STARTING`, `ACTIVE`, `STOPPING`, or `FAILED` only
after durable CMS state supports those states. WebSocket/SSE is an optimization;
the charging-session REST detail remains authoritative.

## Slice 8 — Wallet, Billing, History, and Receipts

### Goal

Expose customer-owned financial and historical views after the charging
lifecycle has durable immutable records.

### Scope

- wallet balance and keyset-paginated immutable ledger;
- charging-session list/detail with exact Wh, tariff/tax snapshots, and status;
- customer payment attempts/settlements only when a payment provider is
  explicitly approved;
- receipt/bill data sufficient for FE PDF generation; no fragile backend file
  URL dependency.

No balance mutation is trusted from the frontend. Ledger writes, session charge
finalization, and payment settlement need idempotency and transactional audit
evidence.

## Slice 9 — Customer Notifications and Realtime Refinement

After durable session and billing events exist, add CPO-scoped customer inbox
notifications and optional realtime invalidations for session, wallet, and
receipt changes. Define event IDs, replay, reconnect, authorization, retention,
and REST snapshot recovery before adding a socket.

## Deferred Account Features

- verified global email change;
- account closure/export/erasure policy, which must respect charging, wallet,
  tax, and statutory retention;
- social login, SMS OTP, passkeys, device attestation;
- cross-CPO roaming and global station discovery;
- user-facing support impersonation.

## Execution Order and Definition of Done

1. Implement and verify Slice 1.
2. Coordinate the CPO publication dependency, then implement Slices 2 and 3.
3. Implement Slice 4 only after tariff/group semantics are accepted.
4. Do not begin HAL or financial work before their prerequisite contracts.

Each implemented slice includes: affected-schema review/migration when needed,
trusted scope and authorization, transactional state/audit effects, strict JSON
validation, error contract, OpenAPI/Swagger, human and FE documentation,
focused tests, PostgreSQL lifecycle verification against an explicitly
disposable database, full Go checks, documentation verification, and residue
scan. No slice is considered complete merely because a route returns `200`.
