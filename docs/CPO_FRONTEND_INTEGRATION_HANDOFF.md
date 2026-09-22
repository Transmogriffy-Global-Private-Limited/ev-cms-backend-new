# CPO Frontend Integration Handoff

## Purpose and authority

This is the complete browser-integration guide for the CPO administration
application. It describes the currently callable CPO surface: administrator
identity, tenant organization, staff, network configuration, commercial
configuration, customer insight, support, operational projections, and history.

This document is the standalone implementation and behavior reference for the
CPO frontend. The OpenAPI document remains the canonical machine-readable
schema used to generate/validate request and response types; it does not
replace the authority, privacy, recovery, and UI rules stated here. Related
documents can give background, but this handoff carries the rules required to
implement the current CPO application without relying on them.

Only a routed OpenAPI operation is callable. Do not infer an API from a model,
table, old application, screen mockup, or a CPO permission-catalog item.

## Evidence, scope, and system model

This document describes the checked-out CMS source revision
`a1f8717bf2346ed63277ca871f994faf25851b8e`. It is source-verified where it
states a current route, schema, authorization rule, configuration behavior, or
durable-state invariant. It is not evidence that this revision is deployed,
that a named remote deployment has a working provider/HAL, or that a physical
charger accepted an OCPP command. `docs/PROJECT_STATE.md` records prior release
observations, which are historical evidence only until an operator repeats the
runtime verification procedure against the intended target.

Use these evidence states literally:

| State | What it proves | What it does not prove |
| --- | --- | --- |
| Source-verified | Behavior in this checkout and its static contract. | A remote runtime, database state, or device effect. |
| Runtime-verified | A request/health check actually ran against the selected target. | A different revision/environment or later state. |
| Prior deployment record | A previously recorded release observation. | Current deployment identity or availability. |
| Unverified | A boundary not exercised because prerequisite authority/state is absent. | Failure or success. |

```text
CPO administrator browser
  -> CMS /api/v1/cpo (tenant administration and durable commercial truth)
  -> CMS operation ledger -> authenticated HAL v1 -> charge point/OCPP
CMS PostgreSQL: tenant, staff, tariffs, wallets, sessions, operation records
HAL: protocol connections, exact OCPP transaction IDs, raw meter/device truth
```

The browser never talks to HAL, never chooses a CPO by body UUID, and never
becomes authority for a device, money, session, tariff, permission, or
operation result. CMS success records are only as strong as their documented
state: an accepted operation is not a physical device effect, a queued invoice
delivery is not SMTP delivery, and an SSE frame is not a reason to overwrite
durable data beyond the dedicated full-snapshot contract.

### Preparation, prerequisites, and stop conditions

This repository contains the Go CMS backend only. No CPO browser project,
package manager, browser build command, mobile signing procedure, or CPO App
ID is present here. Obtain the frontend's own approved repository and build
instructions; do not invent a JavaScript command from this backend checkout.

For a local CMS, install Go 1.25+ and PostgreSQL, then run from the repository
root:

```powershell
Copy-Item .env.example .env
go mod download
go run .
```

Supply the required ignored `.env` values before startup. Do not commit the
file, expose secrets in browser configuration, or aim `TEST_DATABASE_URL` at a
shared/live database. The process applies pending up migrations at startup;
the explicit `go run ./cmd/migrate -direction down` path can remove data and is
not a frontend setup action.

Stop integration/testing and correct the environment before proceeding when:

- `GET /health/live` is not `200` (the process is unavailable);
- `GET /health/ready` is not `200` (PostgreSQL or required workers are not
  ready);
- the served OpenAPI does not contain the intended route or differs from the
  generated client source;
- the configured CPO App ID is absent/unknown or does not correspond to the
  desired active CPO;
- an operation would target a real charger, customer, wallet, invoice email,
  or provider without explicit test authority.

When the browser is hosted cross-origin, current source has only
`CORS_ALLOW_ALL=true`, a wildcard CORS switch that is false by default. Use it
only in an approved non-production environment. There is no configured
origin-allowlist mode in this source. Do not put bearer tokens, App IDs, or
secrets in URLs as a preflight workaround.

## Base URL, environment, and document boundary

Configure one approved CMS origin per frontend environment. Keep the origin
without a trailing slash and derive the CPO API base exactly once:

```ts
const API_ORIGIN = import.meta.env.VITE_EV_CMS_API_ORIGIN;
const API_BASE_URL = `${API_ORIGIN.replace(/\/+$/, "")}/api/v1`;
const CPO_ROOT = `${API_BASE_URL}/cpo`;
```

For local development use `http://127.0.0.1:8080`; the shared development
deployment is `https://dev-evcmsnew.transev.site`. A production origin must be
provided through approved frontend configuration and must never be guessed from
a CPO app ID, backend listener, database, or old deployment note. The CPO App
ID is a separate deploy-time public configuration value. It selects the CPO
context but is not a secret and does not grant access.

All paths in this document are relative to `/api/v1` unless they are written
as full paths. `/cpo` is the administrative resource root. `/app` is the
customer application root and `/platform` is the SuperAdmin root; a CPO
frontend must never call either with its administrative bearer. Health/docs
live at the origin root: `/health/live`, `/health/ready`, and, only where
`API_DOCS_ENABLED=true`, `/docs/` and `/openapi.yaml`.

For a browser hosted on a different origin, the deployment must deliberately
enable the backend CORS policy (`CORS_ALLOW_ALL=true`) before this frontend can
send `Authorization` and `X-CPO-App-ID`. That setting is a deployment concern,
not a browser fallback: do not suppress the headers, move tokens to query
parameters, or proxy around an unapproved CORS configuration.

Every successful HTTP response proves only the documented CMS transition or
read completed. It does not prove SMTP delivery, an external provider effect,
or a physical charger effect unless that exact response field is documented as
such. CMS owns CPO commercial/admin durable state; HAL owns charger connection,
OCPP protocol, transaction identifiers, and raw meter communication.

## Authentication, tenant scope, and headers

The CPO app is an administrative client, not the customer User App.

