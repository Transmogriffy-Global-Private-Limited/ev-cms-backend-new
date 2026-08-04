# CPO Backend AI-Agent Handoff

## Purpose

Give this document to any AI agent assigned to the CPO side of this repository.
It is the fast orientation and execution guide. It does not replace the
authoritative source code, migrations, OpenAPI, or approved plans; it tells the
agent where truth lives, what is already real, what is deliberately absent, and
how to complete one safe end-to-end slice without relying on chat history.

The governing rule is:

> Inspect current data flow and contracts, implement only the assigned coherent
> slice, preserve tenant and HAL boundaries, verify real behavior, and update
> every affected contract in the same change.

## Read This First

Before planning or editing, read these files in order:

1. `AGENTS.md`
2. `docs/CPO_BACKEND_AGENT_HANDOFF.md` (this file)
3. `docs/DEVELOPMENT_PLAN.md`
4. `docs/PROJECT_STATE.md`
5. `docs/SCHEMA.md`
6. `docs/AUTHENTICATION.md`
7. `docs/guides/workflows/cpo-admin-network-configuration.md`
8. `docs/guides/workflows/app-user-authentication.md`
9. `docs/contracts/api/administrative-http-api.md`
10. `docs/contracts/openapi/openapi.yaml`
11. `docs/integrations/ocpp-hal-boundary.md`
12. The relevant ADR and detailed plan under `docs/decisions/` and
    `docs/plans/`

Then inspect:

```powershell
git status --short --branch
git log --oneline -n 10
Get-ChildItem .\src\cpo, .\src\customerauth, .\src\integrations -Recurse -File
```

Do not edit until the assigned behavior, current branch, human changes, route
wiring, tables, queries, callers, tests, and documentation surfaces are known.

## Product and Ownership Model

This is a multi-tenant CMS sold to CPO organizations.

- A CPO is a tenant organization, not a global user role.
- `users` are global login identities.
- `platform_admins` grants platform Superadmin authority.
- `cpo_memberships` grants staff authority inside exactly one CPO.
- `customers` links one global identity to one CPO as an app user.
- A staff membership and a customer relationship are separate things.
- The same global identity may belong to multiple CPOs, but every session
  selects exactly one scope.

There are three authentication planes:

| Plane | Scope | Trusted tenant context | Current authority |
| --- | --- | --- | --- |
| Platform | `PLATFORM` | None | Explicit `platform_admins` row |
| CPO staff | `CPO` | Session `cpo_id` | Active `ADMIN` membership only |
| App user | `CUSTOMER` | Session `cpo_id` and `customer_id` | Active tenant customer |

Never turn a customer into CPO staff. Never treat a CPO as a user role. Never
let a request body, query parameter, or arbitrary header select tenant scope.

## Permanent CPO Security Rules

1. Current callable CPO staff authority is `ADMIN` only.
2. `OWNER`, `OPERATOR`, and `VIEWER` remain dormant persisted enum values. No
   API creates them and they grant no current login or operation authority.
3. Staff management requires a separately approved lifecycle/capability plan.
4. A CPO business request requires a validated CPO bearer session and the
   current `X-CPO-App-ID`.
5. `X-CPO-App-ID` is non-secret application identity metadata. It validates the
   authenticated tenant; it never authenticates a caller or selects a CPO.
6. Login, refresh, password, and `/api/v1/auth/me` avoid the app-ID catch-22.
   Login/refresh/`me` return the current app ID.
7. Temporary-password CPO sessions cannot use business APIs until the password
   is changed.
8. Every tenant-owned query, mutation, job, cache key, event, file, and
   broadcast must retain trusted `cpo_id` context.
9. Cross-CPO references return not found or a documented scoped error; never
   reveal that another tenant owns the identifier.
10. A platform Superadmin does not automatically receive tenant-business or
    decrypted Razorpay access.
11. HTTP completion logging may include only the safe schema in
    `docs/contracts/internal/http-request-logging.md`; never add bodies, raw
    paths, queries, credentials, personal fields, app IDs, API messages, or
    panic values.
