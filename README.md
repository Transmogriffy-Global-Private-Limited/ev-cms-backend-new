# EV CMS Backend

Multi-tenant Go CMS for CPO organizations. It manages the commercial and
administrative side of an EV charging network. The separate Go HAL remains
responsible for OCPP protocol behavior.

The current implementation is the initial tenancy foundation. See
`docs/PROJECT_STATE.md` for implemented behavior and
`docs/DEVELOPMENT_PLAN.md` for approved sequencing.

## Local requirements

- Go 1.25 or later
- PostgreSQL with a user that can create tables and indexes in the target
  database

## Configure in PowerShell

The service reads configuration from process environment variables; it does not
automatically load `.env`.

```powershell
$env:DATABASE_URL = 'postgres://postgres:password@127.0.0.1:5432/ev_cms?sslmode=disable'
$env:HTTP_ADDR = '127.0.0.1:8080'
```

Do not commit real credentials.

## Run

```powershell
go mod download
go run .
```

Startup connects to PostgreSQL and applies embedded versioned up migrations
before serving requests. Local HTTP defaults to `127.0.0.1:8080`.

Health endpoints:

- `GET /health/live` reports process liveness.
- `GET /health/ready` checks PostgreSQL readiness.

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