1. Configure the public CPO App ID in the frontend. Start login with
   `POST /api/v1/auth/login`, header `X-CPO-App-ID: <cpo-app-id>`, and only
   `email`, `password`, `scope: "CPO"` in JSON. Never ask a human for an internal
   CPO UUID or send `cpo_id`; unknown body fields are rejected.
2. Verify the emailed OTP with `POST /api/v1/auth/2fa/verify`. Verify/resend use
   the challenge ID without tenant reselection or an App-ID header. The server
   revalidates the challenge-bound CPO and membership before issuing tokens.
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

`X-CPO-App-ID` is a public context selector, not a secret or access grant.
Initial login selects the intended active CPO and requires the authenticated
user's active membership there. Unknown App IDs, inactive organizations or
memberships, nonmembers and bad credentials share `invalid_credentials`.
After login the internal CPO UUID is fixed in the challenge/session. Refresh
cannot switch it. On protected requests the server derives the CPO from the
bearer session and rejects a stale or mismatching header. Never place either
token or app ID in a URL.

```http
POST /api/v1/auth/login
Content-Type: application/json
X-CPO-App-ID: <cpo-app-id>

{"email":"admin@example.com","password":"<password>","scope":"CPO"}
```

PLATFORM login rejects a supplied App-ID header. CPO login/OTP failure details
are intentionally tenant-safe: do not distinguish an unknown app ID, inactive
CPO, inactive membership, nonmember, or bad credential in frontend copy.

Every capability-protected CPO business route requires an active CPO membership,
the matching app ID, and its documented capability. Roles are source-controlled default bundles,
not endpoint authority: use `GET /api/v1/cpo/access/me`
`effective_permissions` to gate navigation and actions. Explicit `DENY` always
wins, including against an ADMIN default; an explicit `ALLOW` can grant a
capability to an otherwise lower-role member. Support has separate
`support.read`, `support.create`, and `support.reply` capabilities.

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

### Error, refresh, and retry matrix

Every handled failure is `{ "error": { "code", "message" } }`. The stable
code controls UI behavior; the safe message may be shown but is not a parsing
contract. Retain `X-Request-ID` as a support reference without retaining the
request body, token, App ID, review, customer data, or secret.

| HTTP/result | Required frontend behavior |
| --- | --- |
| `400` validation or cursor error | Keep editable safe input, highlight the invalid field/query, and do not retry unchanged data. Clear a malformed stored cursor and restart its list query. |
| `401 unauthorized` | Run at most one serialized refresh. If it fails, clear tokens, CPO state, streams, and cached private data before returning to login. |
| `403 forbidden`, `password_change_required`, or App-ID mismatch | Stop the action. Re-bootstrap `me` and `access/me`; remove inaccessible navigation. App-ID mismatch clears the administrative session rather than retrying another tenant. |
| `404` CPO-owned resource | Display one tenant-safe unavailable state. Do not reveal whether the ID was malformed, missing, or belongs to another CPO. |
| `409` business conflict | Preserve the form/list and use the specific code to guide resolution. Examples include tariff/root conflicts, in-use deletion, serial conflict, and idempotency conflict; never overwrite server state locally. |
| `429` or `503` | Stop automatic loops, preserve safe user input, and provide a bounded manual retry. A `503` never means a provider or charger request was accepted. |
| `500` or network loss | Treat the external effect as unknown. For reads, keep last good data as stale and retry explicitly; for ordinary mutations, refetch first and require a new deliberate user action unless the route's idempotency contract permits same-key replay. |

## Recommended application layout

| Screen | Primary data/actions | Important rule |
| --- | --- | --- |
| Login and OTP | administrative auth endpoints | Login scope is `CPO`; customer auth is separate. |
| Dashboard | analytics, fleet operations, current subscription | Live state is a projection; display freshness. |
| Organization and my profile | organization, admin profile, subscription | Organization legal identity is platform-managed/read-only. |
| Staff and permissions | catalog, staff list/detail/lifecycle | Gate each action from effective staff capabilities; creation/delegation requires both `staff.manage` and `staff.permissions.manage`. |
| Hubs and chargers | network CRUD, visibility, assignment, static status | CMS configuration is not OCPP transport truth. |
| Commercial | GST, nested tariffs, user groups, settings | Use decimal strings; follow tariff/GST precedence. |
| Customers and reports | customers, customer ratings (`GET /cpo/customer-ratings`), sessions, charger transactions, wallet ledger | Requires effective `customers.read`; review pages use the documented legacy or generic keyset cursor, retain every filter through continuation, and render customer email/review text as private plain text. No CPO rating mutation is exposed. |
| Operations | fleet, charger live detail, replay, SSE | Replay then stream; REST remains authoritative. |
| Integrations | provider metadata and credential replacement/removal | Never render or expect secret readback. |
| Support | CPO ticket list/create/detail/reply | CPO can see only its own conversation. |

## Complete CPO route inventory

All following operations require the CPO administrative headers above and the
capability declared by their route family. The OpenAPI document owns field
schemas; `GET /cpo/access/me` is the frontend's authority snapshot.

