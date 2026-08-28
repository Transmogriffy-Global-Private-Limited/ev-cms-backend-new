# SuperAdmin Frontend Integration Handoff

## Purpose

Give this document to the engineer implementing the platform SuperAdmin
frontend. It is a no-chat-history handoff for the complete frontend-consumable
SuperAdmin surface that exists today: administrative authentication, CPO
control, manual subscriptions, personal session security, audit
evidence, worker health, durable event replay, and authenticated SSE.

This document deliberately separates:

- behavior the frontend can integrate now;
- behavior that exists only in backend plans;
- behavior that is partially implemented but cannot yet complete a browser
  workflow; and
- CPO/customer operations that a platform SuperAdmin must never call.

The frontend should not need to inspect Go source to implement the current
surface. When exact schema detail is needed, use these sources in order:

1. `contracts/openapi/openapi.yaml` — authoritative machine-readable HTTP
   contract;
2. this handoff — canonical SuperAdmin frontend workflow and client guidance;
3. `contracts/api/administrative-http-api.md` — exhaustive shared endpoint
   semantics;
4. `guides/workflows/superadmin-support-tickets.md` — complete SuperAdmin
   support-desk lifecycle, REST, recovery, privacy, and known-limit workflow;
5. `contracts/api/superadmin-permission-matrix.md` — manually reviewed
   authority/risk classification for every SuperAdmin API;
6. `SUPERADMIN_CPO_FRONTEND_BOUNDARY.md` — explicit platform-versus-CPO UI
   boundary;
7. `CPO_ADMINISTRATION.md` — focused CPO-screen behavior;
8. `contracts/realtime/platform-events.md` — replay, SSE, ordering, and
   recovery semantics.

Do not infer an endpoint from a database table, Go model, old CMS route, plan,
or mockup. Only routed operations in OpenAPI are callable.

## Integration Snapshot

This handoff was reconciled on 2026-08-25 against the current source tree and
OpenAPI contract. It does not assert which source revision is deployed; verify
the active deployment independently before enabling an environment.

- Development origin: `https://dev-evcmsnew.transev.site`
- Local default origin: `http://127.0.0.1:8080`
- API prefix: `/api/v1`
- Interactive contract: `/docs/`
- Raw OpenAPI: `/openapi.yaml`
- Current source-tree backend contract: 210 HTTP operations across every persona
- Operations used by the SuperAdmin application: 77 API operations
  - 12 shared administrative-authentication operations;
  - 65 `PLATFORM` operations, manually classified in
    `contracts/api/superadmin-permission-matrix.md`, including 17 manual
    subscription operations.

The CPO application has its own ticket-creation/read/reply support operations
under an active CPO session and verified `X-CPO-App-ID` header; they are not a
platform authority path. The platform-only queue/reply/status workflow is
specified in `guides/workflows/superadmin-support-tickets.md`.

The manual subscription API is platform-superadmin-only, excludes feature-key
entitlements, and does not activate provider billing or automatic lifecycle
behavior.

Configure the origin in the frontend environment. Do not hardcode it in API
modules:

```dotenv
VITE_EV_CMS_API_ORIGIN=https://dev-evcmsnew.transev.site
```

Store the origin without a trailing slash. Build paths from the origin; do not
append `/api/v1` twice.

The current development deployment allows cross-origin browser requests. That
is a development policy, not a production guarantee. Production requires an
approved origin policy and HTTPS.

## Readiness Summary

| Capability | FE status | Important boundary |
| --- | --- | --- |
| Platform password + email-OTP login | Ready | Send `scope: "PLATFORM"`; omit `cpo_id` |
| OTP resend | Ready | Replace the old challenge ID with the returned ID |
| Access/refresh rotation | Ready | Refresh token is one-time; coordinate refresh calls |
| `/auth/me` bootstrap | Ready | Reject any response whose scope is not `PLATFORM` |
| Own session list/revoke/logout | Ready | Session list spans every scope for the same global identity |
| Authenticated password change | Ready | Success revokes every session and requires login |
| Forgot/reset password | Ready | Forgot stays generic; an eligible recipient's email contains both recovery ID and code |
| CPO list/search/filter/cursor | Ready | REST is authoritative; reset cursor when filters change |
| CPO slug availability | Ready | Advisory only; creation can still return `cpo_slug_conflict` |
| CPO create/profile/lifecycle/app ID | Ready | Mutations are platform-only; reasons are required where documented |
| Primary-admin inspect/replace/recover | Ready | No password, OTP, token, or mail body is returned |
| CPO administrative-session revocation | Ready | Does not revoke customer or platform sessions |
| Audit log | Ready | Read-only, filtered, newest-first keyset pagination |
| Worker health | Ready | Observational only; no start/stop/retry controls |
| Durable event replay | Ready | At-least-once; deduplicate by event ID |
| Authenticated SSE | Ready | Use `fetch()` streaming, not native `EventSource` |
| Platform-admin governance | Ready | List/invite/grant/activate/deactivate; new identities receive encrypted temporary-password mail; last active authority cannot be removed |
| Locked-user/security operations | Ready | Locked identities, reasoned unlock, security events, and scoped PLATFORM/CPO/ALL session revocation |
| Generic mail operations | Ready | Safe metadata list/detail, retry/cancel, metrics, stale-job reconciliation, and reasoned 30-day-minimum retention |
| Notifications/announcements | Ready | Immutable PLATFORM/CPO audience snapshots, durable recipient rows, platform and CPO list/read APIs |
| Platform overview aggregates | Ready | Bounded CPO/access/session/mail/worker counts and service/database/worker status |
| CPO operational projection and customer intelligence | Ready | Bounded platform views; neither grants CPO impersonation nor secret access |
| Tenant support queue and HAL fact requeue | Ready | Support is a controlled conversation surface; use the complete support-desk workflow, while requeue requires exact-fact confirmation |
| Manual API authority classification | Ready | All platform routes remain server-enforced as `PLATFORM`; the matrix categories are FE risk groupings, not granular RBAC |
| Manual subscriptions | Ready | Plans, issue/renew/status, and history; no feature keys, provider, or automatic lifecycle |
| Platform billing | Intentionally unsupported | No invoice, payment, checkout, or webhook APIs |
| Tenant business data or secret access | Forbidden boundary | A SuperAdmin is not a CPO ADMIN and cannot impersonate one |

## Persona and Authorization Boundary

The backend has three different session planes:

| Persona | Session scope | Tenant context | SuperAdmin FE use |
| --- | --- | --- | --- |
| Platform SuperAdmin | `PLATFORM` | None | Required |
| CPO administrator | `CPO` | One session-bound CPO | Never use for the platform UI |
| App customer | `CUSTOMER` | One customer and CPO | Never use for the platform UI |

The SuperAdmin frontend authenticates through the shared administrative auth
API but always starts login with:

```json
{
  "email": "superadmin@example.com",
  "password": "<current-password>",
  "scope": "PLATFORM"
}
```

Rules the FE must preserve:

- omit `cpo_id` from a platform login request;
- send `Authorization: Bearer <access-token>` on protected operations;
- never send a token in a URL or query string;
- never send `X-CPO-App-ID` on the platform surface;
- never call `/api/v1/cpo/*` or `/api/v1/app/*` with a platform token;
- never present SuperAdmin authority as tenant access;
- never add an impersonation or “open tenant dashboard” feature without a new
  backend contract;
- never display or request tenant Razorpay plaintext, OTPs, temporary
  passwords, refresh tokens, mail ciphertext, or mail bodies.

`X-CPO-App-ID` is a CPO-application routing value used by CPO/customer clients.
It is not a SuperAdmin authorization header, even though the SuperAdmin can
view and rotate the value on a CPO resource.

## Recommended Screen and Route Model

A minimal complete platform application can use this route model:

| FE route | Backend dependencies |
| --- | --- |
| `/login` | `POST /api/v1/auth/login` |
| `/login/verify` | `POST /api/v1/auth/2fa/verify`, `/2fa/resend` |
| `/forgot-password` | `POST /api/v1/auth/password/forgot`; show one generic acknowledgement |
| `/reset-password` | `POST /api/v1/auth/password/reset`; collect the recovery ID and code from the reset email |
| `/platform/cpos` | CPO collection GET and create POST |
| `/platform/cpos/:cpoId` | CPO detail/profile/lifecycle/app-ID operations |
| `/platform/cpos/:cpoId/admin` | Primary-admin read/replace/resend and CPO admin-session revocation |
| `/platform/audit` | `GET /api/v1/platform/audit-logs` |
| `/platform/workers` | `GET /api/v1/platform/workers`, optionally public readiness |
| `/account/sessions` | `/api/v1/auth/me`, `/sessions`, logout operations |
| `/account/password` | authenticated password change |
| `/platform/administrators` | platform administrator list/invite/activate/deactivate |
| `/platform/security` | locked identities, security events, unlock, and session revocation |
| `/platform/mail` | safe mail-job list/detail/retry/cancel, metrics, reconcile, retention |
| `/platform/announcements` | announcement list/create and platform notification list/read |
| `/platform/status` | bounded overview and service status |
| `/platform/subscriptions` | plan catalog create/read/draft/publish/archive |
| `/platform/cpos/:cpoId/subscription` | current subscription, history, and explicit manual lifecycle commands |