12. `LOG_LEVEL=DEBUG` is for safe lifecycle/error classification only. Never
    interpret DEBUG as permission to add payload, credential, personal-data,
    query, raw-path, or raw-error logging.

## System Boundary: CMS Versus OCPP HAL

`OCPPHAL_Go` remains a separate application and database.

The HAL owns:

- OCPP sockets, protocol state, requests, responses, and correlation;
- charger/connector live protocol state;
- raw meter communication;
- command delivery and device recovery;
- the exact OCPP transaction ID.

The CMS owns:

- CPO and customer identities and authorization;
- commercial hub/charger/connector projections;
- tariffs, taxes, wallets, settlement, reporting, and audit policy;
- durable CMS charging-session projections.

Forbidden shortcuts:

- do not embed or copy the HAL into this process;
- do not write the HAL database or share ORM models;
- do not infer successful command delivery from a CMS row update;
- do not invent or replace a HAL transaction ID;
- do not use browser/WebSocket presence as durable session truth;
- do not implement remote start/stop without idempotency, correlation, retry,
  restart recovery, and reconciliation.

Current compatibility fact: the HAL allocates a positive OCPP signed-32-bit
transaction ID. The CMS schema stores it as PostgreSQL `integer`/Go `int32`.
Some HTTP/legacy payloads serialize the decimal digits as a string. Preserve
the exact numeric value across conversion; never substitute the CMS session
UUID or a row UUID.

## Current Implemented CPO Surface

The authoritative machine contract is
`docs/contracts/openapi/openapi.yaml`. The source currently has 111 total
HTTP operations across all planes. Runtime/OpenAPI parity is tested.

### Administrative authentication

Base: `/api/v1/auth`

- `POST /login`
- `POST /2fa/verify`
- `POST /2fa/resend`
- `POST /refresh`
- `POST /password/forgot`
- `POST /password/reset`
- `GET /me`
- `GET /sessions`
- `DELETE /sessions/{session_id}`
- `POST /logout`
- `POST /logout-all`
- `POST /password/change`

CPO ADMIN login requires an active identity, active CPO, active ADMIN
membership, password, and mail OTP. Access tokens are signed then encrypted;
refresh tokens are opaque, hashed, rotating, and reuse-detecting.

### CPO ADMIN identity and organization

Base: `/api/v1/cpo`

- `GET /organization` returns a tenant-safe, read-only CPO projection.
- `GET /admin/profile` returns the global administrator identity profile.
- `PATCH /admin/profile` updates full name and optional phone only.

Superadmin remains the only writer of CPO organization fields. The tenant
organization response omits privileged lifecycle reason and platform actor ID.

### Network and pricing

- `POST/GET /hubs`
- `GET/PATCH /hubs/{hub_id}`
- `POST/GET /chargers`
- `GET/PATCH/DELETE /chargers/{charger_id}`
- `POST/GET /gsts`
- `GET/PATCH /gsts/{gst_id}`
- `POST/GET /tariffs`
- `GET/PATCH /tariffs/{tariff_id}`

Current behavior:

- collections use bounded keyset pagination;
- IDs are server-generated;
- charger creation atomically creates its initial connectors and audit record;
- the six-character public charger ID, CMS UUID, connector UUIDs, and
  `ocpp_identity` are different identifiers;
- `ocpp_identity` is only a mapping value; no HAL call occurs;
- CPO callers cannot write live charger/connector status;
- exact tax and tariff decimals serialize as strings;
- blank tariff currency becomes `INR`;
- related hub, charger, GST, and group records must belong to the same CPO;
- referenced chargers return `409 charger_in_use` rather than cascading data
  loss;
- GST and tariff retirement uses `is_active=false`;
- hub deletion, GST deletion, tariff deletion, and connector add/remove after
  charger creation are not implemented;
- optional tariff relationships can currently be omitted or replaced, but not
  explicitly cleared to null.

### CPO integration credentials

Base: `/api/v1/cpo/integrations`

- list safe metadata;
- get safe provider metadata;
- put/rotate Razorpay credentials;
- delete credentials.

