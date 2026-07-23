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
Do not commit `.env` or real credentials.

## Run

```powershell
go mod download
go run .
```

Startup connects to PostgreSQL and applies embedded versioned up migrations
before idempotently bootstrapping the configured initial superadmin and serving
requests. Local HTTP defaults to `127.0.0.1:8080`.

Health endpoints:

- `GET /health/live` reports process liveness.
- `GET /health/ready` checks PostgreSQL readiness.

Authentication and tenant integration endpoints are documented in
`docs/AUTHENTICATION.md`. Platform CPO management is documented in
`docs/CPO_ADMINISTRATION.md`.

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
go test ./...
go vet ./...
```

PostgreSQL integration tests run only when `TEST_DATABASE_URL` names an
explicitly selected disposable database:

```powershell
$env:TEST_DATABASE_URL = 'postgres://postgres@127.0.0.1:5432/ev_cms_test?sslmode=disable'
go test ./src/auth ./src/cpo ./src/mail ./src/integrations -count=1
```