Suggested CPO detail regions:

1. Identity and lifecycle summary.
2. Business profile edit form.
3. App-ID transition/rotation.
4. Primary-administrator state and safe onboarding-delivery status.
5. Recovery actions with mandatory reason confirmation.
6. CPO-filtered audit history.

Add a subscription-management area only for platform superadmins, following
`docs/contracts/api/manual-subscriptions.md`. Do not add invoice, payment,
checkout, webhook, automatic-renewal, scheduled-change, or provider UI:
those routes do not exist. Platform-admin management, generic mail jobs,
announcements, notifications, and bounded overview/status are available in the
current source contract. Verify the deployed revision and live OpenAPI before
enabling those screens in a specific environment.

## HTTP Conventions

### Request rules

- Requests with bodies use `Content-Type: application/json`.
- Send exactly one JSON object.
- Unknown object fields are rejected.
- Malformed JSON and a second JSON value are rejected.
- Request bodies are limited to 32 KiB.
- GET and DELETE requests without bodies do not need `Content-Type`.
- Use `Accept: application/json` except for the SSE stream.
- Browser requests use bearer headers, not cookies; `credentials: "omit"` is
  appropriate for the current API.

### Response rules

- UUIDs are strings.
- Times are UTC RFC3339 strings.
- Optional fields are omitted when absent; do not require explicit `null`.
- Empty collections are `[]`.
- Authentication and platform-CPO responses use `Cache-Control: no-store` and
  `Pragma: no-cache`.
- Every response includes a server-generated `X-Request-ID`; permissive CORS
  exposes it. Retain it with failed-operation diagnostics as a copyable support
  reference, never with request bodies or credentials.
- `204 No Content` has no JSON body.
- Do not log raw request/response bodies from auth or recovery operations.

### Error envelope

Every handled API failure uses:

```ts
export interface ApiErrorEnvelope {
  error: {
    code: string;
    message: string;
  };
}
```

Use `error.code` for program logic. Display the safe server `message` where it
helps, but do not infer hidden identity, membership, lockout, or mail state
from generic authentication errors.

## Current-Source Governance and Operations

The following source-tree operations are ready for frontend integration. They
require `PLATFORM` bearer authority unless marked CPO notification:

```text
GET/POST /api/v1/platform/administrators
POST /api/v1/platform/administrators/{user_id}/activate|deactivate
GET /api/v1/platform/security/locked-identities
GET /api/v1/platform/security/events
POST /api/v1/platform/security/users/{user_id}/unlock
POST /api/v1/platform/security/users/{user_id}/sessions/revoke
GET /api/v1/platform/mail/jobs
GET /api/v1/platform/mail/jobs/{job_id}
POST /api/v1/platform/mail/jobs/{job_id}/retry|cancel
GET /api/v1/platform/mail/metrics
POST /api/v1/platform/mail/reconcile|retention
GET/POST /api/v1/platform/announcements
GET /api/v1/platform/notifications
POST /api/v1/platform/notifications/{notification_id}/read
GET /api/v1/platform/overview
GET /api/v1/platform/status
GET /api/v1/cpo/notifications
POST /api/v1/cpo/notifications/{notification_id}/read
```

Reasons are trimmed and must be 3–500 characters. Governance commands are
audited. Administrator deactivation revokes platform sessions and cannot leave
zero active platform administrators. Session revocation accepts `PLATFORM`,
`CPO`, or `ALL`; CPO scope requires `cpo_id`. Mail APIs return only safe
metadata and an `error_present` boolean. Retention deletes only `SENT` or
`CANCELED` jobs older than the supplied cutoff, which must be at least 30 days
old.

Announcements use `PLATFORM` or `CPO` audience. A CPO announcement requires a
CPO ID; a platform announcement forbids one. The backend snapshots eligible
recipients transactionally, so later membership changes do not rewrite the
original audience. Platform notification list/read derives the recipient from
the bearer session. CPO notification list/read requires the verified
`X-CPO-App-ID` header and derives the tenant from the CPO session. Overview and
status are bounded aggregates, not tenant business-data exports.

These SuperAdmin routes are present in the current 211-operation source OpenAPI.
The complete manual API-by-API authority and risk classification is
`contracts/api/superadmin-permission-matrix.md`. The CPO ADMIN-only charger
hub-assignment operation is not callable by SuperAdmin. The CPO user
point-lookup remains a tenant CPO ADMIN operation and is not callable by
SuperAdmin.

## TypeScript Contract

These handwritten types cover the SuperAdmin surface. OpenAPI remains the
machine authority if the backend contract changes.

