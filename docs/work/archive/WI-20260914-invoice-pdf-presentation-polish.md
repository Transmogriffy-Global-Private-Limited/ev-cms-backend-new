# WI-20260914-invoice-pdf-presentation-polish

Status: Complete
Owner: Codex
Started: 2026-09-14
Completed: 2026-09-14

## Outcome

Corrected only the approved charging-invoice PDF V2 presentation defects: Charges-header label color/metric alignment, exact Session-details page fitting, and PDF structural validity.

## Scope

- `src/invoice/service.go` Canvas V2 rendering and writer dependency.
- Focused invoice renderer regression tests.

## Non-goals

- Invoice design, snapshots/hydration, email, numbering, delivery, migrations, APIs, HAL, deployment, or database changes.

## Delivered state

Header labels carry white/bold font faces and use their actual bounds to center within the unchanged blue 8 mm cells; Description and Basis remain left-aligned and Amount remains right-aligned. Pagination uses a named 18 mm footer-safe boundary and only preflights the actual next drawable content; Session details preflights its title with its first row, then each later row independently. The Canvas dependency is upgraded to a Go-1.25-compatible writer release that fixes malformed object/stream delimiters and emits TrueType as `/FontFile2`.

## Verification

Focused invoice package tests pass, including the representative `INV/26-27/000075` Remote-stop one-page regression, header face/bounds test, and long Unicode overflow test. Generated normal and overflow PDFs both pass `pdfcpu validate -mode strict`; `pdfcpu info` reports one normal page and three overflow pages. `go test -p 1 ./...`, `go vet -p 1 ./...`, `go build -p 1 ./...`, and `git diff --check` pass with `GOMAXPROCS=2` and `GOTOOLCHAIN=local` on Go 1.25.3. `TEST_DATABASE_URL` is unset; no database verification was run.

Chrome headless cannot capture its built-in PDF-viewer surface, so no visual-screenshot claim is made; the white label and metric-centering behaviour is covered directly by the renderer regression.

## Deployment record

Source revision `16edaff` was rehosted on 2026-09-14. The built binary SHA-256
is `a6200e596cc01d8f70cb113ab8518d93930364568652b58e72e6aa58fa35ce5c`; the
previous executable is retained at
`/root/evcmsnew-backups/pre-invoice-pdf-polish-16edaff-20260914T115128Z/evcmsnew`
(SHA-256
`993b2fb66fe47ec6365eb1d16b0769da66fc99fef48f5bdf2ceb59add231b4e5`).
Post-rehost service/process/install hashes matched, restart count was zero,
local and public health/readiness/docs/OpenAPI returned 200, all six required
current workers were healthy with fresh heartbeats, migration `000069` was
current, OpenAPI remained at 245 operations, Caddy validated, and no error or
panic entries were found for the new process. The database aggregates were 75
READY invoices and five already-SENT deliveries. No invoice artifact or email
was changed, sent, or retried.

The deployed build/test checks were rerun with Go 1.26.4: focused and full Go
tests, vet, module verification, OpenAPI parity, production build, and diff
check passed. Generated samples were recognized as PDF 1.7 (one and three
pages, mode `0600`). `pdfcpu` was unavailable for an independent strict
validation rerun; the strict checks above are the upstream work-item result.
No visual inspection is claimed. `TEST_DATABASE_URL`, `pwsh`, and SMTP
acceptance remain unavailable or unverified.
