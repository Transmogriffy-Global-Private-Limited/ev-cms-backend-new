# CPO Frontend Integration Handoff

## Purpose and authority

This is the complete browser-integration guide for the CPO administration
application. It describes the currently callable CPO surface: administrator
identity, tenant organization, staff, network configuration, commercial
configuration, customer insight, support, operational projections, and history.

Use sources in this order:

1. `contracts/openapi/openapi.yaml` — machine-readable request/response truth;
2. this document — ownership, UI workflow, recovery, and security semantics;
3. `contracts/api/administrative-http-api.md` — detailed HTTP behavior;
4. `CPO_FRONTEND_TARIFF_GST_HANDOFF.md` — tariff/GST lifecycle rules;
5. `CPO_OPERATIONS_LIVE_FE_HANDOFF.md` — live-operations payloads and SSE;
6. `SUPERADMIN_CPO_FRONTEND_BOUNDARY.md` — CPO versus platform authority.

Only a routed OpenAPI operation is callable. Do not infer an API from a model,
table, old application, screen mockup, or a CPO permission-catalog item.

## Authentication, tenant scope, and headers

The CPO app is an administrative client, not the customer User App.

1. Start login with `POST /api/v1/auth/login`, `scope: "CPO"`, and the selected
   `cpo_id`.
2. Verify the emailed OTP with `POST /api/v1/auth/2fa/verify`.
3. Persist access/refresh tokens only in the frontend's approved secure storage.
   A refresh token is one-time: serialize refresh requests and replace both
   tokens atomically. Reuse revokes the whole session.
4. Call `GET /api/v1/auth/me` before mounting the application. Require
   `scope: "CPO"`, an active CPO, and an active membership. Read the returned
   `cpo_app_id` from this trusted response.
5. Send both headers on every `/api/v1/cpo/*` request, including SSE/replay and
   support/integration routes:

```http
Authorization: Bearer <access-token>
X-CPO-App-ID: <current-cpo-app-id>
```

`X-CPO-App-ID` is routing metadata, not a user credential and never chooses a
tenant. The server derives the CPO from the bearer session and rejects a stale
or mismatching header. Never place either token or app ID in a URL.

Core CPO administration and provider-integration routes require an active CPO
`ADMIN` membership. CPO support and notification routes require an active CPO
session plus the matching app ID, but are not role-middleware-gated. `OWNER`,
`OPERATOR`, and `VIEWER` can exist as staff data and the permission catalog
defines their default capability sets; that catalog is not yet a general
route-level authorization contract. The UI must not show partial core
administration navigation merely because a role appears in a staff response.

## Common browser behavior

- Requests with bodies use one JSON object and `Content-Type: application/json`.
  Unknown fields and oversized/multiple JSON values are rejected.
- Treat API data as `no-store`; retain only what is necessary for current UI
  state. Do not cache encrypted-provider writes, credentials, access tokens,
  refresh tokens, OTPs, or server error bodies.
- All handled failures use `{ "error": { "code", "message" } }`. Branch on
  `code`; show the safe `message` where useful; retain `X-Request-ID` as a
  copyable support reference without request data.
- On `401`, attempt one serialized refresh; if that fails, clear local session
  state and return to login. On `403`, keep the screen read-only/blocked and
  re-bootstrap `me` rather than guessing which role or tenant changed.
- CPO-owned missing or cross-CPO records can intentionally return `404`; do not
  disclose a distinction in UI copy.
- After a successful mutation, use the returned resource as the immediate view
  state and re-fetch the relevant collection when ordering, aggregates, or
  related rows may have changed.

## Recommended application layout

| Screen | Primary data/actions | Important rule |
| --- | --- | --- |
| Login and OTP | administrative auth endpoints | Login scope is `CPO`; customer auth is separate. |
| Dashboard | analytics, fleet operations, current subscription | Live state is a projection; display freshness. |
| Organization and my profile | organization, admin profile, subscription | Organization legal identity is platform-managed/read-only. |
| Staff and permissions | catalog, staff list/detail/lifecycle | ADMIN-only today; overrides describe future/effective capability data. |
| Hubs and chargers | network CRUD, visibility, assignment, static status | CMS configuration is not OCPP transport truth. |
| Commercial | GST, nested tariffs, user groups, settings | Use decimal strings; follow tariff/GST precedence. |
| Customers and reports | customers, sessions, charger transactions, wallet ledger | Read-only CPO insight; no CPO customer mutation API. |
| Operations | fleet, charger live detail, replay, SSE | Replay then stream; REST remains authoritative. |
| Integrations | provider metadata and credential replacement/removal | Never render or expect secret readback. |
| Support | CPO ticket list/create/detail/reply | CPO can see only its own conversation. |

## Complete CPO route inventory

All following operations require the CPO administrative headers above. Core
administration and integration operations require active `ADMIN`; support and
notifications use their separately stated active-CPO-session boundary. The
OpenAPI document owns field schemas.

