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
- `TEST_DATABASE_URL` was unset, so no database-backed invoice-lifecycle test was run. No HAL, migration, deployment, or existing invoice mutation occurred.