Secret plaintext is encrypted at rest, accepted write-only, resolved only by
an authorized internal CPO operation, and never returned to CPO or platform
frontends. Razorpay API execution and webhooks are not implemented.

### App-user signup and authentication

Base: `/api/v1/app/auth`

Implemented:

- CPO-scoped signup start, verify, and resend;
- password plus mail-OTP login, verify, and resend;
- refresh rotation;
- current customer (`me`);
- customer-scoped session listing/revocation/logout/logout-all;
- password recovery/reset and authenticated change. Forgot-password stays
  generic while the eligible recipient's encrypted email supplies the recovery
  ID, code, and expiry required by reset.

Successful signup transactionally creates or reuses the global identity,
creates one CPO-scoped customer, and creates its zero-balance INR wallet.

Not implemented: a CPO ADMIN customer directory, customer suspension API,
groups/RFID management APIs, customer profile editing, or verified email
change. Do not confuse implemented customer self-authentication with CPO-side
customer administration.

## Current Data Model: Schema Is Capacity, Not Behavior

Migration one contains tables for:

- CPOs, memberships, customers, user settings, and user groups;
- hubs, chargers, connectors, group access links, and favorites;
- GST and tariffs;
- wallets, wallet transactions, charging sessions, payments, and audit logs.

The presence of a table or Go model does not mean its workflow exists. Most
customer administration, charging, wallet, payment, and reporting tables have
no business API yet.

Trust actual evidence in this order:

1. applied SQL constraints and indexes;
2. current route wiring;
3. handler/service queries and transactions;
4. tests and verification scripts;
5. OpenAPI and human contracts;
6. filenames and legacy names last.

`src/cpo/repository.go` is currently an empty package file. Do not assume a
repository layer exists because of the filename. Existing CPO behavior lives
primarily in `src/cpo/service.go`, with HTTP wiring in `src/cpo/router.go` and
contracts in `src/cpo/schemas.go`.

## Code and Documentation Map

| Concern | Current owner |
| --- | --- |
| Startup and dependency wiring | root `main.go`, `src/routes/routes.go` |
| Administrative auth/session boundary | `src/auth/` |
| App-user auth/session boundary | `src/customerauth/` |
| Platform CPO plus current CPO business service | `src/cpo/` |
| Encrypted provider credentials | `src/integrations/` |
| Durable mail | `src/mail/` |
| Platform events/audit/workers | `src/platformops/` |
| Encryption/password/token helpers | `src/security/` |
| Persistent models | `src/models/` |
| Database connection, seeding, migrations | `db/` |
| Canonical REST contract | `docs/contracts/openapi/openapi.yaml` |
| Exhaustive human HTTP contract | `docs/contracts/api/administrative-http-api.md` |
| Current truth and next approved work | `docs/PROJECT_STATE.md`, `docs/DEVELOPMENT_PLAN.md` |
| CMS/HAL boundary | `docs/integrations/ocpp-hal-boundary.md` |

Do not create ceremonial repository/service abstractions. Add a boundary only
when it owns real policy, transaction behavior, or a second implementation.

## Existing End-to-End Flows

### CPO onboarding to first business request

```text
Superadmin creates CPO
→ CPO, primary ADMIN membership, audit, platform event, and encrypted mail job
  commit atomically
→ CPO starts PENDING with generated dummy app ID
→ Superadmin explicitly activates CPO
→ ADMIN enters email/password/CPO ID
→ backend validates active ADMIN authority and sends mail OTP
→ ADMIN verifies OTP and receives encrypted access token, refresh token, and
  current app ID
→ temporary-password gate requires password change
→ frontend sends bearer token plus X-CPO-App-ID
→ each request revalidates session, user, CPO, ADMIN membership, and current
  app ID from PostgreSQL
→ tenant business handler derives CPO ID from the principal
```

### Initial network configuration

```text
read CPO organization
→ maintain ADMIN identity profile
→ create hub
→ create charger with initial connectors
→ create/choose GST
→ create tariff referencing same-CPO records
→ query authoritative REST projections
```