| Area | Operations | UI and recovery rule |
| --- | --- | --- |
| My administrator identity | `GET`, `PATCH /cpo/admin/profile`; `GET /cpo/users/{user_id}` | Profile changes global login identity fields allowed by the contract, not CPO role/email. Point lookup is tenant-safe, not a directory. |
| Organization and subscription | `GET /cpo/organization`; `GET /cpo/subscription`; `GET /cpo/analytics` | Treat organization identity as read-only. Subscription is informational for CPO; platform renewal is not a CPO action. |
| Staff catalog and lifecycle | `GET /cpo/permissions/catalog`; `GET`, `POST /cpo/staff`; `GET`, `PATCH /cpo/staff/{membership_id}`; `POST /cpo/staff/{membership_id}/activate`, `/suspend`, `/revoke` | Load catalog before editing overrides. Send only known keys once with `ALLOW`/`DENY`; a deny wins. Confirm suspend/revoke and show reason/audit effect. |
| Hubs | `GET`, `POST /cpo/hubs`; `GET`, `PATCH`, `DELETE /cpo/hubs/{hub_id}`; `PUT /cpo/hubs/{hub_id}/customer-visibility`; `GET`, `POST /cpo/hubs/{hub_id}/chargers` | Create hidden; publish only after the required active hub-root tariff exists. Assignment is same-CPO and idempotent at the target. |
| Hub GST | `GET`, `POST`, `PATCH`, `DELETE /cpo/hubs/{hub_id}/gst` | GST is hub/location-owned, independent of tariff. Show a conflict/error instead of fabricating tax. |
| Chargers and connectors | `GET`, `POST /cpo/chargers`; `GET`, `PATCH`, `DELETE /cpo/chargers/{charger_id}`; `GET /cpo/chargers/{charger_id}/image`; `GET`, `PUT /cpo/chargers/{charger_id}/status`; `PUT /cpo/chargers/{charger_id}/customer-visibility` | `charger_id` is the human/OCPP-facing identifier; the CMS UUID is separate. Static CMS status is not live OCPP availability. Deletion can fail with `charger_in_use`. |
| Hub tariffs | `GET`, `POST /cpo/hubs/{hub_id}/tariffs`; `GET`, `PATCH`, `DELETE /cpo/hubs/{hub_id}/tariffs/{tariff_id}` | URL fixes the immutable target. Follow the root-tariff-before-publication sequence. |
| Charger tariffs | `GET`, `POST /cpo/chargers/{charger_id}/tariffs`; `GET`, `PATCH`, `DELETE /cpo/chargers/{charger_id}/tariffs/{tariff_id}` | Charger tariff overrides hub tariff only in customer tariff precedence. |
| User-group tariffs | `GET`, `POST /cpo/user-groups/{user_group_id}/tariffs`; `GET`, `PATCH`, `DELETE /cpo/user-groups/{user_group_id}/tariffs/{tariff_id}` | Group tariff overrides charger and hub tariff; never put a target ID in the body. |
| GST profiles | `GET`, `POST /cpo/gsts`; `GET`, `PATCH /cpo/gsts/{gst_id}` | Decimal monetary/tax values are strings; do not use JavaScript floats. |
| User groups | `GET`, `POST /cpo/user-groups`; `GET`, `PATCH`, `DELETE /cpo/user-groups/{user_group_id}`; `POST /cpo/user-groups/{user_group_id}/members`; `DELETE /cpo/user-groups/{user_group_id}/members/{customer_id}` | Membership changes affect future tariff selection only; do not rewrite settled sessions. |
| Settings and invoice logo | `GET`, `POST`, `PUT /cpo/settings`; `GET /cpo/settings/invoice-logo` | POST and PUT are replacement/upsert forms. Refresh settings after either. |
| Customers | `GET /cpo/customers`; `GET /cpo/customers/{customer_id}` | Customer and usage data are read-only CPO projections. Do not expose customer authentication/wallet mutation controls here. |
| Charging/reporting | `GET /cpo/charging-sessions`; `GET /cpo/charging-sessions/{session_id}`; `GET /cpo/charger-transactions`; `GET /cpo/wallet-transactions` | Use cursor fields unchanged. Show CMS/HAL/OCPP identifiers and reconciliation/settlement state as distinct facts; never infer a missing session from charger live state. |
| Operational projection and realtime | `GET /cpo/operations/fleet`; `GET /cpo/operations/chargers/{charger_id}`; `GET /cpo/operations/events`; `GET /cpo/operations/realtime/stream`; `GET /cpo/operations/live-sessions`; `GET /cpo/operations/live-sessions/snapshot` | The live-session primary route is full-snapshot SSE: replace the table from each frame. Each row supplies `duration_seconds` at `as_of`, `customer_name`, CMS `connector_id`, charger/hub display context, and live meter/SoC status without customer contact or billing data. It needs no event replay or per-update REST refresh. General fleet/charger streams remain invalidation-based. See `CPO_OPERATIONS_LIVE_FE_HANDOFF.md`. |
| Provider integrations | `GET /cpo/integrations`; `GET`, `PUT`, `DELETE /cpo/integrations/{provider}` | PUT submits credentials for encryption but returns metadata only. Never display, log, or expect provider secret plaintext. |
| Support | `GET`, `POST /cpo/support`; `GET /cpo/support/{ticket_id}`; `POST /cpo/support/{ticket_id}/replies` | Any active CPO membership with the app ID may use this tenant-scoped conversation boundary; re-fetch ticket after a reply. |
| CPO notifications | `GET /cpo/notifications`; `POST /cpo/notifications/{notification_id}/read` | Any active CPO membership with the app ID may use these personal notifications. Poll/refetch after relevant platform actions; there is no notification SSE stream. |