| Area | Operations | UI and recovery rule |
| --- | --- | --- |
| My administrator identity and access | `GET`, `PATCH /cpo/admin/profile`; `GET /cpo/users/{user_id}`; `GET /cpo/access/me`, `/cpo/access/permissions` | Profile changes global login identity fields allowed by the contract, not CPO role/email. Access responses are current authority snapshots; point lookup is tenant-safe, not a directory. |
| Organization and subscription | `GET /cpo/organization`; `GET /cpo/subscription`; `GET /cpo/analytics` | Treat organization identity as read-only. Subscription is informational for CPO; platform renewal is not a CPO action. |
| Staff catalog and lifecycle | `GET /cpo/permissions/catalog`; `GET`, `POST /cpo/staff`; `GET`, `PATCH /cpo/staff/{membership_id}`; `POST /cpo/staff/{membership_id}/activate`, `/suspend`, `/revoke` | Load catalog before editing overrides. Send only known keys once with `ALLOW`/`DENY`; a deny wins. Confirm suspend/revoke and show reason/audit effect. |
| Hubs | `GET`, `POST /cpo/hubs`; `GET`, `PATCH`, `DELETE /cpo/hubs/{hub_id}`; `PUT /cpo/hubs/{hub_id}/customer-visibility`; `GET`, `POST /cpo/hubs/{hub_id}/chargers` | Create hidden; publish only after the required active hub-root tariff exists. Assignment is same-CPO and idempotent at the target. |
| Hub GST | `GET`, `POST`, `PATCH`, `DELETE /cpo/hubs/{hub_id}/gst` | GST is hub/location-owned, independent of tariff. Show a conflict/error instead of fabricating tax. |
| Chargers and connectors | `GET`, `POST /cpo/chargers`; `GET`, `PATCH`, `DELETE /cpo/chargers/{charger_id}`; `GET /cpo/chargers/{charger_id}/image`; `GET`, `PUT /cpo/chargers/{charger_id}/status`; `PUT /cpo/chargers/{charger_id}/customer-visibility` | `charger_id` is the human/OCPP-facing identifier; the CMS UUID is separate. Every CPO charger projection includes canonical `average_rating`/`rating_count`. Static CMS status is not live OCPP availability. Deletion can fail with `charger_in_use`. |
| Hub tariffs | `GET`, `POST /cpo/hubs/{hub_id}/tariffs`; `GET`, `PATCH`, `DELETE /cpo/hubs/{hub_id}/tariffs/{tariff_id}` | URL fixes the immutable target. Follow the root-tariff-before-publication sequence. |
| Charger tariffs | `GET`, `POST /cpo/chargers/{charger_id}/tariffs`; `GET`, `PATCH`, `DELETE /cpo/chargers/{charger_id}/tariffs/{tariff_id}` | Charger tariff overrides hub tariff only in customer tariff precedence. |
| User-group tariffs | `GET`, `POST /cpo/user-groups/{user_group_id}/tariffs`; `GET`, `PATCH`, `DELETE /cpo/user-groups/{user_group_id}/tariffs/{tariff_id}` | Group tariff overrides charger and hub tariff; never put a target ID in the body. |
| GST profiles | `GET`, `POST /cpo/gsts`; `GET`, `PATCH /cpo/gsts/{gst_id}` | Decimal monetary/tax values are strings; do not use JavaScript floats. |
| User groups | `GET`, `POST /cpo/user-groups`; `GET`, `PATCH`, `DELETE /cpo/user-groups/{user_group_id}`; `POST /cpo/user-groups/{user_group_id}/members`; `DELETE /cpo/user-groups/{user_group_id}/members/{customer_id}` | Membership changes affect future tariff selection only; do not rewrite settled sessions. |
| Settings and invoice logo | `GET`, `POST`, `PUT /cpo/settings`; `GET /cpo/settings/invoice-logo` | POST and PUT are replacement/upsert forms. Refresh settings after either. |
| Customers and vehicles | `GET /cpo/customers`, `/cpo/customers/{customer_id}`, `/cpo/customers/{customer_id}/wallet-transactions`, `/cpo/customers/visit-counts`, `/cpo/customer-ratings`, `/cpo/vehicles` | Customer, review, vehicle, usage, and wallet data are read-only CPO projections requiring `customers.read`. Do not expose customer authentication, wallet, vehicle, or rating-mutation controls here. |
| Charging/reporting and diagnostics | `GET /cpo/charging-sessions`, `/cpo/charging-sessions/{session_id}`, `/cpo/charging-sessions/{session_id}/trace`, `/cpo/charging-traces/{trace_id}`, `/cpo/charging-traces/{trace_id}/stream`, `/cpo/charger-transactions`, `/cpo/wallet-transactions`; invoice GET/recovery routes | Use cursors unchanged. Historical session views include frozen `price_per_unit`, `unit`, GST rates, and optional customer `start_criteria`/`requested_limit_value`. Invoice download requires financial finality and uses the session UUID, never a client invoice ID. CPO-only `delivery_status` is separate from readiness. Recovery requires `settings.manage`, explicit duplicate-delivery confirmation, and returns queued work rather than SMTP success. Diagnostic trace data is privileged support evidence, not live-control authority. |
| Operational projection and realtime | `GET /cpo/operations/fleet`; `GET /cpo/operations/chargers/{charger_id}`; `GET /cpo/operations/events`; `GET /cpo/operations/realtime/stream`; `GET /cpo/operations/live-sessions`; `GET /cpo/operations/live-sessions/snapshot` | The live-session primary route is full-snapshot SSE: replace the table from each frame. Every row combines the canonical normal-session static context (nested customer/charger/connector, first SoC, frozen tariff/tax, and start criteria) with duration, meter/SoC status, and current projected amount. Legacy flat display fields remain compatible. It needs no event replay or per-update REST refresh. General fleet/charger streams remain invalidation-based. |
| Provider integrations | `GET /cpo/integrations`; `GET`, `PUT`, `DELETE /cpo/integrations/{provider}` | PUT submits credentials for encryption but returns metadata only. Never display, log, or expect provider secret plaintext. |
| Support | `GET`, `POST /cpo/support`; `GET /cpo/support/{ticket_id}`; `POST /cpo/support/{ticket_id}/replies` | `support.read` views queue/history; `support.create` opens a ticket independently; reply requires **both** `support.read` and `support.reply` because it returns the complete conversation. |
| CPO notifications | `GET /cpo/notifications`; `POST /cpo/notifications/{notification_id}/read` | Any active CPO membership with the app ID may use these personal notifications. Poll/refetch after relevant platform actions; there is no notification SSE stream. |

## Application bootstrap, state ownership, and generic list behavior

The CPO application has one authenticated administrative session and one
server-selected tenant context. A safe boot sequence is:

1. Complete login and OTP, then persist the returned token pair atomically.
2. Fetch `GET /api/v1/auth/me`; require `scope: "CPO"`, active CPO and
   membership state, and retain the returned `cpo_app_id` only as the header
   value.
3. Fetch `GET /api/v1/cpo/access/me` before rendering protected navigation.
   It is the current effective-permission snapshot, not a role guess.
4. Fetch only the initial route's REST data. Load the permission catalog before
   presenting a staff-override editor, not as a substitute for `access/me`.
5. Start the single appropriate realtime connection only after its first REST
   snapshot has completed. Reconnect after token replacement, tab resume, or
   cursor recovery as described below.

