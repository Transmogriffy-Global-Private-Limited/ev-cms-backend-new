# Identity, Tenancy, and Application Identity

## Audience

Read this before implementing authentication UI, tenant middleware, staff
management, billing, HAL integration, or support tooling. The identifiers below
answer different authorization questions.

## Why There Are Three Different Identifiers

The CMS deliberately separates a person, an organization, and an application
installation:

- A `user` is one global login identity.
- A `CPO` is a tenant organization that buys and operates the CMS.
- A CPO `app_id` identifies the CPO's current client application deployment.

These values answer different questions and are not interchangeable.

## Glossary and Source of Truth

| Concept | Durable source | Meaning | Not equivalent to |
|---|---|---|---|
| User | `users` | One global login identity | CPO, role, customer |
| Platform admin | `platform_admins` | Platform control-plane authority | Tenant-data access |
| CPO | `cpos` | Tenant organization boundary | App ID or user |
| Membership | `cpo_memberships` | Staff role inside one CPO | Customer relationship |
| Customer | `customers` | Consumer relationship inside one CPO | Staff membership |
| Session | `auth_sessions` | Revocable browser/device authority | Access token alone |
| App ID | `cpos.app_id` | Current client deployment identity | Secret or tenant selector |

## Authorization Planes

A user becomes a platform superadmin only through `platform_admins`. A user
becomes CPO staff only through an active `cpo_membership`. The same identity can
belong to several CPOs, but each authenticated session selects exactly one
scope:

```text
password + email OTP
        |
        v
PLATFORM session ----> platform control-plane APIs
        or
CPO session ----------> one CPO, one fixed membership role
```

Platform authority does not silently grant access to tenant provider secrets
or business records. CPO authority comes from the validated session and current
database state, never from an ID supplied by a caller.

### Implemented authority matrix

| Capability | Public | Platform | CPO ADMIN | Dormant role values |
|---|---:|---:|---:|---:|
| Login and recovery | Yes | Yes | Yes | No |
| Own identity, profile, and sessions | No | Yes | Yes | No |
| Create/activate/suspend CPO | No | Yes | No | No |
| Assign live CPO app ID | No | Yes | No | No |
| Manage initial network/GST/tariff records | No | No | Yes | No |
| Manage Razorpay credential row | No | No | Yes | No |
| Read Razorpay plaintext over HTTP | No | No | No | No |

The database retains `OWNER`, `OPERATOR`, and `VIEWER` enum values only as
future-compatible storage capacity. No current API creates those memberships
and authentication accepts only active `ADMIN` membership for the CPO plane.

## Session and Token Model

OTP verification creates a durable session, a short-lived encrypted access
token, and a one-time opaque refresh token. Only the refresh-token hash is
stored. A session selects exactly one platform or CPO authority.

The access token is not standalone truth. Every protected request decrypts and
verifies it, loads its session, confirms the token/session scope agrees, and
revalidates current user, authority, membership, and CPO status. This is why
logout, suspension, password reset, and authority removal take effect without
waiting for token expiry.

Refresh success atomically marks the submitted token used and creates a
replacement. Observed reuse revokes the whole session. A client must therefore
replace its stored refresh token after every successful refresh.

## What `X-CPO-App-ID` Does

Tenant business routes compare `X-CPO-App-ID` with the current app ID of the
CPO already established by the authenticated principal. The header:

- does not authenticate a user;
- does not select a tenant;
- is not a secret;
- cannot expand a session's authority.

It is routing and deployment identity metadata. Authentication, recovery,
platform control-plane, and health routes do not require it, avoiding a
catch-22 when a client does not yet know the current value.

Every CPO starts with a globally unique `cpo_dummy_...` app ID. A superadmin can
later assign or rotate a live app ID without adding a commercial dependency.
Login verification, refresh, and `GET /api/v1/auth/me` return the current ID so
a legitimate client can recover after rotation.

### Catch-22 exemptions

The app-ID header is not required on health, docs, authentication, recovery,
password, session, or platform control-plane routes. These exemptions let a
client recover a rotated app ID and let a temporary-password user change the
password before entering tenant operations.

## Durable Invariants

- Tenant-owned rows carry a trusted CPO identifier.
- A CPO session contains exactly one CPO and role.
- A platform session contains neither.
- Protected requests revalidate the session, identity, authority, membership,
  and CPO status in PostgreSQL.
- Suspension and session revocation take effect without waiting for access
  token expiry.
- The HAL has a different ownership boundary and cannot derive CMS tenant
  access by writing CMS tables.

## Rule for Every Future Tenant Query

```text
bearer token
  -> validated principal
  -> principal CPO ID
  -> role and ownership policy
  -> app-ID comparison where required
  -> query constrained by the trusted CPO ID
```

Never take tenant authority from a request-body `cpo_id`, app-ID header, global
user ID, or superadmin marker.

## Worked Multi-CPO Example

If one person administers CPO A and views CPO B, there is one user row and two
membership rows. Login to each CPO creates a separate session. The CPO A token
cannot use CPO B's app ID or access CPO B's records. A global password change
revokes both sessions.

See `../../AUTHENTICATION.md` and `../../CPO_ADMINISTRATION.md` for the concrete
HTTP contracts.
