# SuperAdmin and CPO Frontend Boundary

## Purpose

The platform and CPO applications share administrative authentication but are
different authorization planes. This comparison prevents a shared frontend,
role switcher, or convenience link from becoming an undocumented access bypass.

| Concern | SuperAdmin application | CPO administration application |
| --- | --- | --- |
| Trusted scope | `PLATFORM` bearer session | `CPO` bearer session plus matching `X-CPO-App-ID` |
| Scope source | `platform_admins` authority; no tenant context | active membership's server-derived CPO ID; header only confirms app routing |
| Current route gate | Every `/api/v1/platform/*` operation requires `PLATFORM` | CPO routes require active membership, matching app ID, and their documented capability; roles are default bundles and explicit `DENY` wins |
| Primary job | Provision/govern the platform and CPO access | Operate one CPO's network, commercial setup, staff, support, and projections |
| May edit CPO legal identity | Yes: platform CPO profile replacement | No: organization is read-only |
| May activate/suspend CPO or rotate app ID | Yes | No |
| May manage platform administrators/subscriptions/workers/mail/security | Yes | No |
| May manage tenant hubs/chargers/GST/tariffs/user groups/settings/integrations | No | Yes, for its own CPO only |
| May read tenant provider secret plaintext | No | No; CPO can replace encrypted credentials but cannot read secrets back |
| May access tenant customer business data | Only bounded platform-owned support/intelligence routes where explicitly documented; never by CPO impersonation | Only tenant-scoped customer/report projections |
| Realtime | platform replay/SSE | operational replay/SSE; CPO notifications are REST-only |

## Non-negotiable client rules

- A SuperAdmin token must never call `/api/v1/cpo/*`; a CPO token must never
  call `/api/v1/platform/*`.
- Do not send `X-CPO-App-ID` to platform endpoints. Do not let a URL, dropdown,
  local-storage key, or request body select CPO scope.
- A platform CPO detail link is not an impersonation link. It can use only the
  documented platform CPO-control/operations/support/intelligence APIs.
- A CPO cannot self-renew a subscription or change its platform lifecycle. The
  platform app must use the manual subscription/lifecycle endpoints.
- The CPO staff permission catalog is not platform RBAC. It does not bypass the
  current ADMIN core-route gate or the separate active-membership support and
  notification boundary. Do not reuse its keys for SuperAdmin navigation decisions.
- Shared UI components may share formatting/types from OpenAPI, but API clients,
  token state, route guards, navigation, and caches must remain scope-separated.