The internal CMS UUID, a public charger code, and an OCPP identity are different
identifiers. Use the exact identifier required by each route: administrative
charger detail uses the public six-character `charger_id`; typed charger
operations use the CMS charger UUID; connector operations use the CMS connector
UUID. Never infer one from a display label or attempt to generate an OCPP
identity in the browser.

For any standard newest-first list, treat `{before, before_id}` as one opaque,
exclusive keyset pair. Preserve the complete query when requesting its next
page, do not use offsets, and discard accumulated pages after any filter,
sort, tenant, or authority change. The server may omit both next fields when
`has_more` is false. A list view should distinguish initial loading, a valid
empty result, loaded data, continuation loading, and stale/retryable data;
empty is never evidence that an operation succeeded or that permission exists.

For a mutation, disable duplicate submission, send exactly one final request,
render the returned resource immediately, and refetch collections whose sort,
membership, aggregate, or derived visibility can have changed. `204` has no
body: remove/refresh only after that response, not optimistically before it.
Never retry a non-idempotent administrative mutation after an unknown network
outcome. Typed charger operations are the exception only because their explicit
`Idempotency-Key` makes the same payload/key replay-safe.

All displayed timestamps are UTC instants formatted locally. Keep money, tax,
energy, and tariff decimals as strings until a decimal library formats them.
Server text, customer details, ticket content, reviews, and integration labels
are untrusted presentation text. Do not render them as HTML or send them to
analytics, client logs, URLs, or crash reports.

## Network, commercial, and customer workflows

### Hubs, chargers, connectors, and publication

A hub is the CPO's location and commercial publication root. Create it hidden;
the UI must not offer a direct public creation path. Its `customer_visible`
flag is a CMS publication gate, not live charger reachability. A published
charger must also be attached to a published hub and have its own visibility
gate enabled. Unpublishing removes discovery/favorite eligibility for future
customer reads but does not rewrite existing sessions.

Hub create/edit sends only the documented name, address, state, coordinates,
opening-hours, and sanctioned-load fields. A hub deletion returns `204` only
after its chargers are unassigned; it does not delete those chargers. A hub
with tariffs returns `409 hub_has_tariffs`, so direct the operator to resolve
commercial configuration first. Charger-to-hub assignment takes the target
hub UUID in the URL and one same-CPO CMS charger UUID in the body; reassigning
to the current hub is an intentional idempotent no-op. It never moves tariffs
between targets.

Charger create is `multipart/form-data`: encode the JSON charger request in the
`data` field and an optional image in `charger_image`. Do not send JSON with a
manually invented multipart boundary. The form needs the required operational
metadata and at least one connector; connector numbers are positive and unique
inside the request, and each connector has its own capacity. `hub_id` is
optional at creation. The response supplies the server UUID, public
`charger_id`, OCPP mapping value, generated connection URLs, connector UUIDs,
and initial administrative states. Creating a CMS charger does not connect it
to HAL or prove physical/OCPP availability.

Edit only fields allowed by the route and retain absent optional fields rather
than sending guessed nulls. The CMS static charger status (`ACTIVE`,
`INACTIVE`, `SUSPENDED`, `UNDERMAINTENANCE`, or `DECOMMISSIONED`) is a
commercial/administrative state. Its status route may include an
`ocpp_identity`, but a supplied value must match the stored mapping; never use
it to retarget a charger. Deletion can return `409 charger_in_use`; do not
hide the charger locally until the server confirms `204`.

### Tariff, GST, settings, and customer admission

Tariff and GST are independent. A tariff targets exactly one Hub, Charger, or
User Group. Customer tariff precedence is `UserGroup > Charger > Hub`; GST is
resolved from the selected charger's hub independently of the winning tariff.
A CPO UI must therefore never send `gst_id` on a tariff request or calculate a
customer's final charge from a currently edited tariff.

Current supported tariff combinations are `fixed/energy/kwh`,
`fixed/time/minutes`, and `fixed/sessions` with omitted `units`.
`price_per_unit` is an exact decimal per kWh, completed minute, or completed
session respectively. `watt/hour` is historical compatibility data, not a new
tariff option. The CMS has no authoritative idle lifecycle, so active tariff
requests must use zero/omitted `idle_fee_per_min`; a non-zero active value is
rejected as `idle_fee_unsupported`.

The safe hub-publication sequence is: create hidden hub, create one active
unbounded Hub-root tariff, then set hub customer visibility. A visible Hub
without that root is rejected with `409 hub_tariff_root_required`. A target is
fixed by its URL, cannot be moved by a patch, and is protected by temporal
rules: root, open fallback, and nested bounded intervals must remain valid;
conflicts return `409 tariff_temporal_conflict`. `PATCH` is partial except that
changing pricing basis must send its complete final
`tariff_type`/`price_type`/`units` representation; `units: null` deliberately
clears it for a session tariff, and both schedule values null clear a schedule.
Historical sessions keep their frozen tariff/GST data even after later edits.

Create GST profiles with a non-empty name/state and exact 0..100 decimal
SGST/CGST/IGST components. A profile represents either same-state SGST+CGST or
interstate IGST; do not offer a form that sends both non-zero combinations.
Hub GST assignment is location-owned. Missing or invalid commercial
configuration must be surfaced as a server conflict/error, never repaired by
frontend tax arithmetic.

Settings POST/PUT are multipart upserts. They can set text invoice notes, an
optional validated PNG/JPEG invoice logo, and whole-currency
`wallet_min_balance`/`wallet_buffer_min_balance`. Omitted fields preserve the
stored setting. The logo endpoint is an authenticated binary read, not a
public file URL. The wallet policy affects future customer-start admission;
it is not a ledger balance and cannot be represented with a JavaScript float.

### Staff, customers, user groups, reporting, and support

The permission catalog is the only valid editor vocabulary. Staff creation and
delegated permission changes require the documented management capabilities;
do not let an `ADMIN` role label bypass a current explicit `DENY`. An override
is one known key with `ALLOW` or `DENY`; omission returns it to role default.
Suspend/revoke is a consequential lifecycle action: confirm it, show the
reason/audit effect, and re-bootstrap authority afterward because active CPO
sessions can be revoked.