```ts
export type UUID = string;
export type RFC3339 = string;
export type AuthScope = "PLATFORM" | "CPO" | "CUSTOMER";
export type CpoStatus = "PENDING" | "ACTIVE" | "SUSPENDED";
export type CompanyType = "INDIVIDUAL" | "COMPANY";
export type CpoAppIdMode = "DUMMY" | "LIVE";
export type MembershipStatus = "ACTIVE" | "SUSPENDED" | "REVOKED";
export type MailStatus = "PENDING" | "PROCESSING" | "SENT" | "FAILED" | "CANCELED";
export type WorkerStatus = "HEALTHY" | "DEGRADED" | "STALE" | "DISABLED";

export interface ChallengeResponse {
  challenge_id: UUID;
  expires_at: RFC3339;
  resend_available_at: RFC3339;
}

export interface TokenResponse {
  access_token: string;
  access_token_expires_at: RFC3339;
  refresh_token: string;
  session_expires_at: RFC3339;
  token_type: "Bearer";
  must_change_password: boolean;
  // Present only for CPO scope; absent for the SuperAdmin PLATFORM scope.
  cpo_app_id?: string;
  cpo_app_id_mode?: CpoAppIdMode;
}

export interface AuthUser {
  id: UUID;
  email: string;
  full_name: string;
  is_verified: boolean;
  mfa_enabled: boolean;
  must_change_password: boolean;
  last_login_at?: RFC3339;
}

export interface PlatformMeResponse {
  user: AuthUser;
  scope: "PLATFORM";
  // cpo_id, role, cpo_app_id, and cpo_app_id_mode must be absent.
}

export interface AuthSession {
  id: UUID;
  scope: AuthScope;
  cpo_id?: UUID;
  role?: "ADMIN" | "OWNER" | "OPERATOR" | "VIEWER";
  ip_address?: string;
  user_agent: string;
  created_at: RFC3339;
  last_seen_at: RFC3339;
  expires_at: RFC3339;
  is_current: boolean;
}

export interface Cpo {
  id: UUID;
  slug: string;
  business_name: string;
  company_type: CompanyType;
  gstin: string;
  address: string;
  city: string;
  state: string;
  pincode: string;
  status: CpoStatus;
  status_reason: string;
  status_changed_at: RFC3339;
  status_changed_by_user_id?: UUID;
  app_id: string;
  app_id_mode: CpoAppIdMode;
  app_id_updated_at: RFC3339;
  created_at: RFC3339;
  updated_at: RFC3339;
}

export interface InitialAdmin {
  user_id: UUID;
  email: string;
  full_name: string;
  role: "ADMIN";
  identity_created: boolean;
}

export interface CreateCpoResponse {
  cpo: Cpo;
  admin: InitialAdmin;
}

export interface CpoListResponse {
  cpos: Cpo[];
  next_before?: RFC3339;
  next_before_id?: UUID;
  has_more: boolean;
}

export interface CpoSlugAvailability {
  slug: string;
  available: boolean;
}

export interface OnboardingDelivery {
  job_id: UUID;
  template:
    | "CPO_STAFF_NEW_IDENTITY"
    | "CPO_STAFF_EXISTING_IDENTITY"
    | "CPO_ONBOARDING_RESENT";
  status: MailStatus;
  attempts: number;
  sent_at?: RFC3339;
  created_at: RFC3339;
  updated_at: RFC3339;
}

export interface PrimaryAdmin {
  user_id: UUID;
  email: string;
  full_name: string;
  role: "ADMIN";
  membership_status: MembershipStatus;
  identity_active: boolean;
  identity_verified: boolean;
  must_change_password: boolean;
  last_login_at?: RFC3339;
  latest_onboarding_delivery?: OnboardingDelivery;
}

export interface SessionRevocationResponse {
  revoked_sessions: number;
  revoked_refresh_tokens: number;
}

export interface PlatformEvent {
  id: number;
  type: string;
  actor_user_id?: UUID;
  resource_type: string;
  resource_id?: string;
  data: Record<string, unknown>;
  occurred_at: RFC3339;
}

export interface PlatformEventPage {
  events: PlatformEvent[];
  next_cursor: number;
  has_more: boolean;
}

export interface AuditRecord {
  id: UUID;
  cpo_id?: UUID;
  user_id?: UUID;
  action: string;
  entity: string;
  entity_id?: UUID;
  details: Record<string, unknown>;
  created_at: RFC3339;
}

export interface AuditPage {
  records: AuditRecord[];
  next_before?: RFC3339;
  next_before_id?: UUID;
  has_more: boolean;
}

export interface Worker {
  id: UUID;
  name: string;
  instance_key: string;
  status: WorkerStatus;
  required: boolean;
  started_at: RFC3339;
  last_heartbeat_at: RFC3339;
  last_job_completed_at?: RFC3339;
  metadata: Record<string, unknown>;
}

export interface KeysetPage<T> {
  next_before?: RFC3339;
  next_before_id?: UUID;
  has_more: boolean;
}

export interface PlatformAdministrator {
  user_id: UUID;
  email: string;
  full_name: string;
  identity_active: boolean;
  identity_verified: boolean;
  authority_active: boolean;
  status_reason: string;
  status_changed_at: RFC3339;
  status_changed_by_user_id?: UUID;
  created_at: RFC3339;
  updated_at: RFC3339;
}

export interface LockedIdentity {
  user_id: UUID;
  email: string;
  full_name: string;
  locked_until: RFC3339;
}

export interface ReasonRequest {
  reason: string; // trimmed 3-500 characters
}

export interface AdministrativeSessionRevocationRequest extends ReasonRequest {
  scope: "PLATFORM" | "CPO" | "ALL";
  // Required exactly when scope is CPO; forbidden otherwise.
  cpo_id?: UUID;
}

export interface MailJob {
  id: UUID;
  to_email: string;
  cpo_id?: UUID;
  user_id?: UUID;
  template: string;
  status: MailStatus;
  attempts: number;
  max_attempts: number;
  available_at: RFC3339;
  locked_at?: RFC3339;
  error_present: boolean;
  sent_at?: RFC3339;
  created_at: RFC3339;
  updated_at: RFC3339;
}

export interface MailMetric {
  template: string;
  status: MailStatus;
  count: number;
}

export interface Announcement {
  id: UUID;
  audience: "PLATFORM" | "CPO";
  cpo_id?: UUID;
  title: string;
  body: string;
  created_by_user_id: UUID;
  created_at: RFC3339;
  expires_at?: RFC3339;
  recipient_count: number;
}

export interface Notification {
  id: UUID;
  announcement_id: UUID;
  audience: "PLATFORM" | "CPO";
  cpo_id?: UUID;
  title: string;
  body: string;
  created_at: RFC3339;
  expires_at?: RFC3339;
  read_at?: RFC3339;
}

export interface PlatformWorkerStatus {
  name: string;
  status: string;
  required: boolean;
}

export interface PlatformOverview {
  cpos: Record<string, number>;
  active_platform_admins: number;
  active_sessions: number; // administrative plus CPO-local customer sessions
  mail: Record<string, number>;
  workers: PlatformWorkerStatus[];
}

export interface PlatformStatus {
  service: string;
  version: string;
  database: "connected" | "unavailable";
  workers: PlatformWorkerStatus[];
}

export interface SubscriptionPlanTerms {
  currency: string; // uppercase ISO 4217, for example INR
  price_minor: number;
  billing_interval: "MONTHLY" | "YEARLY";
  interval_count: number;
  trial_days: number;
}

export interface SubscriptionPlan {
  id: UUID;
  code: string;
  name: string;
  description: string;
  status: "DRAFT" | "PUBLISHED" | "ARCHIVED";
  created_by: UUID;
  created_at: RFC3339;
  updated_at: RFC3339;
}

export interface SubscriptionPlanVersion extends SubscriptionPlanTerms {
  id: UUID;
  plan_id: UUID;
  version: number;
  status: "DRAFT" | "PUBLISHED";
  published_at?: RFC3339;
  published_by?: UUID;
  created_at: RFC3339;
  updated_at: RFC3339;
}

export interface SubscriptionPlanView {
  plan: SubscriptionPlan;
  draft?: SubscriptionPlanVersion;
  published_versions: SubscriptionPlanVersion[];
}

export interface SubscriptionAction {
  reason: string; // 1-500 characters
  idempotency_key: string; // 1-120 characters; persist before retrying
}

export interface CpoSubscription {
  id: UUID;
  cpo_id: UUID;
  plan_version_id: UUID;
  status: "TRIAL" | "ACTIVE" | "PAUSED" | "PAST_DUE" | "CANCELLED" | "EXPIRED";
  starts_at: RFC3339;
  trial_ends_at?: RFC3339;
  current_period_starts_at: RFC3339;
  current_period_ends_at: RFC3339;
  cancel_at_period_end: false;
  pending_plan_version_id?: never;
  pending_change_at?: never;
  cancelled_at?: RFC3339;
  ended_at?: RFC3339;
  created_by: UUID;
  created_at: RFC3339;
  updated_at: RFC3339;
}

export interface CpoSubscriptionView {
  subscription: CpoSubscription;
  plan: SubscriptionPlan;
  plan_version: SubscriptionPlanVersion;
}

export interface CpoSubscriptionHistory {
  id: UUID;
  subscription_id: UUID;
  cpo_id: UUID;
  previous_status?: CpoSubscription["status"];
  next_status: CpoSubscription["status"];
  previous_plan_version_id?: UUID;
  next_plan_version_id: UUID;
  actor_user_id: UUID;
  reason: string;
  idempotency_key: string;
  effective_at: RFC3339;
  metadata: Record<string, unknown>;
  created_at: RFC3339;
}
```

The event cursor is an OpenAPI `int64` serialized as a JSON number. Current IDs
are ordinary JavaScript-safe integers. Assert `Number.isSafeInteger(event.id)`
and stop with an integration error if that ever becomes false; the backend
contract would need a string cursor before IDs exceed JavaScript's safe range.

## Complete Endpoint Inventory

### Health and contract discovery

| Method and path | Auth | Success | FE use |
| --- | --- | --- | --- |
| `GET /health/live` | None | `200 {"status":"ok"}` | Process reachability only |
| `GET /health/ready` | None | `200 ready` or `503 not_ready` | Database and required-worker readiness |
| `GET /openapi.yaml` | None when docs enabled | `200` YAML | Contract discovery/build input |
| `GET /docs` | None when docs enabled | `307` to `/docs/` | Browser convenience |
| `GET /docs/` | None when docs enabled | `200` Swagger UI | Interactive manual testing |

`API_DOCS_ENABLED=false` removes all three documentation routes and they return
`404`; health and business routes remain registered.

### Administrative authentication and account security

| Method and path | Request | Success | Important errors/behavior |
| --- | --- | --- | --- |
| `POST /api/v1/auth/login` | platform login | `202 ChallengeResponse` | `invalid_credentials`, `rate_limited`, `mail_unavailable` |
| `POST /api/v1/auth/2fa/verify` | challenge ID + 6-digit code | `200 TokenResponse` | `invalid_challenge`; replace local token state |
| `POST /api/v1/auth/2fa/resend` | challenge ID | `202 ChallengeResponse` | cooldown; old challenge becomes invalid |
| `POST /api/v1/auth/refresh` | current refresh token | `200 TokenResponse` | one-time rotation; reuse revokes session |
| `POST /api/v1/auth/password/forgot` | email | `202 generic message` | eligible recipient gets recovery ID, code, and expiry by email |
| `POST /api/v1/auth/password/reset` | recovery ID + code + new password | `200 message` | success revokes every session; sign in again |
| `GET /api/v1/auth/me` | bearer | `200 PlatformMeResponse` | must resolve to `PLATFORM` |
| `GET /api/v1/auth/sessions` | bearer | `200 {sessions}` | includes the identity's PLATFORM and CPO sessions |
| `DELETE /api/v1/auth/sessions/{session_id}` | bearer | `204` | owned session only; may revoke current session |
| `POST /api/v1/auth/logout` | bearer | `204` | revokes current session |
| `POST /api/v1/auth/logout-all` | bearer | `204` | revokes every scope for the global identity |
| `POST /api/v1/auth/password/change` | current/new passwords | `200 message` | revokes every session; sign in again |

### Platform CPO control plane

Every operation below requires a current `PLATFORM` bearer session and no
`X-CPO-App-ID`.

| Method and path | Success | Primary FE purpose |
| --- | --- | --- |
| `POST /api/v1/platform/cpos` | `201 CreateCpoResponse` | Provision pending CPO and primary ADMIN |
| `GET /api/v1/platform/cpos` | `200 CpoListResponse` | Search/filter/cursor collection |
| `GET /api/v1/platform/cpos/slug-availability?slug=...` | `200 CpoSlugAvailability` | Validate and preflight a normalized slug |
| `GET /api/v1/platform/cpos/{cpo_id}` | `200 Cpo` | Authoritative detail refresh |
| `PUT /api/v1/platform/cpos/{cpo_id}/profile` | `200 Cpo` | Replace editable business fields |
| `POST /api/v1/platform/cpos/{cpo_id}/activate` | `200 Cpo` | Reasoned manual access grant |
| `POST /api/v1/platform/cpos/{cpo_id}/suspend` | `200 Cpo` | Reasoned access removal and tenant-session revocation |
| `PUT /api/v1/platform/cpos/{cpo_id}/app-id` | `200 Cpo` | Set/rotate live app ID |
| `GET /api/v1/platform/cpos/{cpo_id}/primary-admin` | `200 PrimaryAdmin` | Safe administrator/onboarding status |
| `PUT /api/v1/platform/cpos/{cpo_id}/primary-admin` | `200 PrimaryAdmin` | Restore or replace responsible administrator |
| `POST /api/v1/platform/cpos/{cpo_id}/primary-admin/resend-onboarding` | `202 PrimaryAdmin` | Credential-free access reminder |
| `POST /api/v1/platform/cpos/{cpo_id}/administrative-sessions/revoke` | `200 counts` | CPO-staff session incident response |