This flow ends in CMS inventory. It does not register the charger in the HAL or
prove the device is online.

### App-user account creation

```text
active CPO app supplies current X-CPO-App-ID
→ signup challenge and encrypted OTP mail
→ verify challenge
→ create/reuse global identity + tenant customer + INR wallet atomically
→ customer password login + OTP
→ CUSTOMER-scoped session and app-ID validation
```

## Remaining CPO Roadmap

This section is dependency guidance, not blanket implementation approval. The
human must assign or approve a specific slice before an agent changes code.

### Candidate A: CPO customer administration

Can be designed without HAL transport:

- bounded customer list/detail/search/filter;
- customer activation/suspension policy;
- safe identity/contact visibility;
- customer session revocation;
- user groups and customer assignment;
- RFID/idTag issuance, assignment, replacement, block, and revoke;
- hub/charger group-access management;
- audit, mail, OpenAPI, FE contract, and PostgreSQL tenant-isolation tests.

Do not overwrite global identity credentials/profile while attaching an
existing user to another CPO.

### Candidate B: Complete network lifecycle

- explicit provisioned/active/retired semantics distinct from live OCPP state;
- connector add/remove/replacement policy;
- serial number and OCPP mapping uniqueness/ownership;
- safe archival instead of destructive deletion where history depends on a
  network record.

### Required design before billing: immutable tariff versions

The current tariff CRUD is an initial configuration surface. Before charging
sessions consume tariffs, define effective periods, draft/published state,
immutable versions, applicability priority, conflict resolution, and tariff/tax
snapshots. A running or completed session must never be silently repriced by a
later tariff edit.

### Required cross-repository contract: CMS/HAL integration

Before implementing transport, approve:

- independent service authentication and authorization;
- CPO/charger/OCPP identity mapping;
- command and callback schemas;
- idempotency and deduplication keys;
- correlation and ordering;
- retry, timeout, partial failure, and restart behavior;
- reconciliation when either service was unavailable;
- exact transaction-ID and integer-Wh semantics;
- versioning, observability, and secret handling.

### Charging lifecycle

After tariff and HAL contracts:

- durable start/stop command intents;
- idempotent HAL callbacks;
- active/completed/failed session state machine;
- exact HAL transaction ID preservation;
- meter start/stop Wh and monotonicity;
- tariff/tax snapshots;
- lost-response, duplicate, reconnect, restart, and reconciliation behavior;
- CPO list/detail/operational controls plus recoverable realtime invalidation.

Already-active sessions must remain stoppable, callback-ingestible, and
billable even if the CPO is suspended.

### Wallet, settlement, and customer charging payment

After charging lifecycle:

- exact immutable session charge calculation;
- atomic idempotent wallet ledger settlement;
- provider payment/top-up initiation if approved;
- verified webhook and reconciliation;
- reversal/refund rules and receipts.

This is charging settlement between a CPO and its customers. It is not the
retired platform subscription/invoice/payment system. CPO platform access
remains manual Superadmin control.

### Reporting and tenant realtime

After authoritative workflows exist:

- bounded operational and financial aggregates;
- exports with explicit date limits;
- CPO audit visibility;
- tenant-scoped event delivery, replay, deduplication, and REST recovery.

### Staff management remains deferred

Do not activate dormant roles until a plan covers invite/acceptance, role
matrix, last-admin protection, removal/suspension, session revocation,
recovery, audit, mail, frontend contracts, and migration/backfill behavior.

## How to Execute an Assigned Slice

### 1. State the observable outcome

Example:

> An authenticated CPO ADMIN can list only customers belonging to the session
> CPO with bounded keyset pagination; another tenant's customer ID is never
> disclosed.

### 2. Map the complete affected surface

Include every applicable item:

- migration and constraints;
- models and constants;
- trusted principal helpers;
- request/response schema;
- validation;
- service/domain decision;
- transaction and locking;
- tenant-scoped queries;
- audit/event/mail/outbox;
- handler and route registration;
- OpenAPI and human/FE contract;
- focused and PostgreSQL tests;
- route/OpenAPI parity;
- development plan, project state, changelog, ADR/integration guide.