Customer directory, visit counts, wallet transactions, session history,
charger transactions, ratings, invoices, and diagnostic traces are CPO-scoped
read/support surfaces, not customer-account administration. They never permit
password, wallet, rating, or customer-profile mutation. Maintain their cursor,
time-range, and filter state in the URL or view model without exposing customer
email/phone/review text in telemetry. Customer/session/invoice data is durable
CMS evidence; never replace a session's frozen money, tax, or tariff with the
current commercial configuration.

User-group membership changes are same-CPO operations. Assigning a current
member and removing an absent one are idempotent, but a customer assigned to a
different group produces `409 customer_already_in_group`. Group tariff changes
affect future tariff selection only. A group that still owns tariffs cannot be
deleted (`409 user_group_in_use`).

Support tickets are tenant-private durable conversations. `support.read`
lists/reads, `support.create` opens, and a reply requires both `support.read`
and `support.reply` because the response includes the conversation. Use a
fresh client idempotency key for each message where the route requires it; do
not retry an uncertain send with a new key. Integration credential PUT accepts
secret material for encryption but later GET responses are metadata only.
Never display, preserve, or expect a plaintext secret after save; DELETE
removes the CPO's stored credential configuration, not an external provider
account.

## Commercial and network invariants the UI must preserve

### Charging-session stop provenance

CPO charging-session list/detail and charger-transaction responses may include
an additive `stop` object. Use `stop.requested_initiator` and
`stop.requested_reason` for the actual requested-stop actor/policy and
machine-readable reason; use `stop.ocpp_reason` for the charger-reported OCPP
`StopTransaction.reason`. `Remote` describes protocol delivery, not the
business cause. A spontaneous charger stop may therefore contain only
`stop.ocpp_reason`. Existing session `stop_reason` and transaction `reason`
remain legacy compatibility fields; do not infer or synthesize canonical stop
provenance from them.

The charging-session list supports `sort_by`/`sort_order`, generic
`cursor_value`/`cursor_id` keyset pagination, and tenant-scoped equality and
range filters. A generic cursor is a pair: preserve the same sort, filters,
and both returned cursor values. Do not send it with legacy
`before`/`before_id`; that legacy cursor is only for the historical newest-first
created-at list (`before_id` is the optional equal-timestamp tie-breaker). For duration sorting or duration filters, echo the response
`as_of` on every continuation page. An `end_time` cursor may be the string
`null` for an open session.

`total_kwh` for an open `START_PENDING`, `ACTIVE`, or `STOP_PENDING` session
is the latest durable meter delta, not a final settlement value. The server
applies usage filtering and sorting to that same displayed value. Continue to
treat all decimal strings as exact decimals rather than JavaScript numbers.

- A hub tariff root is the publication prerequisite. The safe path is hidden hub
  → enabled unbounded hub tariff → customer visibility. Do not optimistically
  present a hub as public before the server accepts it.
- Tariff precedence is `UserGroup > Charger > Hub`; GST resolves independently
  from the selected charger's hub. Current tariff values are future commercial
  policy, while settled sessions retain snapshots.
- For a historical charging session, render its returned tariff/GST fields as
  session-time commercial evidence. Do not replace them with the currently
  configured tariff or hub GST. `start_criteria` records the customer's
  `AUTO`, `ENERGY`, `TIME`, or `MONEY` request; it does not tell the UI how the
  tariff bills the completed session.
- Use exact decimal strings for money, GST rates, and kWh amounts. Never add,
  compare, or round currency with binary JavaScript numbers.
- CPO legal identity is platform-owned. A CPO UI may display GSTIN/state/PIN but
  cannot edit them; only the SuperAdmin profile replacement route can do so.
- Customer visibility and static charger status are commercial/CMS publication
  controls. They never claim that the charger is physically connected or OCPP
  `Available`.

### Charger rating summary, filtering, and ordering

Every CPO `ChargerResponse`/`ChargerView` projection, including the primary
charger list, one-charger detail, hub charger list, hub detail, and operational
charger projections, has the same rating summary:

```ts
type ChargerRatingSummary = {
  average_rating?: number; // rounded to at most 2 decimals
  rating_count: number;    // always present; 0 for an unrated charger
};
```

It is the current-CPO read-time aggregate of session-owned
`overall_rating` rows only. Never calculate it from `station_rating` or
`charger_rating`, infer it from optional review text, cache a client-side
counter, or expose a reviewer through a charger card. A customer replacement
rating naturally changes the next read; there is no aggregate mutation or
invalidation endpoint.

`GET /cpo/chargers` is bounded (default `limit=50`, maximum `200`) and accepts
`q`, `min_average_rating`, `max_average_rating`, `has_ratings`, `sort_by`, and
`sort_order`. Rating bounds are inclusive `1..5`, the minimum cannot exceed
the maximum, and an unrated charger does not match a bound. `has_ratings=true`
means `rating_count > 0`; `false` means exactly zero and cannot be combined
with an average bound. `sort_by` is `created_at` (default), `average_rating`,
or `rating_count`; `sort_order` is `asc` or `desc` (rating sorts default to
descending).

Created-at traversal retains `before` plus `before_id`. Rating sorts use
`cursor_value` plus `cursor_id`, with the charger UUID as the tie-breaker.
Retain the complete original query when following any cursor. For
`average_rating`, unrated chargers are `NULLS LAST` in either direction, and
literal `cursor_value=null` continues the final unrated segment. Do not use
offsets or re-sort the page in the browser.

`GET /cpo/hubs/{hub_id}/chargers` accepts the same rating filters and ordering
fields but intentionally remains an unpaged, return-all hub collection. Do
not attach `limit`, `before`, or generic cursor fields to that route, and do
not assume the primary list's pagination envelope changes the hub-list
contract.

### Customer ratings and reviews

`GET /cpo/customer-ratings` is a `customers.read` CPO-only collection. Its
tenant is derived from the CPO bearer/session and matching app ID; the client
never sends `cpo_id`. It returns flattened customer, charger, and hub display
identity plus one rating row. Treat `customer_email` as personal data and
`reason` as untrusted plain text: do not render it as HTML, include it in a
public map/card, or expose it to a customer-facing discovery screen.

