# Work Item: Charging-session invoice PDF UI/UX

## Delivered

- Rebuilt only the pure-Go Canvas invoice composition: CPO/amount header, bounded contain-fit frozen logo, customer/location cards, horizontal charging summary, truthful charges table, quiet supplier note, demoted session details, and issuer/page-count footer.
- Retained immutable snapshot data, frozen logo assets, Unicode shaping, lifecycle behavior, numbering, hydration, delivery, HAL, and migrations. New invoices use `canvas-noto-v2`; the previous Canvas `canvas-noto-v1` layout remains dispatchable for rows stamped before the redesign.
- Charge presentation never reverse-engineers tax or pre-tax rupees. It exposes frozen energy/rate/tax-rate facts and keeps the frozen final amount authoritative.

## Verification

- Focused invoice renderer tests passed, including one-page/multi-page, Unicode, long fields, absent optional fields, CGST/SGST/IGST, location de-duplication, logo bounds, and sample generation.
- The generated sample passed `pdfcpu validate -mode strict`.
- Final checks passed: `go test -p 1 ./...`; `go vet -p 1 ./...`; `go build -p 1 ./...`; `git diff --check`.
- Installed Edge headless opened the PDF but cannot rasterize its internal PDF viewer in this environment, yielding a blank capture. This is a visual-inspection tooling limitation; no false visual-pass claim is made.
- `TEST_DATABASE_URL` was unset; no database lifecycle, HAL, or migration test was run. The generated sample is PDF 1.7, one page, mode `0600`. `pdfcpu` is unavailable in the deployment shell; strict validation was recorded upstream. Visual inspection did not yield a renderable capture.

## Deployment verification

- Rehosted source revision `5116558` (redesign `b2bccc1` plus the
  renderer-version compatibility fix). Active binary SHA-256:
  `993b2fb66fe47ec6365eb1d16b0769da66fc99fef48f5bdf2ceb59add231b4e5`.
  Prior executable retained at
  `/root/evcmsnew-backups/pre-invoice-pdf-uiux-b2bccc1-20260914T105841Z/evcmsnew`
  (SHA-256
  `cfc621dd5e61a48af09bba0e8f70b9cb5de05b0e5b7ffa0f7cb6aa27cf23d8a8`). No
  migration or API change; `000069` remains current.
- All 74 invoice rows were READY and four deliveries already SENT at
  verification; this rehost changed no existing artifact and sent/retried no
  email. Process/install hashes match, restart count is zero, local/HTTPS
  health/readiness/docs pass, required workers are healthy, Caddy validates,
  and no new service errors were found.
- `TEST_DATABASE_URL`, `pwsh`, and local `pdfcpu` are unavailable. SMTP
  acceptance and a visual pass are not claimed.
