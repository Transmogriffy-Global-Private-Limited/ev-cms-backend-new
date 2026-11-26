# CoW module entitlement, access and permissions - Run 1

**Implementation:** `f2be703b1d200eb0173c64ba655124496fb2755c` on `anubhab-work`; unchanged by Run 2A `77d2d85885d2fa4db8559ccbeb7c8049a0d9f974`. **Source accepted and published; VPS2 deployment not verified.**

## Existing and added HTTP contracts

| Method | Path | Result |
| --- | --- | --- |
| GET | `/api/v1/platform/cpos/{cpo_id}/modules/cow` | CPO-specific committed grant; absent row is disabled, version zero |
| PUT | `/api/v1/platform/cpos/{cpo_id}/modules/cow` | Enable/disable CoW with reason and optimistic version |
| GET | `/api/v1/cpo/cow/access` | Current signed-in CPO member's actual CoW eligibility and effective permissions |

Platform requests require a valid `PLATFORM` bearer and an active Platform administrator, not a CPO staff token. Never attach `X-CPO-App-ID` to the Platform routes. CPO access discovery requires an authenticated CPO bearer, current active membership and matching `X-CPO-App-ID`; tenant identity cannot be supplied by the caller. Discovery remains available to active members while CoW is disabled and returns `{enabled:false,eligible:false,effective_permissions:[]}`.

PUT request body: `{"enabled":true,"expected_version":0,"reason":"Approved for CoW operations"}`. All fields are required; version is nonnegative; reason trims to 3-500 UTF-8 **bytes**. A first enable changes version 0 to 1. If expected_version differs, return 409 `entitlement_version_conflict`; refetch and confirm with user before retrying. A same-state request at the current version is a no-op and adds no event/audit entry.

GET grant response (default off): `{"cpo_id":"11111111-1111-4111-8111-111111111111","module":"cow","enabled":false,"version":0}`. Versioned changes additionally report `reason`, `changed_by_user_id`, and `changed_at`. GET CPO access response has `cpo_id`, `module:"cow"`, `enabled`, `eligible`, and `effective_permissions` (string array). Backend error envelope is `{"error":{"code":"...","message":"..."}}`; handle malformed ID/body (400), expired authentication (401), absent authority (403), missing CPO (404), stale version (409), internal failure (500).

## Existing interfaces whose values expand

JSON shapes are unchanged, but these CPO responses may now contain `cow.*` keys: `GET /api/v1/cpo/access/me` (`role_defaults`, `allow`, `deny`, `effective`), `GET /api/v1/cpo/access/permissions` and `GET /api/v1/cpo/permissions/catalog` (permission definitions), and CPO staff collection/detail/create/update projections (`overrides`, `role_defaults`, `effective_permissions`). There are 24 existing + 45 new = **69** canonical catalog keys. OWNER/ADMIN default to all keys; OPERATOR to 18 CoW keys, VIEWER to 10. Source-controlled role details are in `src/cpopermissions/registry.go`. Explicit DENY always overrides default and ALLOW. A permission key is not an implemented future feature route.

## All 45 source-controlled CoW keys

```text
cow.users.read
cow.users.manage
cow.drivers.read
cow.drivers.review
cow.drivers.manage
cow.drivers.suspend
cow.fleet.read
cow.fleet.manage
cow.fleet.maintenance
cow.fleet.assign
cow.schedules.read
cow.schedules.manage
cow.dispatch.read
cow.dispatch.manage
cow.dispatch.override
cow.jobs.read
cow.jobs.manage
cow.jobs.cancel
cow.jobs.reassign
cow.charging.read
cow.charging.command
cow.charging.reconcile
cow.charging.trace.read
cow.pricing.read
cow.pricing.manage
cow.payments.read
cow.payments.manage
cow.refunds.manage
cow.invoices.read
cow.invoices.manage
cow.compensation.read
cow.compensation.manage
cow.compensation.adjust
cow.ratings.read
cow.support.read
cow.support.reply
cow.support.manage
cow.incidents.read
cow.incidents.manage
cow.notifications.read
cow.notifications.manage
cow.exports.manage
cow.system.settings.read
cow.system.settings.manage
cow.audit.read
```

For FE controls, query the CPO access projection and consume its `effective_permissions`; never derive CoW eligibility from a role alone. Future mutation services must enforce entitlement and membership/permission at the transaction boundary, not trust an earlier middleware result. Customer app sessions are separate from CPO administrative sessions.

## Durability, testing and limits

Migration `000080` stores the per-CPO versioned module grant and append-only grant events; real toggles commit the row/event/Platform audit together under a CPO lock. DOWN refuses destructive loss of history. The source passed isolated PG17 entitlement lifecycle, version conflict, tenant isolation, audit and guarded migration acceptance before the Run 1 commit. No hosted deployment claim follows from source tests.

Run 2A adds customer saved service addresses (not a new CPO staff endpoint); see [address FE contract](cow-customer-service-addresses.md). No CoW driver, fleet, ServiceJob, dispatch, pricing, payment, charging command, or HAL operation becomes callable merely because its key exists. The canonical machine contract is [OpenAPI](../openapi/openapi.yaml).