Filters are `customer_id`, internal charger UUID `charger_id`, `hub_id`,
`session_id`, inclusive `min_overall_rating`/`max_overall_rating`,
`min_station_rating`/`max_station_rating`,
`min_charger_rating`/`max_charger_rating`, and `has_review`. Every rating
bound is an integer 1 through 5 and each minimum must not exceed its matching
maximum. `has_review=true` means a meaningful persisted `reason`; `false`
means absent or historically blank text.

Sort with `created_at` (default), `updated_at`, `overall_rating`,
`station_rating`, or `charger_rating`, and `sort_order=asc|desc`. The legacy
created-at traversal uses `before`/`before_id`; every other sort uses
`cursor_value`/`cursor_id`. Optional station/charger scores sort `NULLS LAST`;
their cursor value `null` continues the final absent-score segment. Preserve
all filters and sort settings across continuation, and never combine legacy
and generic cursors.

When a rating has a session, its `session` object is durable CMS context, not
live HAL state. Its complete frontend type is defined in the next section; it
contains durable session status/times, exact final-or-projected CMS totals,
currency, settlement status, and optional connector context.

It is intentionally absent for legacy nullable-session rows. Do not invent a
session from charger metadata or issue one session request per review row.
There is no CPO review deletion, reply, moderation, public reviewer identity,
or CPO rating mutation in this contract.

### Complete rating-screen data contract and request discipline

Use the server fields directly. These TypeScript shapes cover a review table,
filter state, detail drawer, and keyset continuation. Timestamps are UTC
instants; energy and money remain exact decimal strings.

```ts
type CPORating = {
  id: string;
  cpo_id: string;
  customer_id: string;
  customer_name: string;
  customer_email: string;
  charger_id: string;       // CMS UUID: valid CPO filter value
  charger_code: string;     // public six-character display identifier
  charger_name: string;
  hub_id?: string;
  hub_name?: string;
  session_id?: string;
  overall_rating: 1 | 2 | 3 | 4 | 5;
  station_rating?: 1 | 2 | 3 | 4 | 5;
  charger_rating?: 1 | 2 | 3 | 4 | 5;
  reason?: string;
  created_at: string;
  updated_at: string;
  session?: CustomerRatingSession;
};

type CustomerRatingSession = {
  id: string;
  status: string;
  start_time: string;
  end_time?: string;
  total_kwh: string;
  total_amount: string;
  currency: string;
  settlement_status: string;
  connector?: { id: string; number: number; type: string };
};

type CPORatingPage = {
  ratings: CPORating[];
  next_before?: string;
  next_before_id?: string;
  next_cursor_value?: string;
  next_cursor_id?: string;
  has_more: boolean;
};

type CPOChargerPage = {
  chargers: Array<ChargerRatingSummary & { id: string }>;
  next_before?: string;
  next_before_id?: string;
  next_cursor_value?: string;
  next_cursor_id?: string;
  has_more: boolean;
};
```

Construct a fresh `URLSearchParams` from current filter state for each first
page. Include only selected filters. Serialize booleans as `true`/`false`,
UUIDs as strings, and a generic null cursor as literal text `null`; do not let
JavaScript serialize a null cursor by omitting it. On continuation, copy every
original selection and add exactly one cursor pair:

```text
# default newest-first review list
GET /api/v1/cpo/customer-ratings?has_review=true&before=2026-09-22T10%3A15%3A00Z&before_id=3e1c...

# optional-dimension ordering in its final absent-score segment
GET /api/v1/cpo/customer-ratings?sort_by=charger_rating&sort_order=desc&cursor_value=null&cursor_id=3e1c...

# primary charger list ordered by aggregate rating
GET /api/v1/cpo/chargers?min_average_rating=4&sort_by=average_rating&sort_order=desc&cursor_value=4.25&cursor_id=3e1c...
```

Do not attach a cursor to a first request, copy a `before` pair into a generic
sort, use page numbers/offsets, or derive a cursor from a rendered cell. When
filters, sort, current CPO app, or effective permission change, discard rows
and cursors before refetching.

### Rating workflows and UI states

The charger directory begins with `GET /cpo/chargers`; filter/rank it using the
canonical aggregate. A hub detail uses its unpaged charger collection and the
same filters/sort only. A Reviews link can carry returned internal
`charger_id`, `hub_id`, or `session_id` into `GET /cpo/customer-ratings`; it
must not carry `cpo_id`, customer email, or public `charger_code` as a server
filter. The screen may cross-link to returned customer/charger/hub/session
context but must tolerate absent session context for legacy rows.

Show initial loading separately from an empty result. Disable or hide review
data/navigation when `customers.read` is absent. On `401`, use one serialized
refresh then clear the session if it fails. On `403`, refresh `access/me` and
remove the protected feature; never call it an empty list. On `404`, show a
tenant-safe unavailable state. On `400`, retain local filter input with a
field-level correction and do not retry unchanged. On `500`/network failure,
retain the last successful page, mark it stale, and retry only the exact user
requested read. Render reviews and all personal fields as text only; never put
them in analytics, map markers, public dashboard cards, or crash reports.

## Typed charger operations and durable recovery

Charger operations are CPO administrative commands, not generic OCPP
passthrough and not customer start/stop. Every action route uses the CMS
charger UUID, requires `chargers.operations`, both ordinary CPO headers, and a
client-generated `Idempotency-Key` no longer than 128 characters. Reuse the
same key only for the same typed payload after a lost response; a different
payload with that key returns `409 idempotency_conflict`. Persist the returned
operation ID and poll its detail route rather than issuing a second physical
command.

