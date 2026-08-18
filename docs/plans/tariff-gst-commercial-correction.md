# Tariff and GST Commercial Correction

Status: Implemented and Verified (database lifecycle coverage pending)

## Decision

Tariff and Hub GST remain independent. A tariff has exactly one Hub, Charger,
or UserGroup target and resolves by `USERGROUP > CHARGER > HUB`. GST always
resolves from the selected charger's Hub, never from the tariff.

## Corrected tariff contract

`price_per_unit` is a fixed price with one of three valid combinations:

- `fixed` + `energy` + `kwh`: price per kWh; meter Wh is divided by 1000 using
  exact decimal arithmetic.
- `fixed` + `time` + `minutes`: price per actual durable session minute.
- `fixed` + `sessions` + no unit: one price per completed session.

The existing session-duration command cutoff remains operational protection and
does not define the tariff time basis.

## Implementation slices

1. Replace persisted `watt/hour` with `kwh` in a forward migration while
   retaining numeric tariff amounts, then enforce the valid combinations in
   one validator.
2. Use the shared pricing interpretation for customer price, wallet hold, new
   snapshots, and settlement. Keep dedicated readers for historical
   `price_per_kwh` snapshots and the released migration-40
   `price_per_unit`/`watt/hour` snapshots.
3. Centralize Hub/GST compatibility, validate the fully resulting relation for
   GST and Hub mutations under row locks, and defend the customer resolver
   against corrupted persisted state.
4. Disallow non-zero idle fees because the CMS has no durable authoritative
   idle interval. Preserve zero as valid and do not derive idle from session
   duration.

## Acceptance and verification

- New energy prices such as INR 16.91 per kWh retain `16.91` and settle
  7200 Wh as 7.2 kWh times that rate.
- Admission and settlement share base and GST calculations for all bases.
- Active/historical sessions use their own snapshots after later tariff/GST
  edits.
- No normal Hub/GST mutation can persist a relationship invalid for the Hub
  state; runtime customer pricing rejects a corrupt relationship.
- Run focused CPO/customer/database tests, docs/OpenAPI route checks, full Go
  tests and vet, and report skipped disposable-PostgreSQL lifecycle coverage.
