# Work Item: Future invoice hydration and CPO issuer identity

## Scope

Completed the future charging-session invoice issuance boundary only: authoritative entity hydration and consistency validation, CPO supplier/email identity, and safe legacy CPO invoice-logo import/snapshotting. Historical issued invoices remain immutable and out of scope.

## Delivered

- Issuance loads Customer, Connector, Charger, optional Hub, StartIntent, Payment, CPO, and Settings by their authoritative session/CPO identifiers instead of relying on incidental GORM nested population.
- Required tenant and relation consistency is validated before a number is allocated. A hubless charger remains supported; a non-null HubID must resolve to the owning CPO Hub.
- Supplier snapshot and visible invoice-email sender/subject use the owning CPO business identity; the authenticated SMTP address is not changed.
- Blank logo settings remain allowed. Nonblank configured logos now fail issuance distinctly when invalid, missing, unsafe, oversized, or malformed. Valid UUID-named historical `uploads/` logos decode, import content-addressably into the controlled `uploads/invoice-logos` store, update the CPO setting, and freeze by immutable asset hash.

## Verification

- Focused: `go test -p 1 ./src/invoice ./src/mail -run 'Test(InvoiceIssuance|SnapshotLogo|InvoiceIssuer|InvoiceFromName|LogoAsset|BuildSnapshot|CustomerInvoice|RenderPDF|StoreValidated)' -count=1`
- Final: `go test -p 1 ./...`; `go vet -p 1 ./...`; `go build -p 1 ./...`; `git diff --check`
- All listed checks passed with `GOMAXPROCS=2` and `GOTOOLCHAIN=local`.
- `TEST_DATABASE_URL` was unset, so no database-backed invoice-lifecycle test was run. No HAL or migration change occurred.

## Deployment verification

- Rehosted source revision `756e339`; binary SHA-256
  `cfc621dd5e61a48af09bba0e8f70b9cb5de05b0e5b7ffa0f7cb6aa27cf23d8a8`.
  Prior executable retained at
  `/root/evcmsnew-backups/pre-invoice-hydration-issuer-756e339-20260914T091516Z/evcmsnew`
  (SHA-256
  `d4f08a0178d9460cacd951e461ac467e5f432395005be2d358e314084919b33c`). No
  migration was needed; `000069` remains current.
- Service/process hash, readiness, workers, local and HTTPS routes, Caddy, and
  post-restart error count passed. Database aggregates showed 72 READY invoices
  and two already-SENT deliveries; no invoice artifact or email was changed by
  this rehost.
- `pwsh` and `TEST_DATABASE_URL` are unavailable. SMTP attachment acceptance
  remains untested.
