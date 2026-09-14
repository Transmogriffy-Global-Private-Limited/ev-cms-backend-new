# WI-20260914-invoice-customer-presentation

Status: Complete
Owner: Codex
Started: 2026-09-14

## Scope

Customer-facing charging-session invoice and email presentation only: Hub-based
location snapshot/display, optional-field omission, customer wording, and PDF
page progression. No invoice lifecycle, delivery-state, numbering, migration,
HAL, deployment, or commercial-state changes.

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
