# EV CMS Backend

Multi-tenant Go CMS for CPO organizations. It manages the commercial and
administrative side of an EV charging network. The separate Go HAL remains
responsible for OCPP protocol behavior.

The current implementation includes the tenancy/schema foundation and the
administrative authentication and credential boundary. See
`docs/AUTHENTICATION.md` for the API and configuration contract,
`docs/CPO_ADMINISTRATION.md` for platform CPO provisioning and app identity,
`docs/PROJECT_STATE.md` for implemented behavior, and
`docs/DEVELOPMENT_PLAN.md` for approved sequencing.

CPO access is granted or removed manually by platform superadmins through
activation and suspension. This CMS does not expose tenant subscription,
entitlement, platform-invoice, or platform-payment management APIs.
The Superadmin CPO surface also provides collection search/filter/cursors,
business-profile maintenance, lifecycle decision evidence, primary-admin
recovery/onboarding status, and scoped CPO administrative-session revocation.

## Local requirements

- Go 1.25 or later
- PostgreSQL with a user that can create tables and indexes in the target
  database

## Configure

Copy `.env.example` to the ignored `.env` file and replace every required
placeholder. The service loads `.env` first and process environment variables
override its values.

```powershell
Copy-Item .env.example .env
```

The initial superadmin email and password, independent authentication and
encryption keys, and SMTP settings are required by the credential surface.
`MAIL_ENABLED=true` is required for administrative login and password recovery.
The checked-in example selects Hostinger implicit TLS on
`smtp.hostinger.com:465` with `team@transev.in`; supply only the mailbox
password in the ignored `.env` or process environment. Do not commit `.env` or
real credentials.

## Run

```powershell
go mod download
go run .
```

Startup connects to PostgreSQL and applies embedded versioned up migrations
before idempotently bootstrapping the configured initial superadmin and serving
requests. The code default remains `127.0.0.1:8080`; the current development
example opts into temporary unrestricted network access with:

```dotenv
HTTP_ADDR=0.0.0.0:8080
CORS_ALLOW_ALL=true
```

Other machines use `http://<server-ip>:8080`, not `http://0.0.0.0:8080`.
Authentication and tenant authorization remain enforced.

Health endpoints:

- `GET /health/live` reports process liveness.
- `GET /health/ready` checks PostgreSQL readiness.

Authentication and tenant integration endpoints are documented in
`docs/AUTHENTICATION.md`. Platform CPO management is documented in
`docs/CPO_ADMINISTRATION.md`. Public CPO-scoped customer signup is documented
with the complete app-user login, session, `me`, and password surface in the
HTTP contract and OpenAPI explorer.

Interactive API documentation is served from the same listener:

- `http://<server-ip>:8080/docs/` — embedded Swagger UI with **Try it out**
- `http://<server-ip>:8080/openapi.yaml` — canonical OpenAPI 3.1 contract

Both routes are registered only when `API_DOCS_ENABLED=true` (the backward-
compatible default and current development setting). Set it to `false` in
sensitive deployments; restart is required.

The complete human endpoint handoff is
`docs/contracts/api/administrative-http-api.md`.

## Migrations

Run all pending up migrations:

```powershell
go run ./cmd/migrate -direction up
```

Roll back only the latest applied migration:

```powershell
go run ./cmd/migrate -direction down
```

Down migrations remove data. Inspect the selected database and migration before
running them. The application never runs a down migration automatically.

## Verify

```powershell
.\scripts\verify-docs.ps1
go test ./...
go vet ./...
```

PostgreSQL integration tests run only when `TEST_DATABASE_URL` names an
explicitly selected disposable database:

```powershell
$env:TEST_DATABASE_URL = 'postgres://postgres@127.0.0.1:5432/ev_cms_test?sslmode=disable'
go test ./src/auth ./src/cpo ./src/mail ./src/integrations -count=1
```
