# WI-20260915-invoice-tariff-basis

Status: Complete
Owner: Codex

## Outcome and scope

Correct the current invoice Basis column using frozen tariff billing semantics
and actual usage. Energy bills kWh, time bills elapsed minutes, sessions bills
once. AUTO/ENERGY/TIME/MONEY select independent stop limits, not billing units.
Scope: invoice projection, V3 rendering, tests and invoice documentation.

## Invariants and verification

Preserve amounts/taxes, stop-limit details, one-page layout, legacy V2 dispatch,
and READY artifacts. No billing, HAL, schema/API or live changes. Verified all twelve
tariff/limit combinations, fractional duration, session nil units, historical
per-kWh/per-Wh snapshots, missing/invalid facts, and actual generated PDFs.

Focused/full Go tests, vet, docs verifier and diff checks pass. Twelve PDFs parse
as one page with correct basis and independent selected-limit text; representative
time/session PDFs visually inspected. Matrix fixtures use valid admission limits
that differ from actual usage, including a MONEY ceiling above fixed-session cost.
`TEST_DATABASE_URL` is unset; database lifecycle/live generation remain unverified.
Branch publication is authorized; this slice is not deployed.