## Commercial and network invariants the UI must preserve

- A hub tariff root is the publication prerequisite. The safe path is hidden hub
  → enabled unbounded hub tariff → customer visibility. Do not optimistically
  present a hub as public before the server accepts it.
- Tariff precedence is `UserGroup > Charger > Hub`; GST resolves independently
  from the selected charger's hub. Current tariff values are future commercial
  policy, while settled sessions retain snapshots.
- Use exact decimal strings for money, GST rates, and kWh amounts. Never add,
  compare, or round currency with binary JavaScript numbers.
- CPO legal identity is platform-owned. A CPO UI may display GSTIN/state/PIN but
  cannot edit them; only the SuperAdmin profile replacement route can do so.
- Customer visibility and static charger status are commercial/CMS publication
  controls. They never claim that the charger is physically connected or OCPP
  `Available`.

## Operational realtime and recovery

For operational views, do the following in order:

1. Fetch the authoritative REST snapshot for the visible fleet/charger or
   ongoing-session table. The live-session table intentionally contains only
   materialized ongoing sessions and display-safe live telemetry.
2. Replay `GET /cpo/operations/events?after_id=<saved>` until `has_more=false`.
3. Connect to `/cpo/operations/realtime/stream` with `fetch` streaming, not
   native `EventSource`, because both required headers must be sent.
4. Treat each event as invalidation evidence. Dedupe by event ID and re-fetch
   the affected REST resource before updating durable UI state.
5. Persist the cursor only after the event's refresh completes. On disconnect,
   token rotation, or tab resume, replay again before reconnecting.
6. If the server returns `realtime_cursor_expired`, discard that cursor, reload
   the visible authoritative snapshots, then reconnect without it.

Use the general operations replay/SSE pair for fleet and charger views. For the
ongoing-session table, connect directly to `/cpo/operations/live-sessions` and
replace the table from each `snapshot` or `live_sessions` frame. On reconnect,
open it again for a fresh initial snapshot. Use
`/cpo/operations/live-sessions/snapshot` only for explicit JSON recovery or
pagination; `/events` is advanced reconciliation, not a frontend requirement.

HAL owns physical connection/OCPP truth; CMS owns the durable projection. A
fresh-looking client must not invent live certainty if the response is `STALE`,
`UNKNOWN`, absent, or a reconciliation state.

## Permission-editor behavior

`GET /cpo/permissions/catalog` is the authoritative list of override keys.
The staff UI may use it to configure membership data, but it must communicate
the current server reality:

- current core CPO administration and integration routes are `ADMIN`-gated;
- `ADMIN` defaults to every registered catalog capability;
- `DENY` takes precedence over default role capability and `ALLOW` can grant a
  known capability where a future route-level check uses it;
- a catalog/override does not itself make an `OWNER`, `OPERATOR`, or `VIEWER`
  core administration route callable today; support and notifications are
  independently available to an active CPO membership;
- omit an override to return to the default; never submit duplicate keys.

This is intentionally not a frontend-only authorization scheme. The backend
must be the future enforcement point for any capability-specific route.

## CPO versus SuperAdmin difference

The CPO app is tenant-scoped operations and commercial administration. The
SuperAdmin app manages the platform, CPO registration/lifecycle, platform
administrators, subscription records, mail/security/worker operations, and
global support. It must not become a CPO dashboard or a route to tenant secrets.

Read `SUPERADMIN_CPO_FRONTEND_BOUNDARY.md` before sharing components. In
particular, never reuse a platform bearer token with `/cpo`, never attach
`X-CPO-App-ID` to `/platform`, and never present a SuperAdmin as an implicit
CPO ADMIN.

## Verification checklist for the frontend

- [ ] CPO login sends `scope: "CPO"` and a selected CPO ID; OTP/refresh flows
  handle one-time refresh replacement.
- [ ] Bootstrap validates `me.scope`, CPO context, and app ID before mounting.
- [ ] Every CPO request, replay, and SSE connection carries both required
  headers without putting them in URLs.
- [ ] Route inventory above is wired from OpenAPI types, without a CPO-side
  legal-identity/profile mutation screen.
- [ ] Commercial forms use exact decimal strings and publication order.
- [ ] Operational events invalidate and refetch REST state; no UI treats SSE as
  durable state or assumes CMS static state equals OCPP truth.
- [ ] Integration secret values never appear after save or in diagnostics.
- [ ] Staff permissions are described as current metadata plus future backend
  capability potential, not a client-side bypass.