### Platform observation and realtime

| Method and path | Success | Primary FE purpose |
| --- | --- | --- |
| `GET /api/v1/platform/events` | `200 PlatformEventPage` | Durable catch-up/polling/recovery |
| `GET /api/v1/platform/realtime/stream` | `200 text/event-stream` | Low-latency invalidation with replay |
| `GET /api/v1/platform/audit-logs` | `200 AuditPage` | Immutable privileged-action evidence |
| `GET /api/v1/platform/workers` | `200 {workers}` | Current logical-worker health |

### Platform governance, security, and mail operations

All operations in this section require a current `PLATFORM` bearer session.
They expose safe administrative metadata only: never render a control for a
password, OTP, token, decrypted mail payload, or stored mail error text.

| Method and path | Success | Primary FE purpose |
| --- | --- | --- |
| `GET /api/v1/platform/administrators` | `200 AdministratorPage` | Keyset-paginated platform-authority list; `include_inactive` defaults false |
| `POST /api/v1/platform/administrators` | `201 Administrator` | Invite a new identity or grant authority to an existing active identity |
| `POST /api/v1/platform/administrators/{user_id}/activate` | `200 Administrator` | Restore platform authority with a reason |
| `POST /api/v1/platform/administrators/{user_id}/deactivate` | `200 Administrator` | Remove authority, revoke platform sessions, and preserve the global identity |
| `GET /api/v1/platform/security/locked-identities` | `200 LockedIdentityPage` | List currently locked identities |
| `GET /api/v1/platform/security/events` | `200 AuditPage` | Read-only security-event evidence |
| `POST /api/v1/platform/security/users/{user_id}/unlock` | `204` | Unlock an identity with a reason |
| `POST /api/v1/platform/security/users/{user_id}/sessions/revoke` | `200 counts` | Revoke a selected user's `PLATFORM`, `CPO`, or `ALL` sessions |
| `GET /api/v1/platform/mail/jobs` | `200 MailPage` | Safe mail-outbox metadata with keyset pagination and filters |
| `GET /api/v1/platform/mail/jobs/{job_id}` | `200 MailJob` | Refresh one safe mail-job projection |
| `POST /api/v1/platform/mail/jobs/{job_id}/retry` | `200 MailJob` | Requeue an eligible failed mail job |
| `POST /api/v1/platform/mail/jobs/{job_id}/cancel` | `200 MailJob` | Cancel an unsent job with a reason |
| `GET /api/v1/platform/mail/metrics` | `200 {metrics}` | Counts by template and delivery state |
| `POST /api/v1/platform/mail/reconcile` | `200 {requeued}` | Requeue stale processing jobs with a reason |
| `POST /api/v1/platform/mail/retention` | `200 {deleted}` | Delete only terminal jobs older than a supplied 30-day-minimum cutoff |

### Platform communication and status

| Method and path | Success | Primary FE purpose |
| --- | --- | --- |
| `GET /api/v1/platform/announcements` | `200 AnnouncementPage` | Keyset-paginated immutable announcement history |
| `POST /api/v1/platform/announcements` | `201 Announcement` | Create a `PLATFORM` or one-CPO audience snapshot |
| `GET /api/v1/platform/notifications` | `200 NotificationPage` | Current SuperAdmin's durable notification inbox; supports `unread_only` |
| `POST /api/v1/platform/notifications/{notification_id}/read` | `204` | Mark only the current recipient's notification as read |
| `GET /api/v1/platform/overview` | `200 PlatformOverview` | Bounded aggregate dashboard, not a tenant-data export |
| `GET /api/v1/platform/status` | `200 PlatformStatus` | Service/database/worker status display only |

### Support desk

All support operations require a `PLATFORM` bearer session and never a CPO app
ID. The queue returns full CPO conversations rather than a tenant-impersonation
view. Use the complete lifecycle, error/recovery, privacy, and current-limit
rules in `guides/workflows/superadmin-support-tickets.md`; in particular,
`PENDING` does not identify the party expected to reply, and a later reopen
does not clear a previous `closed_at` timestamp.

| Method and path | Success | Primary FE purpose |
| --- | --- | --- |
| `GET /api/v1/platform/support/tickets` | `200 SupportTicket[]` | Load every visible CPO conversation, newest ticket update first, with full message threads |
| `GET /api/v1/platform/support/tickets/{ticket_id}` | `200 SupportTicket` | Refresh one complete conversation |
| `POST /api/v1/platform/support/tickets/{ticket_id}/replies` | `200 SupportTicket` | Append one durable platform reply and set `PENDING`; no idempotency key exists |
| `PATCH /api/v1/platform/support/tickets/{ticket_id}/status` | `200 SupportTicket` | Set `OPEN`, `PENDING`, `RESOLVED`, or `CLOSED`; confirm this durable workflow action |

### Manual subscription management

All operations below are platform-superadmin-only. They are manual commercial
records, not access control: CPO activation and suspension remain the separate
`/platform/cpos/{cpo_id}/activate|suspend` lifecycle. There are no feature
keys, provider checkout, payments, invoices, webhooks, automatic renewal, or
scheduled change endpoints.

| Method and path | Success | Primary FE purpose |
| --- | --- | --- |
| `GET /api/v1/platform/plans` | `200 {plans}` | Load the full plan catalog, draft, and published versions |
| `POST /api/v1/platform/plans` | `201 SubscriptionPlanView` | Create a plan and first draft version |
| `GET /api/v1/platform/plans/{plan_id}` | `200 SubscriptionPlanView` | Refresh a plan and all its versions |
| `PUT /api/v1/platform/plans/{plan_id}/draft` | `200 SubscriptionPlanView` | Create or replace the one editable draft version |
| `POST /api/v1/platform/plans/{plan_id}/publish` | `200 SubscriptionPlanView` | Publish the draft; published versions are immutable and issueable |
| `POST /api/v1/platform/plans/{plan_id}/archive` | `200 SubscriptionPlanView` | Prevent future issue/change-plan selection while retaining history |
| `GET /api/v1/platform/cpos/{cpo_id}/subscription` | `200 CpoSubscriptionView` | Load the CPO's current manual subscription |
| `POST /api/v1/platform/cpos/{cpo_id}/subscription` | `201 CpoSubscriptionView` | Issue the first current subscription from a published plan version |
| `POST /api/v1/platform/cpos/{cpo_id}/subscription/renew` | `200 CpoSubscriptionView` | Explicitly record the next/current period |
| `POST /api/v1/platform/cpos/{cpo_id}/subscription/change-plan` | `200 CpoSubscriptionView` | Immediately change to a published plan version |
| `POST /api/v1/platform/cpos/{cpo_id}/subscription/activate` | `200 CpoSubscriptionView` | Move `TRIAL` to `ACTIVE` explicitly |
| `POST /api/v1/platform/cpos/{cpo_id}/subscription/pause` | `200 CpoSubscriptionView` | Pause an eligible current subscription |
| `POST /api/v1/platform/cpos/{cpo_id}/subscription/resume` | `200 CpoSubscriptionView` | Resume a `PAUSED` or `PAST_DUE` subscription as `ACTIVE` |
| `POST /api/v1/platform/cpos/{cpo_id}/subscription/mark-past-due` | `200 CpoSubscriptionView` | Explicitly mark an eligible subscription past due |
| `POST /api/v1/platform/cpos/{cpo_id}/subscription/expire` | `200 CpoSubscriptionView` | End a current subscription as `EXPIRED` |
| `POST /api/v1/platform/cpos/{cpo_id}/subscription/cancel` | `200 CpoSubscriptionView` | Cancel immediately; cancellation-at-period-end is unsupported |
| `GET /api/v1/platform/cpos/{cpo_id}/subscription/history` | `200 {history}` | Newest-first audited transition history, bounded to 500 |

## Authentication State Machine

### 1. Start login

```http
POST /api/v1/auth/login
Content-Type: application/json
```

```json
{
  "email": "superadmin@example.com",
  "password": "<password>",
  "scope": "PLATFORM"
}
```

On `202`, keep only the challenge ID and the two server timestamps. The response
means the challenge and encrypted mail job committed; it does not prove SMTP
delivery.

Do not reveal whether `invalid_credentials` means the email, password,
platform authority, identity state, or lockout failed. Use one generic error.

### 2. Verify or resend OTP

Verification:

```json
{
  "challenge_id": "5cef4c95-a1da-448e-bd7c-19d570cd4497",
  "code": "123456"
}
```

Resend:

```json
{
  "challenge_id": "5cef4c95-a1da-448e-bd7c-19d570cd4497"
}
```

- Code input is exactly six ASCII digits.
- Disable resend until `resend_available_at` according to server time.
- A successful resend returns a new challenge ID and invalidates the old one.
- Replace the whole challenge state, not only the expiry timer.
- `invalid_challenge` intentionally combines expired, consumed, invalidated,
  wrong-code, and attempt-limit outcomes.

