# WI-20260923-customer-cpo-support-rehost

Status: Complete - rehosted and verified
Owner: Codex
Started: 2026-09-23

Development-plan reference: Customer-to-own-CPO support
Related source records:
- `docs/work/archive/WI-20260922-customer-cpo-support.md`
- `docs/work/archive/WI-20260922-customer-cpo-support-actor-ownership.md`
- `docs/work/archive/WI-20260923-customer-cpo-support-owner-immutability.md`

## Objective

Review all support commits after the last hosted source, complete safe
prerequisites, apply the required migrations only after backup and preflight,
rehost first, verify the resulting runtime, then reconcile non-rehost records
and push reviewed changes to `main` if needed.

## Scope

- Customer-to-own-CPO support routes, authorization, tenant isolation,
  idempotency, durable outbox templates, and owner-immutability migrations.
- Configuration parity for the two new frontend support-link templates.
- Source/build/test, database backup and migration, binary rollback, rehost,
  runtime verification, documentation, and publication.

## Non-goals

- No HAL or physical-charger acceptance, unrelated cleanup, force push, or
  destructive data operation.
- Do not expose `.env` values, mail payloads, tokens, or customer content.
- Do not claim SMTP delivery from outbox or health evidence alone.

## Execution and evidence

- Local and `origin/main` were at `ea0f8fe` before rehost. The hosted binary is
  now the candidate SHA-256
  `28b6de6c028c4ccc44ad83d352e2904dba1173d7e7d8f381b3d0b7fe5bd215cc`; live
  migration is 74.
- Source delta adds migrations 72, 73, and 74, support routes, config fields,
  mail templates, and contract/docs updates. The two new `.env` keys were
  added locally from `.env.example` without printing values.
- The pre-migration custom-format dump is
  `/root/evcmsnew-backups/pre-customer-cpo-support-ea0f8fe-20260923T112639Z/devevcmsnewdb.dump`
  (mode `0600`, SHA-256
  `cf5e3ae8f82e2092ac2b68239a5be8d68e9edf47051c5aa2d9b2418c1ab4b4cd`). The
  replaced binary is retained at
  `/root/evcmsnew-backups/pre-customer-cpo-support-ea0f8fe-20260923T112639Z/evcmsnew.before`
  (SHA-256
  `1a2819c96224d1016e3d2655b591993f405954cbe153faa71cfd6735e4e8d4b0`).
- Rehost completed through `rehost-evcmsnew -t 0 --no-tail`. The service PID is
  62535 with zero restarts. Local/public health, readiness, Swagger, OpenAPI,
  Caddy, route-auth boundaries, process/install hashes, worker status, and
  configuration key parity were verified.
- Focused support/config/migration/route tests, `go test -p 1 ./...`,
  `go vet -p 1 ./...`, production build, `go mod verify`, and `git diff --check`
  passed. `TEST_DATABASE_URL` is unset and `pwsh` is unavailable. SMTP,
  HAL, and physical-charger acceptance remain unverified.

## Publication

Documentation was reconciled after runtime verification. The explicit user
authorization covers normal publication to `main`; commit and remote-SHA
verification remain the final release actions for this work item.
