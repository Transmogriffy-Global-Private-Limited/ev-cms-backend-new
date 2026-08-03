# CPO Administration Frontend Guide

This is the implementation guide for the Superadmin CPO screen. The canonical
machine contract is `contracts/openapi/openapi.yaml`; exhaustive shared API
semantics are in `contracts/api/administrative-http-api.md`. The same operations
are executable from `/docs/` when `API_DOCS_ENABLED=true`.

For the complete SuperAdmin application handoff—including authentication,
TypeScript types, audit/workers, realtime code, error UX, and known gaps—use
`SUPERADMIN_FRONTEND_HANDOFF.md`.

## Boundary

Base path: `/api/v1/platform/cpos`.

Every operation requires:

```http
Authorization: Bearer <encrypted-platform-access-token>
Content-Type: application/json
```

Only a current `PLATFORM` session is accepted. Never send `X-CPO-App-ID` on
this control-plane surface: an app ID is routing metadata, not authority.
Superadmin CPO administration does not read subscription, invoice, payment, or
entitlement state.

## Recommended Screen Model

Use four REST-backed regions:

1. CPO collection: list, search, filters, and cursor.
2. CPO identity: business profile, lifecycle evidence, and app ID.
3. Primary administrator: identity, membership state, password-change flag, and
   the latest safe onboarding-delivery metadata.
4. Recovery actions: resend onboarding, replace/restore the primary
   administrator, or revoke all CPO administrative sessions.

REST is authoritative. Realtime events tell the frontend what to refresh; they
are not replacement CPO objects.

## Collection

`GET /api/v1/platform/cpos`

Optional query parameters:

| Name | Meaning |
| --- | --- |
| `q` | Case-insensitive substring across business name, slug, GSTIN, app ID, and primary-admin name/email. Maximum 200 characters. |
| `status` | Exact `PENDING`, `ACTIVE`, or `SUSPENDED`. |
| `app_id_mode` | Exact `DUMMY` or `LIVE`. |
| `limit` | 1–200, default 50. |
| `before` | Exclusive RFC3339 creation timestamp cursor. |
| `before_id` | UUID tie-breaker; required with `before`, forbidden without it. |

`200 OK`:

```json
{
  "cpos": [
    {
      "id": "c821a013-5041-42f7-80c8-aa153cf9d455",
      "slug": "example-charging",
      "business_name": "Example Charging Private Limited",
      "company_type": "COMPANY",
      "status": "ACTIVE",
      "status_reason": "Approved after onboarding review",
      "status_changed_at": "2026-07-31T09:30:00Z",
      "status_changed_by_user_id": "5cef4c95-a1da-448e-bd7c-19d570cd4497",
      "app_id": "cpo_dummy_735f36a898b84ce68a350db38c90bf9b",
      "app_id_mode": "DUMMY",
      "app_id_updated_at": "2026-07-31T09:00:00Z",
      "created_at": "2026-07-31T09:00:00Z",
      "updated_at": "2026-07-31T09:30:00Z"
    }
  ],
  "next_before": "2026-07-31T09:00:00Z",
  "next_before_id": "c821a013-5041-42f7-80c8-aa153cf9d455",
  "has_more": true
}
```

When `has_more=true`, send both returned cursor fields unchanged. Changing a
filter or search term starts a new query and discards the previous cursor.

## Create

`POST /api/v1/platform/cpos`

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

`201 Created` returns `{"cpo": <CPO>, "admin": <initial-admin>}`. The CPO starts
`PENDING`, with lifecycle reason `Initial provisioning`, one generated dummy
app ID, and exactly one active primary `ADMIN` membership.

Creation atomically persists the CPO, membership, audit record, platform event,
and correlated encrypted mail job. A new email receives a generated temporary
password only through the welcome email; the API never returns it, and the
transaction fails if the welcome payload lacks that credential. An existing
active global identity keeps its existing password and receives an assignment
email. `201` proves the mail job committed, not SMTP delivery; use the
primary-admin delivery status to distinguish `SENT` from pending or failed
delivery. Mail being disabled fails the command before anything is created.

## Detail and Business Profile

`GET /api/v1/platform/cpos/{cpo_id}` returns the current CPO.

`PUT /api/v1/platform/cpos/{cpo_id}/profile` replaces editable business fields:

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

`business_name` and `company_type` are required. `gstin`, `address`, `city`,
`state`, and `pincode` are replacement fields: null/blank GSTIN or omission
clears GSTIN; omission clears an address field. The immutable CPO ID, slug, app
ID, and lifecycle state cannot be changed here.

## Lifecycle

Activation and suspension require a trimmed 3–500 character human reason:

```http
POST /api/v1/platform/cpos/{cpo_id}/activate
POST /api/v1/platform/cpos/{cpo_id}/suspend
```

```json
{"reason":"Approved after onboarding review"}
```

The returned CPO includes the authoritative `status_reason`,
`status_changed_at`, and `status_changed_by_user_id`.