### 3. Establish the platform session

On successful verification:

1. atomically store the access token, its expiry, the current refresh token,
   and session expiry;
2. call `GET /api/v1/auth/me`;
3. require `scope === "PLATFORM"`;
4. reject/clear the session if CPO-only fields appear as the selected scope;
5. if `must_change_password` is true, allow only account password/session
   operations until the password is changed;
6. start the REST bootstrap and realtime loop.

The backend uses bearer tokens, not cookies. For a browser-only SPA, keeping
the access token in memory reduces passive persistence, but no JavaScript
storage choice protects a refresh token from an active XSS compromise. A BFF
with secure HTTP-only cookies provides a stronger browser boundary if the FE
architecture can support one. Whatever design is chosen, never log tokens or
embed them in URLs, analytics, error reports, Redux devtools, or query caches.

### 4. Refresh safely

Refresh tokens rotate and are one-time. The submitted token becomes unusable
as soon as refresh succeeds. Reuse revokes the whole session.

The FE must:

- allow only one refresh request at a time;
- make waiting requests share that promise;
- replace access and refresh tokens as one atomic state update;
- retry an authorization-rejected API request at most once after refresh;
- clear the complete local session on `invalid_refresh_token`;
- coordinate across tabs if tabs share one refresh token; otherwise each tab
  should establish its own session;
- use the returned expiry timestamps instead of assuming configured TTLs.

Schedule a proactive refresh shortly before access expiry, but also recover on
focus and from a protected `401` because background-tab timers are unreliable.
Abort and reconnect SSE with the new access token after refresh.

### 5. Logout and session operations

- `logout` revokes only the current session.
- deleting a listed session revokes that owned session; deleting the current
  session immediately invalidates the current UI and SSE stream;
- `logout-all` revokes every PLATFORM and CPO session belonging to the same
  global identity, not just the current platform scope;
- password change/reset also revokes every session.

Show `scope`, device/user-agent, IP when present, times, and `is_current` in the
session UI. Confirm the broader effect before logout-all.

### 6. Password recovery

`POST /api/v1/auth/password/forgot` is enumeration-safe and returns only a
generic message. For an eligible active identity, the encrypted
`PASSWORD_RESET_OTP` mail contains the opaque recovery ID (`challenge_id`),
six-digit code, and shared expiry. The response deliberately contains none of
those values, so unknown and eligible emails remain indistinguishable.

The FE flow is:

1. submit the email and always show the same acknowledgement;
2. collect the recovery ID, code, and new password on the reset screen;
3. send those values to `POST /api/v1/auth/password/reset`;
4. on success, clear all local authentication state and return to login because
   every session was revoked.

Treat malformed, expired, superseded, consumed, wrong-code, and attempt-limited
inputs as the same `invalid_challenge` outcome. A reset email generated before
this contract was deployed has no recovery ID and cannot be completed; request
a new reset email instead of attempting to recover internal database state.

## Reference API Client Behavior

This pattern handles the shared envelope and `204` responses without assuming
all responses are JSON:

```ts
export class ApiFailure extends Error {
  constructor(
    public readonly status: number,
    public readonly code: string,
    message: string,
    public readonly requestId?: string,
  ) {
    super(message);
  }
}

async function failureFrom(response: Response): Promise<ApiFailure> {
  let code = "http_error";
  let message = `Request failed with HTTP ${response.status}.`;
  const requestId = response.headers.get("X-Request-ID") ?? undefined;
  try {
    const body = (await response.json()) as ApiErrorEnvelope;
    if (body?.error?.code) code = body.error.code;
    if (body?.error?.message) message = body.error.message;
  } catch {
    // A proxy or network edge may return a non-JSON failure.
  }
  return new ApiFailure(response.status, code, message, requestId);
}

export async function requestJson<T>(
  origin: string,
  path: string,
  init: RequestInit = {},
  accessToken?: string,
): Promise<T> {
  const headers = new Headers(init.headers);
  headers.set("Accept", "application/json");
  if (init.body !== undefined) headers.set("Content-Type", "application/json");
  if (accessToken) headers.set("Authorization", `Bearer ${accessToken}`);

  const response = await fetch(`${origin}${path}`, {
    ...init,
    headers,
    credentials: "omit",
  });
  if (!response.ok) throw await failureFrom(response);
  if (response.status === 204) return undefined as T;
  return (await response.json()) as T;
}
```

Use a refresh single-flight around this primitive. Do not automatically retry a
mutation after a network timeout or lost response: current command endpoints
do not accept idempotency keys. A protected `401` occurs before the handler and
may be retried once after refresh; an ambiguous network failure requires an
authoritative GET before deciding whether to submit the command again.

## CPO Collection

`GET /api/v1/platform/cpos` accepts:

| Query | Rule |
| --- | --- |
| `q` | Case-insensitive substring; maximum 200 characters |
| `status` | `PENDING`, `ACTIVE`, or `SUSPENDED` |
| `app_id_mode` | `DUMMY` or `LIVE` |
| `limit` | 1–200; default 50 |
| `before` | Exclusive RFC3339 creation-time cursor |
| `before_id` | UUID tie-breaker; must be paired with `before` |

Search covers business name, slug, GSTIN, app ID, and the primary
administrator's name/email.

Pagination rules:

1. Start without cursor fields.
2. Render `cpos` in returned order; it is newest first.
3. If `has_more`, keep both `next_before` and `next_before_id`.
4. Send both values unchanged for the next page.
5. If search or any filter changes, discard rows/cursors and start again.
6. Never synthesize a cursor from the displayed timestamp alone.

For infinite scroll, deduplicate CPOs by `id`. Realtime invalidation or a
successful mutation may cause the frontend to refresh the first page; do not
mix a cursor from the old filter/snapshot into the refreshed collection.

## CPO Workflows

### Slug preflight

Call
`GET /api/v1/platform/cpos/slug-availability?slug=${encodeURIComponent(candidate)}`
after a short debounce and cancel the prior request when the field changes.
The server trims and lowercases the candidate and returns that normalized value:

```json
{"slug":"example-charging","available":true}
```

Only display the result if the response slug still matches the form's current
normalized slug. `available=true` does not reserve it. Another creation can win
the race, so keep `409 cpo_slug_conflict` handling on the final POST and attach
it to the slug field as a fresh validation failure.

### Create and onboard

```json
{
  "slug": "example-charging",
  "business_name": "Example Charging Private Limited",
  "company_type": "COMPANY",
  "gstin": "19ABCDE1234F1Z5",
  "address": "1 Example Road",
  "city": "Kolkata",
  "state": "West Bengal",
  "pincode": "700001",
  "admin": {
    "email": "admin@example.com",
    "full_name": "CPO Administrator"
  }
}
```

Normalization/validation:

- slug is trimmed/lowercased, max 80, and uses single-hyphen-separated words;
- business name is required, max 255;
- company type is `INDIVIDUAL` or `COMPANY`;
- GSTIN is required, uppercased, structurally valid with a valid Indian GSTIN
  checksum, globally unique after normalization, and its state code must match
  the selected registration state;
- address, city, state, and pincode are all required after trimming; pincode is
  exactly six digits, and the text maxima are 5000/100/100;
- admin email is normalized lowercase, valid, max 320;
- admin full name is required, max 255;
- status and app-ID fields are server-owned and must not be sent.

Success creates, atomically:

- one `PENDING` CPO with reason `Initial provisioning`;
- one generated dummy app ID in `DUMMY` mode;
- one active primary `ADMIN` membership;
- audit evidence;
- one durable platform event; and
- one encrypted onboarding/assignment mail job.

If the email is new, `identity_created=true` and the generated temporary
password exists only in the encrypted welcome job, SMTP renderer memory, and
recipient email. The welcome job is rejected before the CPO transaction commits
if that credential is absent. If the email already belongs to an active global
identity, `identity_created=false`; no temporary password is generated and its
password, name, verification state, and unrelated memberships are not
overwritten.

`201` proves the mail job committed, not that SMTP delivered it. Fetch the
primary-admin resource and distinguish `PENDING`/`PROCESSING`/`FAILED` from
`SENT` before telling the operator that credentials were sent.

Mail disabled returns `503 mail_unavailable` before creation. Do not optimistically
show success until `201` is received.

### Detail and business profile

Use `GET /api/v1/platform/cpos/{cpo_id}` after navigation, mutation, or relevant
event. The response does not embed primary-admin data; fetch that separate
resource in parallel.

Profile update is replacement-style:

```json
{
  "business_name": "Example Charging Limited",
  "company_type": "COMPANY",
  "gstin": "19ABCDE1234F1Z5",
  "address": "2 Example Road",
  "city": "Kolkata",
  "state": "West Bengal",
  "pincode": "700001"
}
```

Critical FE rule: every field shown is required. GSTIN, address, city, state,
and pincode cannot be null, blank, or omitted. Validate the GSTIN checksum and
state-code/state match before submit when practical, but always render the
server's field-specific error because it is authoritative. Build the request from the
complete form snapshot, not only dirty fields. The endpoint cannot change
slug, ID, lifecycle, app ID, membership, or tenant data.