| Intended operation | Request route and constrained input | UI truth boundary |
| --- | --- | --- |
| Reset | `POST /operations/chargers/{cms_charger_uuid}/reset`, `SOFT` or `HARD`, optional bounded reason | An OCPP confirmation is not proof of a later physical reboot. |
| Unlock connector | `POST .../unlock`, one same-CPO CMS connector UUID | A protocol acknowledgement is not proof that a cable is physically released. |
| Change availability | `POST .../availability`, `OPERATIVE`/`INOPERATIVE`, optional same-CPO connector UUID | This is an OCPP request, not the CMS customer-visibility flag. |
| Clear cache | `POST .../clear-cache`, no generic passthrough body | Never claim the charger cache is clear before the durable terminal result. |
| Trigger message | `POST .../trigger-message`, only the documented allowlisted OCPP action and optional connector | `follow_on` is diagnostic observation, not causation or session truth. |
| Read/change configuration | `GET` or audited `POST .../configuration/read`; `POST .../configuration` needs `chargers.manage` too | Sensitive values are redacted and must never enter browser persistence. |

The history list `GET /cpo/operations/charger-operations` is a side-effect-free
CMS record and uses its own newest-first `{before,before_id}` pair. Detail
`GET /cpo/operations/charger-operations/{operation_id}` can reconcile an
ambiguous row using the exact same HAL ID, but never emits a new OCPP command.
The exchange route exposes safe grouped OCPP evidence and can legitimately
return an empty `exchanges` collection while asynchronous trace delivery is
pending.

`PERSISTED`, `DISPATCH_CLAIMED`, `DELIVERY_ATTEMPTED`, `HAL_ACCEPTED`, and
`RECONCILIATION_REQUIRED` are not success. Nonterminal records omit
`completed_at`. `OCPP_CONFIRMED` means the HAL received an OCPP response, not
a later physical effect; `CONFIRMED_ABSENT` means exact lookup found no record
and still does not authorize replay. Give ambiguous outcomes an inspectable
"confirming delivery" state with detail/history links. Never auto-retry by
minting a new key, never use operation state to change wallet/session state,
and never promise a device effect that the CMS cannot observe.

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

- each route uses its documented capability; `ADMIN` defaults to every
  registered catalog capability but does not bypass an explicit `DENY`;
- `DENY` takes precedence over every default and `ALLOW` can grant a known
  capability to another active role, subject to the backend delegation checks;
- do not infer route authority from `OWNER`, `OPERATOR`, or `VIEWER`; always
  consume `effective_permissions` and handle a fresh `403` after a permission
  change;
- omit an override to return to the default; never submit duplicate keys.

This is not a frontend-only authorization scheme. The backend evaluates the
current membership and overrides on every protected CPO request.

## CPO versus SuperAdmin difference

The CPO app is tenant-scoped operations and commercial administration. The
SuperAdmin app manages the platform, CPO registration/lifecycle, platform
administrators, subscription records, mail/security/worker operations, and
global support. It must not become a CPO dashboard or a route to tenant secrets.

Never reuse a platform bearer token with `/cpo`, never attach `X-CPO-App-ID`
to `/platform`, and never present a SuperAdmin as an implicit CPO member with
tenant capabilities. Shared visual components must receive an explicit
authority/context input and must not derive CPO access from a platform user.

## Verification checklist for the frontend

- [ ] CPO login sends `scope: "CPO"` and `X-CPO-App-ID`, with no body CPO UUID; OTP/refresh flows
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
- [ ] Charger tables preserve rating filters, sort order, and the matching
  cursor style; hub charger screens stay unpaged.
- [ ] Review screens require `customers.read`, retain all query filters across
  pages, render `reason` as plain text, and show session context only when the
  server supplies it.

## Verification: contract, browser, operational, and release gates

Run the smallest check that proves each claim and record the target/origin,
source revision, time, status/code, and safe request ID. Do not call a route
"verified" because a screen rendered mock data, an API returned `202`, or a
previous deployment record exists.

### Source and contract gate

From this CMS repository root, run before publishing a CPO browser build that
depends on a changed backend contract:

```powershell
.\scripts\verify-docs.ps1
go test ./src/cpo -count=1
go test ./src/routes -run TestOpenAPIContractMatchesRuntimeRoutesAndServesUI -count=1
go test ./...
go vet ./...
git diff --check
```

The documentation verifier, route/OpenAPI parity test, and Go tests are
source-level evidence only. They do not call a configured provider, send mail,
operate a charger, select a disposable PostgreSQL database, or prove a remote
binary. If Windows resource limits prevent a broad Go check, retry it with
bounded parallelism, for example `GOMAXPROCS=2 go test -p 1 ./...`, and record
that exact limitation. Database lifecycle/concurrency tests require an
explicitly inspected disposable `TEST_DATABASE_URL`; never use a shared or
live database.

### Runtime target and schema gate

Select the intended CMS origin explicitly:

```powershell
$cmsOrigin = 'http://127.0.0.1:8080' # replace only with an approved target
Invoke-WebRequest "$cmsOrigin/health/live" -UseBasicParsing
Invoke-WebRequest "$cmsOrigin/health/ready" -UseBasicParsing
Invoke-WebRequest "$cmsOrigin/openapi.yaml" -UseBasicParsing
```

Liveness and readiness must return `200` before CPO workflow testing. The
OpenAPI request returns `200` only when runtime docs are enabled; a `404` can
be deliberate (`API_DOCS_ENABLED=false`). In that case obtain the approved
release artifact/contract before generating types, rather than assuming the
checkout is deployed. Never paste bearer/refresh tokens, OTPs, customer data,
provider credentials, App IDs, raw command payloads, or configuration values
into terminal history, screenshots, tests, or a ticket.

### End-to-end acceptance matrix

Run these only in an authorized non-production tenant with a test admin,
test mail path, test inventory, and, for device operations, an approved test
HAL/charge-point topology. The CMS frontend does not authorize testing on a
real customer's charger/wallet merely because a route is available.