- Repeating the current state is lifecycle-idempotent: it does not rewrite the
  reason or emit another lifecycle audit/event.
- Repeating suspension still removes any CPO sessions created since the first
  suspension.
- Suspension revokes all CPO-staff and customer sessions plus unused refresh
  tokens tied to the CPO. Platform sessions are unaffected.
- Activation requires neither a live app ID nor commercial state.

## App ID

`PUT /api/v1/platform/cpos/{cpo_id}/app-id`

```json
{"app_id":"example_charging_production"}
```

The server trims and lowercases the value. It must be 16–100 permitted
characters, globally unique, and not use the reserved `cpo_dummy_` prefix.
Rotation does not revoke sessions. It immediately invalidates the previous
`X-CPO-App-ID` value; a CPO client recovers the current value through login,
refresh, or `/api/v1/auth/me`.

## Primary Administrator

`GET /api/v1/platform/cpos/{cpo_id}/primary-admin`

```json
{
  "user_id": "e5288707-7266-44d4-b5a2-a87d06f1f2b7",
  "email": "admin@example.com",
  "full_name": "CPO Administrator",
  "role": "ADMIN",
  "membership_status": "ACTIVE",
  "identity_active": true,
  "identity_verified": true,
  "must_change_password": true,
  "latest_onboarding_delivery": {
    "job_id": "4ccb8733-b2e5-4f35-9953-f0e5f32176f2",
    "template": "CPO_ADMIN_WELCOME",
    "status": "PENDING",
    "attempts": 0,
    "created_at": "2026-07-31T09:00:00Z",
    "updated_at": "2026-07-31T09:00:00Z"
  }
}
```

Mail metadata is deliberately safe: it contains no encrypted payload,
password, OTP, or token.

Replace or restore the primary administrator:

`PUT /api/v1/platform/cpos/{cpo_id}/primary-admin`

```json
{
  "email": "replacement@example.com",
  "full_name": "Replacement Administrator",
  "reason": "Previous administrator left the organization"
}
```

The command is serialized per CPO and normalized email. It guarantees one
primary administrator:

- an existing active identity is reused without changing its password;
- a new identity gets a generated temporary password through encrypted welcome
  mail only, and an incomplete welcome payload fails the transaction;
- an inactive identity is rejected;
- the previous primary membership is revoked and its CPO-scoped sessions and
  refresh tokens are revoked;
- assigning the already-active current primary is a no-op;
- assigning the current primary after its membership was revoked restores it
  and sends credential-free onboarding details.

## Onboarding Recovery

`POST /api/v1/platform/cpos/{cpo_id}/primary-admin/resend-onboarding`

```json
{"reason":"Administrator requested access instructions again"}
```

This queues the current CPO ID and app ID, and directs the recipient to password
recovery if needed. It never retrieves, regenerates, or sends a password. The
`202 Accepted` response is the primary-admin view with the new safe delivery
metadata.

## Administrative Session Recovery

`POST /api/v1/platform/cpos/{cpo_id}/administrative-sessions/revoke`

```json
{"reason":"Suspected credential exposure"}
```

`200 OK`:

```json
{
  "revoked_sessions": 3,
  "revoked_refresh_tokens": 2
}
```

This is safe to repeat and always records the reason and resulting counts. It
revokes every active `CPO` session for that CPO, not platform sessions,
customer sessions, identities, memberships, or business data.

## Realtime Refresh Map

Deduplicate events by numeric ID. On these events, refresh:

| Event | Refresh |
| --- | --- |
| `platform.cpo.created` | collection |
| `platform.cpo.profile_updated` | collection and visible CPO detail |
| `platform.cpo.activated`, `platform.cpo.suspended` | collection and detail |
| `platform.cpo.app_id_rotated` | detail |
| `platform.cpo.primary_admin_changed` | primary-admin card and detail |
| `platform.cpo.primary_admin_onboarding_resent` | primary-admin card |
| `platform.cpo.admin_sessions_revoked` | visible recovery result or audit view |

After reconnect, replay `/api/v1/platform/events`. On
`realtime_cursor_expired`, discard the expired cursor, reload the current
collection/detail/primary-admin REST snapshots, and reconnect without it.

## Error Handling

Every failure uses:

```json
{"error":{"code":"invalid_reason","message":"Reason must be between 3 and 500 characters."}}
```

The principal UI decisions are:

- `401 unauthorized`: clear the platform access state and use the normal refresh
  or login flow.
- `403 forbidden`: the session is authenticated but not platform-authorized.
- `404 cpo_not_found` or `primary_admin_not_found`: close stale detail state and
  refresh the collection.
- `409 cpo_conflict`: show the unique-field conflict without assuming which
  current record owns it.
- `409 admin_identity_inactive` or `primary_admin_unavailable`: the platform
  operator must choose or reactivate an eligible identity through a future
  governance surface.
- `503 mail_unavailable`: do not claim onboarding or resend succeeded.

Use OpenAPI for the complete validation-code matrix.