### Activate and suspend

Both commands require:

```json
{"reason":"Approved after onboarding review"}
```

The trimmed reason must be 3–500 characters. Use an explicit confirmation
dialog that displays the target CPO and resulting state.

Activation:

- changes the CPO to `ACTIVE`;
- permits eligible CPO ADMIN login;
- requires neither a live app ID nor commercial record;
- is lifecycle-idempotent when already active.

Suspension:

- changes the CPO to `SUSPENDED`;
- blocks new tenant access;
- revokes current CPO-staff and customer sessions and unused refresh tokens;
- preserves tenant data;
- never revokes platform sessions;
- repeats the revocation scan even when already suspended, while avoiding a
  duplicate lifecycle audit/event.

If a network failure makes the outcome ambiguous, GET the CPO before offering
the command again.

### Set or rotate the live app ID

```json
{"app_id":"example_charging_production"}
```

- server trims and lowercases;
- length is 16–100;
- characters are letters, digits, underscore, or hyphen;
- reserved `cpo_dummy_` prefix is rejected;
- value is globally unique;
- success sets `app_id_mode` to `LIVE`;
- existing sessions remain valid;
- old tenant `X-CPO-App-ID` values fail immediately.

Warn the operator that existing CPO/customer frontends must adopt the returned
value from login, refresh, or `/auth/me`. Do not describe app-ID rotation as
credential rotation.

### Inspect the primary administrator

Load `GET /api/v1/platform/cpos/{cpo_id}/primary-admin` alongside detail.

Display:

- email and full name;
- membership and identity active state;
- verified and must-change-password state;
- last login when present;
- safe latest onboarding job template/status/attempt count/timestamps.

Absent optional timestamps and absent delivery metadata are normal. Never add
a “show password,” “show OTP,” “show mail body,” or “decrypt” control.

Mail status UX:

- `PENDING`: queued or waiting for retry;
- `PROCESSING`: claimed by a worker;
- `SENT`: SMTP send returned success;
- `FAILED`: bounded attempts exhausted; platform operators may inspect the safe
  job metadata and use the dedicated mail retry operation when the job is
  eligible.

### Restore or replace the primary administrator

```json
{
  "email": "replacement@example.com",
  "full_name": "Replacement Administrator",
  "reason": "Previous administrator left the organization"
}
```

- email is normalized lowercase and max 320;
- full name is 1–255;
- reason is 3–500;
- a new identity gets a credential-bearing welcome job; only a `SENT` delivery
  status proves SMTP accepted the email containing its temporary password;
- existing active identity is reused without changing its password or global
  profile, so the submitted full name is not an edit for that identity;
- inactive identity returns `409 admin_identity_inactive`;
- previous primary membership and that user's CPO sessions/refresh tokens for
  this CPO are revoked;
- unrelated CPO, customer, and platform sessions remain isolated;
- assigning the already-active current primary is a side-effect-free retry;
- assigning the same primary after membership revocation restores it and sends
  credential-free onboarding details.

After success, replace the primary-admin query result and refresh CPO detail and
audit. Do not assume the submitted name is the returned name.

### Resend onboarding

```json
{"reason":"Administrator requested access instructions again"}
```

The current identity and membership must be active. `202` queues current CPO
ID/app-ID information and password-recovery guidance. It never regenerates or
sends a password. Replace the primary-admin resource with the returned view so
the UI shows the new delivery metadata.

### Revoke all CPO administrative sessions

```json
{"reason":"Suspected credential exposure"}
```

The response contains revoked session and refresh-token counts. The operation:

- affects only `CPO` administrative sessions for that CPO;
- does not affect customer sessions;
- does not affect the current platform session;
- does not change the CPO lifecycle or membership;
- is safe to repeat and still writes auditable zero counts.

This is different from CPO suspension. Use a distinct confirmation and explain
the narrower effect.

## Platform Governance and Operations

### Platform administrators

Use `GET /api/v1/platform/administrators` for a keyset-paginated authority
list. Inactive authority is omitted unless `include_inactive=true`. The list
contains identity and authority state, but never passwords, sessions, OTPs, or
mail payloads.

`POST /api/v1/platform/administrators` accepts an email and full name. It
either creates a new global identity and queues a credential-bearing invitation
or grants authority to an existing active identity. A successful `201` means
the database transaction and encrypted mail job committed; it does not prove
SMTP delivery.

Activate and deactivate use `POST .../{user_id}/activate|deactivate` with a
3–500 character reason. Deactivation preserves the global identity, revokes
its platform sessions, and refuses to remove the last active platform
administrator. Confirm the target and consequence before either command.

### Security operations

`GET /api/v1/platform/security/locked-identities` lists currently locked
identities. `GET /api/v1/platform/security/events` is read-only audit evidence.
Unlocking uses `POST /api/v1/platform/security/users/{user_id}/unlock` with a
reason and returns `204`.

`POST /api/v1/platform/security/users/{user_id}/sessions/revoke` accepts:

```json
{
  "reason": "Suspected credential exposure",
  "scope": "CPO",
  "cpo_id": "c821a013-5041-42f7-80c8-aa153cf9d455"
}
```

`scope` is `PLATFORM`, `CPO`, or `ALL`; `cpo_id` is required only for `CPO`.
The response gives revoked session and refresh-token counts. This is an
incident-response operation, not identity deletion or CPO suspension.

### Mail operations

`GET /api/v1/platform/mail/jobs` supports `status`, `template`, `cpo_id`, and
`user_id` filters plus the standard `limit`, `before`, and `before_id` cursor.
List/detail responses contain recipient address, template, state, attempts,
timestamps, and `error_present`; they never return decrypted payloads or error
text.

- `POST /api/v1/platform/mail/jobs/{job_id}/retry` requeues an eligible failed
  job and returns its safe metadata.
- `POST /api/v1/platform/mail/jobs/{job_id}/cancel` cancels an unsent job and
  requires a reason.
- `GET /api/v1/platform/mail/metrics` returns counts grouped by template and
  status.
- `POST /api/v1/platform/mail/reconcile` requeues stale processing jobs and
  requires a reason.
- `POST /api/v1/platform/mail/retention` requires `{before, reason}`. The
  cutoff must be at least 30 days old; only `SENT` and `CANCELED` jobs can be
  deleted. Show the resulting count and require explicit confirmation.

### Announcements and notifications

`POST /api/v1/platform/announcements` accepts `audience`, `title`, `body`, and
an optional future `expires_at`. `audience` is `PLATFORM` or `CPO`; a CPO
announcement requires `cpo_id`, while a platform announcement forbids it.
Creation snapshots eligible recipients and creates durable notification rows
in the same transaction. The snapshot does not change when later membership
changes.

`GET /api/v1/platform/announcements` lists immutable announcements with
recipient counts. `GET /api/v1/platform/notifications` lists the current
SuperAdmin's recipient-owned inbox and accepts `unread_only=true`. Marking
`POST /api/v1/platform/notifications/{notification_id}/read` returns `204` and
cannot mark another recipient's notification.

### Overview and status

`GET /api/v1/platform/overview` provides bounded CPO, active-admin,
active-session, mail, and worker aggregates for dashboard cards. It is not a
tenant business-data export. `GET /api/v1/platform/status` provides service
version, database availability, and logical-worker status. Both are
read-only; they do not expose worker start/stop/retry controls.

## Manual Subscription Workflows

Manual subscription operations require `PLATFORM` authority and are separate
from CPO administrative access. A subscription status never activates,
suspends, or otherwise authorizes a CPO administrator; use the CPO lifecycle
endpoints for that control. Expiry does block only new customer charging starts
and wallet recharge-order creation for the CPO; stop, reconciliation,
settlement, customer reads, and pre-expiry recharge verification continue.

### Plan catalog

Create plans with `POST /api/v1/platform/plans` using `code`, `name`, optional
`description`, and `terms`:

```json
{
  "code": "standard",
  "name": "Standard",
  "description": "Manual standard plan",
  "terms": {
    "currency": "INR",
    "price_minor": 49900,
    "billing_interval": "MONTHLY",
    "interval_count": 1,
    "trial_days": 14
  }
}
```

Codes are lowercase underscore-separated identifiers. Currency is three
uppercase letters, price is non-negative minor units, interval is `MONTHLY` or
`YEARLY`, interval count is 1–120, and trial days are 0–365.

Use `GET /api/v1/platform/plans` for the catalog or
`GET /api/v1/platform/plans/{plan_id}` for one plan. `PUT .../{plan_id}/draft`
replaces the single editable draft. `POST .../{plan_id}/publish` makes that
version immutable and issueable. `POST .../{plan_id}/archive` prevents future
issue/change-plan selection but preserves historical reads.

### CPO subscription lifecycle

`POST /api/v1/platform/cpos/{cpo_id}/subscription` issues a current
subscription from a published `plan_version_id`. `renew` records a new period
and reactivates the same `EXPIRED` record after manual payment confirmation;
an expired renewal starts at the command time rather than backdating into an
already elapsed period. `change-plan` immediately selects another published
version, and the explicit transition commands move the current record through
the supported states:

| Command | Supported behavior |
| --- | --- |
| `activate` | `TRIAL` → `ACTIVE` |
| `pause` | `TRIAL`, `ACTIVE`, or `PAST_DUE` → `PAUSED` |
| `resume` | `PAUSED` or `PAST_DUE` → `ACTIVE` |
| `mark-past-due` | `TRIAL`, `ACTIVE`, or `PAUSED` → `PAST_DUE` |
| `expire` | Explicitly ends the current subscription as `EXPIRED` |
| `cancel` | Cancels immediately; period-end cancellation is unsupported |

Every write request includes a trimmed `reason` and client-generated
`idempotency_key`. Retry the same command with the same key and actor after an
ambiguous response; do not create a new key until authoritative GET/history
confirms that the first command did not commit. Reusing a key for a different
operation or CPO returns `409 idempotency_conflict`.

`GET /api/v1/platform/cpos/{cpo_id}/subscription` returns the current joined
subscription, plan, and plan version. `GET .../history` returns up to 500
newest-first audited transitions. There are no provider, invoice, payment,
checkout, webhook, mail, automatic renewal, scheduled change, feature-key, or
entitlement APIs.

## Platform Audit

`GET /api/v1/platform/audit-logs` supports:

- `before` RFC3339 cursor;
- `before_id` UUID tie-breaker paired with `before`;
- `limit` 1–500, default 50;
- exact `action` and `entity` strings, max 100;
- `actor_user_id` UUID;
- `cpo_id` UUID.

Results are newest first. When `has_more`, send both returned cursor fields.
Changing a filter resets the cursor.

Current CPO-control actions include:

- `CPO_CREATED`;
- `CPO_PROFILE_UPDATED`;
- `CPO_STATUS_ACTIVE`;
- `CPO_STATUS_SUSPENDED`;
- `CPO_APP_ID_SET_LIVE`;
- `CPO_PRIMARY_ADMIN_REPLACED`;
- `CPO_PRIMARY_ADMIN_RESTORED`;
- `CPO_PRIMARY_ADMIN_ONBOARDING_RESENT`;
- `CPO_ADMIN_SESSIONS_REVOKED`.

Treat action/entity strings as display-mapped stable identifiers. Retain a
fallback label for future unknown actions. `details` is sanitized structured
metadata, but the FE should still avoid forwarding complete audit records to
third-party analytics.

Audit is durable security evidence. Platform events are retention-bounded UI
invalidation. Do not merge them into one source of truth.

## Worker Health and Readiness

`GET /api/v1/platform/workers` is read-only. It exposes one current process
incarnation for each logical worker. Historical rows are retained operationally
but never appear as additional current worker cards after restart.

- `HEALTHY`: reported healthy and fresh;
- `DEGRADED`: worker reported a problem;
- `STALE`: derived because the last heartbeat is too old;
- `DISABLED`: intentionally not running;
- `required=true`: the current incarnation for this worker role must be fresh
  and healthy for readiness.

Registration atomically supersedes the prior incarnation for a logical worker.
An old delayed heartbeat cannot overwrite the newer current state; if the
current incarnation disappears, its heartbeat eventually becomes `STALE`.

Use `GET /health/ready` for the aggregate readiness badge and workers for the
diagnostic table. The API cannot start, stop, restart, retry, or kill a worker.
Do not render action buttons that imply those controls exist.

## Realtime, Replay, and Recovery

### Authority model

PostgreSQL and REST are authoritative. Events announce committed facts and
tell the frontend what to refetch. Do not merge event `data` into a CPO object
as if it were a complete resource.

Delivery properties:

- durable retained source: `platform_events`;
- ascending numeric cursor;
- at-least-once delivery;
- duplicates are possible;
- SSE and REST replay expose the same facts;
- old events expire after configured retention;
- stream heartbeats revalidate the access token, durable session, identity,
  and platform authority;
- the stream closes after logout/revocation/expiry, network loss, or shutdown.

### Current event payloads and invalidation

| Event type | Current `data` fields | FE action |
| --- | --- | --- |
| `platform.cpo.created` | `status`, `app_id_mode` | Refresh collection first page |
| `platform.cpo.profile_updated` | Empty object | Refresh collection and visible detail |
| `platform.cpo.activated` | `previous_status`, `status`, `reason` | Refresh collection and detail |
| `platform.cpo.suspended` | `previous_status`, `status`, `reason` | Refresh collection and detail |
| `platform.cpo.app_id_rotated` | `app_id_mode` | Refresh CPO detail; event does not carry app ID |
| `platform.cpo.primary_admin_changed` | `new_user_id`, `identity_created`, `change_type`, `reason`, optional `previous_user_id` | Refresh primary admin and detail |
| `platform.cpo.primary_admin_onboarding_resent` | `primary_admin_user_id`, `reason` | Refresh primary admin |
| `platform.cpo.admin_sessions_revoked` | `reason`, `revoked_sessions`, `revoked_refresh_tokens` | Refresh audit/recovery display |

`resource_type` is `CPO` and `resource_id` is the CPO UUID for these events.
Do not assume unlisted worker, mail, governance, notification, or overview
events are currently emitted.

### REST catch-up algorithm

1. Load the cursor saved for this API origin and platform user, or start at 0.
2. GET `/api/v1/platform/events?after_id=<cursor>&limit=500`.
3. Process events in ascending ID order.
4. Ignore an event whose ID was already applied.
5. Schedule/deduplicate the necessary REST refetches.
6. Save `next_cursor` only after the page has been processed.
7. Continue while `has_more=true`.
8. Open SSE using the final cursor.

An unfiltered application stream is recommended. If multiple views subscribe,
use one connection and an internal event dispatcher rather than one SSE
connection per component.

### Authenticated SSE

Native browser `EventSource` cannot attach the required bearer header. Use
`fetch()` streaming:

```ts
export async function consumePlatformEvents(args: {
  origin: string;
  accessToken: string;
  lastEventId?: number;
  signal: AbortSignal;
  onEvent: (event: PlatformEvent) => Promise<void> | void;
}): Promise<number | undefined> {
  const headers = new Headers({
    Accept: "text/event-stream",
    Authorization: `Bearer ${args.accessToken}`,
  });
  if (args.lastEventId !== undefined) {
    headers.set("Last-Event-ID", String(args.lastEventId));
  }

  const response = await fetch(
    `${args.origin}/api/v1/platform/realtime/stream?limit=500`,
    { headers, credentials: "omit", signal: args.signal },
  );
  if (!response.ok) throw await failureFrom(response);
  if (!response.body) throw new Error("SSE response has no readable body.");

  const reader = response.body.getReader();
  const decoder = new TextDecoder();
  let buffer = "";
  let committedCursor = args.lastEventId;

  for (;;) {
    const { value, done } = await reader.read();
    buffer += decoder.decode(value, { stream: !done });
    buffer = buffer.replace(/\r\n/g, "\n");

    for (;;) {
      const boundary = buffer.indexOf("\n\n");
      if (boundary < 0) break;
      const frame = buffer.slice(0, boundary);
      buffer = buffer.slice(boundary + 2);

      let frameId: number | undefined;
      const data: string[] = [];
      for (const line of frame.split("\n")) {
        if (line.startsWith(":")) continue; // heartbeat comment
        if (line.startsWith("id:")) frameId = Number(line.slice(3).trim());
        if (line.startsWith("data:")) data.push(line.slice(5).trimStart());
      }
      if (data.length === 0) continue;

      const event = JSON.parse(data.join("\n")) as PlatformEvent;
      if (!Number.isSafeInteger(event.id) || event.id !== frameId) {
        throw new Error("Unsafe or inconsistent platform event cursor.");
      }
      if (committedCursor !== undefined && event.id <= committedCursor) continue;
      await args.onEvent(event);
      committedCursor = event.id; // persist only after successful processing
    }

    if (done) return committedCursor;
  }
}
```

Use an `AbortController` to close the stream during logout, token replacement,
or application teardown. Reconnect with bounded exponential backoff plus jitter.
Because the access token is short-lived, proactively refresh and reconnect
before its reported expiry rather than waiting for a heartbeat to close it.

### Cursor expiry

If replay or the initial SSE response returns `409 realtime_cursor_expired`:

1. discard the saved cursor;
2. reload the CPO collection first page and every visible CPO/primary-admin
   detail;
3. reload audit/workers if those views are active;
4. reconnect without the expired cursor;
5. accept redundant retained events and deduplicate them.

After streaming starts, the server cannot replace the response status; a later
auth/database failure closes the connection. A closed stream is not proof that
the last command failed or that no event committed.

## Error-to-UX Matrix

