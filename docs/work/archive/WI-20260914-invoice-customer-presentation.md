# WI-20260914-invoice-customer-presentation

Status: Complete
Owner: Codex
Started: 2026-09-14
Last updated: 2026-09-14 (rehost and runtime verification complete)

## Scope

Customer-facing charging-session invoice and email presentation only: Hub-based
location snapshot/display, optional-field omission, customer wording, and PDF
page progression. No invoice lifecycle, delivery-state, numbering, migration,
HAL, or commercial-state changes.

## Verification

Focused invoice presentation tests cover Hub snapshot/email semantics, hubless
fallback, absent optional labels, one-page output, and forced multi-page
Unicode content. They passed on 2026-09-14.

Also passed on 2026-09-14:

- `go test -p 1 ./...`
- `go vet -p 1 ./...`
- `go build -p 1 ./...`
- `git diff --check`

No database action was required or performed.

## Deployment verification

- Rehosted source revision `3358ec8` after building and verifying binary
  SHA-256
  `d4f08a0178d9460cacd951e461ac467e5f432395005be2d358e314084919b33c`.
  Prior binary is retained at
  `/root/evcmsnew-backups/pre-invoice-presentation-3358ec8-20260914T122510+0530/evcmsnew`
  (SHA-256
  `7c4fd89d00e0f19e97594017453aa9f0fbe78a1770cad84c23b09facab86a1a8`).
- No migration was needed; `000069` remains current. Existing 71 READY PDFs
  remained immutable and hash/size/signature/mode verified; one pre-existing
  delivery remained SENT. No email was sent or retried.
- Focused and full Go tests, vet, and OpenAPI parity passed before rehost.
  Service and required workers are healthy; local/public endpoints,
  245-operation OpenAPI, Caddy validation, and post-restart error-log scan
  passed.
- `TEST_DATABASE_URL` and `pwsh` are unavailable; SMTP attachment acceptance
  remains untested.