### 3. Define failure behavior before coding

Decide:

- duplicate request behavior;
- concurrent request behavior;
- not-found versus conflict semantics;
- transaction rollback boundaries;
- retry and idempotency policy;
- process crash and restart behavior;
- downstream outage behavior;
- what the frontend should do after timeout or stale state.

### 4. Implement the smallest complete vertical slice

No TODO handlers, fake success, unregistered routes, undocumented payloads, or
events without consumers/recovery. Do not combine unrelated cleanup.

### 5. Verify narrow to broad

```powershell
gofmt -w <changed-go-files>
go test ./src/<changed-package> -count=1
.\scripts\verify-docs.ps1
go test ./src/routes -run TestOpenAPIContractMatchesRuntimeRoutesAndServesUI -count=1
go test ./...
go vet ./...
git diff --check
git status --short
```

For persistence behavior, set `TEST_DATABASE_URL` only to an explicitly
disposable database and run the focused PostgreSQL lifecycle test. Never point
such a test at the development VPS or a valuable local database.

## New Endpoint Checklist

Every new CPO endpoint must include, where applicable:

- stable method/path and operation ID;
- authentication and correct session plane;
- current app-ID validation;
- ADMIN policy or an explicitly approved future capability;
- trusted tenant derivation;
- request-size and strict-JSON behavior;
- field validation and normalization;
- same-CPO related-record checks;
- transaction/lock boundaries;
- idempotency or explicit retry semantics;
- audit and durable event/outbox behavior;
- canonical error envelope/status/code;
- route wiring;
- OpenAPI request, response, errors, examples, auth, and descriptions;
- exhaustive human contract and FE workflow update;
- focused tests, runtime/OpenAPI parity, and PostgreSQL verification;
- project-state, plan, changelog, and ADR updates when meaning changed.

## Tenant-Isolation Review Checklist

Before completion, prove:

- CPO ID comes from a validated principal;
- every base query contains the trusted CPO scope;
- every related ID is checked within the same CPO;
- composite foreign keys enforce cross-table tenant ownership where needed;
- pagination cursors cannot escape filters or tenant scope;
- background jobs carry CPO context;
- audit records use the correct actor and CPO;
- events contain safe metadata only;
- caches/files/realtime channels, if introduced, are CPO-scoped;
- error behavior does not reveal another tenant's row;
- platform and customer sessions cannot call CPO ADMIN operations;
- suspended CPO behavior matches the operation's recovery requirements.

## Database and Migration Rules

- PostgreSQL is durable truth.
- Fifteen migrations are already deployment history. Do not edit an applied
  migration to change new behavior; add the next forward migration.
- Migration fifteen is the current deployed migration. It adds tariff
  effective-date columns and a PostgreSQL overlap constraint.
- Prefer additive, backward-compatible migrations.
- Never drop, truncate, or broadly delete without explicit human approval.
- Use composite keys/foreign keys to protect tenant relationships.
- Use exact numeric types for money and taxes; do not use binary floating point
  for financial calculations.
- Energy used for billing is integer Wh.
- Use transactions for a business mutation plus audit/event/outbox work.
- Use constraints and idempotency keys for concurrency invariants, not only
  application pre-checks.
- Migrations seven/eight are historical. Migration nine preserves the former
  commercial prototype in `retired_commercial`; migration twelve restores only
  manual subscription tables; migration thirteen returns dormant entitlement
  tables to `retired_commercial`; migration fourteen adds the deployed
  Superadmin control-plane records; migration fifteen adds deployed tariff
  effective-date enforcement. Platform billing and automatic workers remain
  retired; subscription state never controls CPO access.

## API and Error Conventions