| Status/code | FE behavior |
| --- | --- |
| `400 invalid_request` | Treat as client/schema bug or invalid form; never resend unchanged automatically |
| `400 invalid_*` | Map known field code to the form; keep a safe form-level fallback |
| `401 invalid_credentials` | Generic login failure; do not reveal account/authority/lock state |
| `401 invalid_challenge` | Clear/replace OTP state and return to a recoverable challenge flow |
| `401 unauthorized` | Attempt one coordinated refresh when appropriate; otherwise clear session |
| `401 invalid_refresh_token` | Clear access and refresh state; require login |
| `403 forbidden` | Authenticated but not a platform session; block the platform application |
| `404 cpo_not_found` | Close stale detail and refresh collection |
| `404 primary_admin_not_found` | Show recovery state; do not fabricate an administrator |
| `404 session_not_found` | Refresh own sessions; target was absent or not owned |
| `409 cpo_slug_conflict` | Attach to slug; an earlier availability result is not a reservation |
| `409 cpo_gstin_conflict` | Attach to GSTIN; it is already assigned to another CPO |
| `409 cpo_app_id_conflict` | Attach to app ID; the requested ID is already assigned |
| `409 admin_identity_conflict` | A concurrent request created the identity; retry once through the normal mutation flow |
| `409 cpo_admin_membership_conflict` | Refresh primary-admin state; the identity is already a member of this CPO |
| `409 cpo_primary_admin_conflict` | Refresh primary-admin state; another primary assignment won the race |
| `409 cpo_conflict` | Safe form-level fallback for an unrecognized uniqueness constraint |
| `409 admin_identity_inactive` | Cannot assign this identity with current APIs |
| `409 primary_admin_unavailable` | Refresh primary admin; active identity/membership is required |
| `409 realtime_cursor_expired` | Full REST snapshot recovery, then cursor reset |
| `429 rate_limited` | Disable rapid retry; show generic retry-later state |
| `503 mail_unavailable` | Do not claim login/onboarding/resend succeeded |
| `500 internal_error` | Show safe retry/support message; never display guessed infrastructure details |
| Network timeout/abort | Outcome may be unknown; GET authoritative state before retrying a mutation |

The backend currently does not return `Retry-After`. Use conservative UI
cooldowns and the explicit OTP resend timestamp where available.

## Mutation Retry and Cache Rules

- GET requests are safe to retry.
- Activation is idempotent for lifecycle evidence; repeated suspension also
  re-runs session revocation.
- Primary-admin assignment to the already-active current admin is a no-op.
- CPO administrative-session revocation is repeatable and auditable even at
  zero counts.
- Platform governance, security, mail, announcement, notification, overview,
  and status writes are not safe to repeat blindly; use the authoritative GET
  or returned resource after an ambiguous result.
- Manual subscription writes require the same `idempotency_key` for a retry.
  Do not generate a new key until subscription GET/history confirms that the
  first command did not commit.
- Create, profile replacement, app-ID rotation, primary-admin replacement, and
  onboarding resend have no client idempotency key.
- After any ambiguous mutation result, use GET/replay/audit to determine state.
- Do not let generic query-cache retries repeat POST/PUT automatically.
- Invalidate by stable query keys such as `cpos:list:<filters>`,
  `cpo:<id>`, `cpo:<id>:primary-admin`, `platform:audit:<filters>`, and
  `platform:workers`.

REST response bodies should replace cached resources. Realtime events should
invalidate them.

## Security Checklist for the FE

- Require HTTPS outside loopback development.
- Keep the API origin in trusted build/runtime configuration.
- Never accept an arbitrary API origin from a query parameter.
- Never put bearer or refresh tokens in URLs.
- Never use native `EventSource` with a query-string token.
- Never log passwords, OTPs, access tokens, refresh tokens, or full auth bodies.
- Redact `Authorization` in error reporting and network instrumentation.
- Avoid third-party analytics on login, OTP, password, primary-admin, and audit
  screens.
- Clear token/challenge state on logout and authority mismatch.
- Coordinate one-time refresh rotation across requests and shared tabs.
- Render server strings as text, never HTML.
- Treat event/audit `data` and `details` as untrusted JSON for rendering.
- Confirm target CPO and reason before lifecycle, admin replacement, onboarding
  resend, and session revocation.
- Do not claim mail delivery from a queued `202`; display safe job status when
  the primary-admin resource provides it.
- Do not expose or request tenant integration secrets.
- Do not implement SuperAdmin-to-CPO impersonation.

## FE Verification Checklist

### Contract and connectivity

- [ ] Environment origin loads without a hardcoded `/api/v1` duplication.
- [ ] `/health/live` and `/health/ready` are handled separately.
- [ ] `/openapi.yaml` parses and includes all 66 required SuperAdmin API operations.
- [ ] Swagger is treated as a development tool, not embedded in the product UI.
- [ ] Local/mock types preserve optional-field omission.

### Authentication

- [ ] Login sends exact `PLATFORM` scope and no CPO ID.
- [ ] Resend replaces the challenge ID.
- [ ] OTP/resend timers use returned timestamps.
- [ ] `/auth/me` rejects non-platform scope.
- [ ] Refresh is single-flight and atomically replaces both tokens.
- [ ] Shared-tab behavior cannot reuse one consumed refresh token.
- [ ] Logout/session revocation aborts SSE and clears local state.
- [ ] Password change routes back to login after global revocation.
- [ ] Forgot-password UI discloses no account existence and collects the recovery ID and code delivered by email.

### Governance, mail, communication, and subscriptions

- [ ] Administrator list pagination and `include_inactive` are implemented.
- [ ] Invite/grant, activate, and deactivate confirmations explain mail and
  last-active-admin consequences.
- [ ] Locked identities, security events, reasoned unlock, and scoped session
  revocation are available without exposing secrets.
- [ ] Mail list filters, safe metadata, retry/cancel, reconciliation, metrics,
  and 30-day-minimum retention are implemented with explicit confirmations.
- [ ] Announcement audience rules and immutable recipient snapshots are shown.
- [ ] Platform notifications support unread filtering and recipient-owned read.
- [ ] Overview/status remain bounded read-only diagnostics.
- [ ] Plan drafts, publishing, archiving, manual issue/renew/change, explicit
  transitions, history, and idempotent retries are implemented.
- [ ] The FE does not present subscription status as CPO access or introduce
  feature keys, entitlements, provider billing, or automatic lifecycle.

### CPO control

- [ ] List filters reset both cursor fields.
- [ ] Slug preflight is debounced/cancelled and final creation still handles `cpo_slug_conflict`.
- [ ] Detail and primary admin load as separate resources.
- [ ] Profile form sends a complete replacement snapshot.
- [ ] GSTIN and every address field are required in create and profile forms.
- [ ] Reasons are trimmed and validated at 3–500 characters.
- [ ] Suspension confirmation explains tenant-session revocation.
- [ ] App-ID rotation confirmation explains immediate client impact.
- [ ] Existing-primary identity reuse does not promise a name/password change.
- [ ] Mail status uses safe metadata only.
- [ ] Administrative-session revocation is described as CPO-staff-only.

### Realtime and observation

- [ ] One authenticated fetch-stream connection is shared across the app.
- [ ] Last processed event ID is persisted only after handler success.
- [ ] Duplicate IDs are ignored.
- [ ] Events trigger REST invalidation rather than object merging.
- [ ] Cursor expiry performs full visible-state recovery.
- [ ] Stream reconnect uses backoff and never places tokens in URLs.
- [ ] Audit keyset cursors stay paired with their filters.
- [ ] Worker UI has no unsupported control buttons.

### Safe integration testing

Creating/suspending CPOs, replacing administrators, resending onboarding, and
revoking sessions mutate durable data and may send real email. Run those tests
only with an approved test CPO, approved recipient addresses, and explicit
authorization. Do not use the development database as if it were disposable.

The backend has source and disposable-PostgreSQL verification for current CPO
control behavior. This handoff's reconciliation rechecked public health,
Swagger, and OpenAPI only; it did not use platform credentials, send mail, or
mutate the deployed database.

## Known Gaps and Required Backend Decisions

The FE should raise, not paper over, these gaps:

1. Stateful PostgreSQL lifecycle verification for the Superadmin/mail/
   notification surface remains pending because no disposable test database is
   configured.
2. There is no generated frontend SDK or committed generated types.
3. There is no SuperAdmin tenant impersonation workflow. The deliberately
   limited cross-CPO support conversation workflow is documented separately in
   `guides/workflows/superadmin-support-tickets.md`.
4. Manual subscription plans, CPO subscriptions, and history are platform-only.
   Feature keys/entitlements, platform invoices, payments, checkout,
   webhooks, and automatic lifecycle behavior remain intentionally unsupported.

If the FE generates types from OpenAPI, pin the generator version in the FE
repository and make regeneration plus type-checking part of its verification.
The backend does not currently publish or verify a generated SDK.

## Handoff Completion Definition

The current SuperAdmin FE integration is complete when:

- PLATFORM login, OTP, refresh, scope bootstrap, and account-session behavior
  follow the state machine above;
- forgot/reset consumes only the recovery ID and code delivered to the eligible
  recipient and preserves the generic start response;
- every implemented CPO command/query is wired with its exact boundary,
  validation, retry, and recovery behavior;
- audit and workers are presented as read-only authoritative queries;
- one authenticated SSE/replay client deduplicates events and refreshes REST;
- cursor expiry and token rotation recover without losing authoritative state;
- unavailable features are absent or explicitly marked unavailable;
- no tenant business/secret access or impersonation path is introduced;
- mutation testing uses approved data and recipients;
- the FE's own tests cover token-rotation races, cursor pairing, event
  deduplication, optional fields, `204`, error envelopes, and the documented
  negative paths.
