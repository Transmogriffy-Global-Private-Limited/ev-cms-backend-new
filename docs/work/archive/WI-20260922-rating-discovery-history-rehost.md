# WI-20260922-rating-discovery-history-rehost

Status: Complete — source reviewed, rehosted, and runtime-verified
Owner: Codex
Started: 2026-09-22
Completed: 2026-09-22

Development-plan reference: Customer ratings / customer discovery
Related feature work: `docs/work/archive/WI-20260922-rating-discovery-and-history.md`
Issue/PR reference: None

## Outcome

Reviewed the customer/CPO rating-discovery and history changes advanced after
the prior `9e2940c` deployment, fixed release-blocking pagination validation,
rehosted the tested binary, and reconciled the API and deployment records.

## Scope completed

- Compared the source delta from hosted revision `9e2940c` through
  `fbcfc39285e628a5062c77235f9efb0b7b067d9b` and found no schema/configuration
  or `.env.example` change.
- Fixed malformed, out-of-range, non-finite, and sort-incompatible cursor
  acceptance in customer rating history, customer charger discovery, CPO
  charger discovery, and CPO customer-rating lists. Nullable `null` cursors
  remain supported only for nullable rating sorts; typed timestamp/integer
  values are bound for customer rating-history pagination.
- Source fix commit: `b924cae84fa35a2168e6e545550fc243257ff9b5`.
- Rehosted candidate SHA-256:
  `1a2819c96224d1016e3d2655b591993f405954cbe153faa71cfd6735e4e8d4b0`.
  The replaced executable is retained at
  `/root/evcmsnew-backups/pre-rating-cursor-validation-fbcfc39-20260922T115310+0530/evcmsnew`
  (SHA-256
  `85fcdad6285df7759f0438ab1949cb701b3b74f2a38d44473dd0c2925ab8af7a`).

## Verification

- Focused `customerauth`/`cpo` tests, route/OpenAPI parity, full
  `go test -p 1 ./...`, `go vet -p 1 ./...`, production build, and
  `git diff --check` passed.
- Runtime PID 225852 is active with zero restarts and matching process/install
  SHA. Local and HTTPS liveness, readiness, Swagger, and OpenAPI return 200;
  both new customer/CPO rating routes return 401 without credentials. Source,
  local, and public OpenAPI each contain 250 operation IDs.
- Database `devevcmsnewdb` remains at migration
  `000071_customer_ratings.up.sql`; all seven required/current workers report
  HEALTHY; mail outbox remains 518 SENT. Caddy validates; no post-start
  error/panic/fatal log matches were found.
- `.env` is mode `0600` with 64 key names matching `.env.example` exactly and
  no blank entries. No new environment fields were required.

## Limits and publication

- PostgreSQL integration tests were skipped because `TEST_DATABASE_URL` is
  unset; the PowerShell docs verifier was unavailable because `pwsh` is not
  installed. Physical charger/OCPP and SMTP acceptance were not performed.
- The source and these release records are included in the requested normal
  fast-forward publication to `origin/main`; the final push/ref verification
  is performed after documentation checks.