- JSON media type, strict request decoding, one JSON object, bounded body.
- Errors use `{"error":{"code":"...","message":"..."}}`.
- Protected credential and profile responses use `Cache-Control: no-store`.
- UUIDs are server-generated unless a contract explicitly says otherwise.
- Times are RFC3339 UTC.
- Exact decimals are returned as JSON strings.
- Collection endpoints are bounded; prefer stable keyset cursors.
- GET is safe and side-effect-free.
- Unknown fields and invalid enum values must not be silently accepted.
- Do not expose password hashes, OTPs, tokens, encrypted payloads, provider
  secrets, internal mail bodies, or unnecessary tenant PII.

Use the OpenAPI and human contract for the exact currently implemented field
rules. Do not guess from model JSON tags.

## Old CMS and Legacy Data

The old CMS is reference evidence, not architecture to restore.

When a legacy behavior must be preserved:

1. inspect actual tables, columns, constraints, SQL queries, payloads,
   callbacks, and executable flow;
2. inspect the patched working behavior, not names alone;
3. identify the real invariant and integration consumer;
4. reimplement that invariant inside the new ownership model;
5. do not copy global roles, cross-tenant assumptions, hidden coupling, or
   direct HAL database writes.

Names in the old CMS can be misleading. Data flow wins.

## Documentation Discipline

- OpenAPI is the machine-readable source of truth.
- `docs/contracts/api/administrative-http-api.md` owns exhaustive HTTP
  semantics.
- Focused workflow guides teach the human/FE sequence.
- Integration documents own cross-system behavior and recovery.
- `docs/DEVELOPMENT_PLAN.md` distinguishes approved, active, deferred, and
  proposed work.
- `docs/PROJECT_STATE.md` describes implemented reality only.
- `docs/AI_CHANGELOG.md` records meaningful agent-assisted changes and actual
  verification.

Do not claim planned behavior is implemented. Do not claim a PostgreSQL or
live integration check ran when it did not.

## Git and Collaboration Rules

- Treat `main` and `anubhab-work` as the authoritative lines unless the human
  explicitly changes that decision.
- A contributor branch may lag or contain input history; do not overwrite,
  merge, delete, or publish it without explicit instruction.
- Preserve uncommitted human work.
- Do not stage, commit, push, merge, rebase, switch branches, delete branches,
  deploy, or modify remotes unless explicitly requested.
- When authorized to commit, stage an explicit reviewed file list and run all
  required checks first.
- When authorized to push multiple authority branches, fetch first and require
  fast-forward safety; never force-push implicitly.

## Prohibited Assumptions

Do not assume:

- a model means an API exists;
- `repository.go` owns current data access;
- all persisted CPO roles are callable;
- app ID is a secret or tenant selector;
- platform Superadmin may inspect tenant business data;
- charger creation contacts the HAL;
- CMS status is live protocol truth;
- mutable tariff rows are safe for completed-session billing;
- a callback is delivered exactly once;
- one successful happy path proves restart/retry correctness;
- old CMS names correctly describe behavior;
- a successful unit test proves PostgreSQL constraints or HTTP wiring;
- documentation may be updated later.

## Definition of Done

An assigned CPO slice is done only when:

- the initiating call reaches the intended decision;
- validation, auth, ADMIN policy, app ID, tenant scope, and ownership pass;
- durable state and invariants are correct under concurrency;
- audit/event/mail/downstream effects share the required transaction boundary;
- duplicate, retry, timeout, crash, restart, and reconciliation behavior is
  explicit;
- every producer and consumer agrees on the contract;
- focused, PostgreSQL/integration, route/OpenAPI, full Go, vet, and diff checks
  pass as applicable;
- Swagger exposes the same authoritative operation;
- educational, integration, API, plan, state, changelog, and ADR documentation
  match reality;
- unrelated human work is untouched;
- publishing/deployment occurred only with explicit permission.

## Required Final Handoff From the Agent

The agent's final report must state:

1. observable behavior delivered;
2. files and surfaces changed;
3. tenant/auth/HAL decisions;
4. migrations and data implications;
5. commands and tests actually run;
6. behavior not verified and why;
7. compatibility and frontend implications;
8. commit/push/deployment status;
9. the next sensible but not automatically approved slice.

If any of these facts cannot be established from the repository, the agent
must inspect further or report the uncertainty instead of inventing an answer.