| Workflow | What proves success | Important failure/recovery proof |
| --- | --- | --- |
| CPO login and authority | CPO OTP flow yields `me.scope="CPO"`; `access/me` supplies current effective permissions and matching app context. | Generic login failure does not disclose tenant membership. A refresh cannot switch CPO; an App-ID mismatch removes local session state. |
| Staff lifecycle | Returned membership and fresh `access/me` reflect the intended capability/lifecycle transition. | `403` after a permission change is authoritative; explicit `DENY` remains stronger than role defaults. |
| Hub/charger publication | A hidden hub, valid active root tariff, and explicit visibility update produce the expected CMS projection. | Tariff-root/deletion/in-use conflict leaves server state unchanged; static publication never proves OCPP connection. |
| Commercial configuration | Server returns the saved tariff/GST/settings projection and a customer preview/read follows server precedence. | Invalid decimal/basis/temporal/GST combination returns validation/conflict; browser never computes a substitute tax/final bill. |
| Customer/reporting reads | Current authorized CPO returns only its safe projections; cursor continuation has no duplicate/gap under the selected sort. | `404` remains tenant-safe; reviews/emails remain private text; current configuration never overwrites frozen session values. |
| Invoice delivery recovery | Returned queued/recovery record is visible and later delivery/read state proves the outcome. | Queueing is not SMTP success; do not auto-repeat ambiguous delivery or mutate billing/session state. |
| Operational read/realtime | REST snapshot first, replay/stream follows, and the correct live table semantics are applied. | Expired event cursor triggers full REST reload; stale/unknown projection is rendered honestly, not converted to device truth. |
| Typed charger operation | Durable operation record, exact operation detail/history, and any permitted OCPP evidence show the documented lifecycle. | `DELIVERY_ATTEMPTED`/reconciliation is ambiguous; same-key retry only, no new key/command, and OCPP confirmation is not physical-effect proof. |
| Integration credentials | PUT returns safe metadata; later GET never reveals secret plaintext. | Unknown network outcome is not retried with a new secret blindly; secret values never enter state/logs. |

## Troubleshooting, recovery, and escalation evidence

| Symptom | Collect (safe evidence only) | Safe recovery | Never do |
| --- | --- | --- | --- |
| Browser CORS/preflight failure | Browser status, allowed headers, origin, no credentials | Verify approved environment and whether wildcard CORS was deliberately enabled for that test target. | Move bearer/App ID into a URL or weaken production CORS without authorization. |
| Repeated `401` | HTTP status/code, whether one refresh occurred, frontend build ID | One serialized refresh, then clear private CPO state and log in again. | Parallel refresh/replay loops or reuse a consumed refresh token. |
| `403` after navigation/action | Current `access/me` effective permissions and safe request ID | Re-bootstrap identity/authority; remove or disable the action. | Assume `ADMIN` is sufficient or construct a client-side bypass. |
| `404` for CPO object | Route kind, opaque ID, safe request ID | Show tenant-safe unavailable state and validate navigation source. | Reveal cross-tenant existence or try a different tenant/app ID. |
| Tariff/hub conflict | Error code and returned/current resource where available | Preserve form, explain the specific invariant, refetch, then submit a deliberate corrected request. | Force visibility, invent GST, or delete/recreate historical commercial rows. |
| Live fleet appears wrong | Returned `as_of`, freshness, state, event ID, REST snapshot | Refresh/replay/reconnect by the documented stream contract; show stale/unknown. | Treat a browser timer, static status, or one OCPP message as current truth. |
| Charger operation stuck/ambiguous | Operation ID, kind/state, safe request ID, history/detail/exchange response | Re-fetch operation detail; let exact-ID reconciliation/worker proceed; escalate with IDs. | Submit a new operation key, call HAL from browser, or claim a physical outcome. |
| Invoice/provider delivery unclear | CMS session/operation/recovery ID and returned delivery state | Use the documented server read/recovery route with explicit operator confirmation. | Resend automatically, change settlement, or expose provider data to the browser. |
| Suspected tenant/privacy leak | Endpoint, UI route, safe response metadata, request ID; do not copy PII | Stop display/export, preserve minimal evidence, escalate through the approved security path. | Probe other IDs/tenants, retain leaked content, or put it in a public issue. |

## Maintenance, compatibility, and safe change procedure

Treat a CPO frontend change as a cross-layer contract change whenever it
touches a route, payload, capability, state, pagination, realtime behavior,
commercial invariant, or device-operation lifecycle.

1. Start from the current route/handler and OpenAPI operation, then trace its
   authorization middleware, service, durable persistence/projection, worker
   or HAL boundary, frontend consumer, tests, and this handoff.
2. Classify every claim as source, current runtime, prior deployment, or
   unverified. Do not carry a prior deployed revision/operation count forward
   as evidence for a new release.
3. Preserve tenant derivation and capability enforcement server-side. A CPO
   App ID is context metadata, a role is a default bundle, and a browser
   selection is not authorization.
4. Preserve historical snapshots: completed sessions, tariffs/GST, ratings,
   invoices, operation records, and audit evidence cannot be repaired by
   frontend replacement with current inventory or configuration.
5. Preserve list compatibility: retain legacy `before`/`before_id` behavior
   separately from typed generic cursors; cursor values are opaque and order
   changes require an explicit compatibility plan.
6. For an operation change, maintain idempotency key semantics, durable
   pre-delivery state, exact-ID reconciliation, no-replay ambiguity handling,
   redaction, and the distinction between protocol evidence and physical
   effect. Do not collapse it into a generic command endpoint.
7. Update OpenAPI, human contract, this document, focused/broad tests,
   relevant active work item, project state, and release evidence together.
   Regenerate client types from the exact contract and run the gates above.

Do not run migrations, change runtime configuration, restart/deploy CMS,
rotate a CPO App ID, delete commercial/customer data, send invoice recovery,
or operate a real charge point as a frontend release step unless separately
authorized. Rollback is not a generic answer for persisted sessions, invoices,
or ambiguous charger commands: first inspect the durable record and use the
documented forward recovery/reconciliation path. A frontend rollback is safe
only if the older client continues to understand every persisted API state it
will render.

## Explicit limits and unsupported authority

- This repository contains no CPO browser source/build/deploy procedure; this
  document gives backend integration requirements, not a fictitious frontend
  command.
- It does not prove physical OCPP device effect, real SMTP/provider delivery,
  or disposable PostgreSQL behavior. Those require separately authorized
  runtime acceptance.
- CPO users cannot use platform routes, choose a tenant by UUID, change CPO
  legal identity, mutate customer credentials/wallet/ratings, see provider
  secrets, or call HAL/OCPP directly.
- API docs can be deliberately disabled without disabling business routes;
  docs availability is not a health or authorization indicator.
