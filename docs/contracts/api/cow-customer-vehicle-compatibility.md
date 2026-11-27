# CoW Run 2B - Existing Customer Vehicle Compatibility Contract

**Status:** Windows source and focused disposable PG17 acceptance passed on 2026-11-27; not yet VPS2 migrated or deployed. Git publication separately controlled.

## Ownership and API inventory

All five pre-existing routes remain at `/api/v1/app/vehicles` (GET list, POST create, GET/PATCH/DELETE by ID). They continue to require the customer bearer and validated `X-CPO-App-ID`. Ownership is derived only from authenticated principal CPO/customer. Foreign and missing resources both return `404 vehicle_not_found`. There are **zero new public operations**. The existing hard-delete behavior remains; no persistent historical ServiceJob yet exists to justify changing deletion semantics during this run.

## Additional optional request and response fields

| JSON field | Type | Meaning |
| --- | --- | --- |
| `battery_capacity_wh` | nullable positive int64 | Customer-declared nominal battery capacity in watt-hours. Not measured live energy or state of charge. |
| `connector_types` | nullable array of 1-8 distinct strings (1-40 chars) | Customer-declared compatible connector labels, case folded to uppercase and trimmed. Example `CCS2`. Labels are **not verified** against HAL hardware. |
| `supports_ac` | nullable boolean | Whether the vehicle is declared capable of AC charging. |
| `supports_dc` | nullable boolean | Whether the vehicle is declared capable of DC charging. |
| `max_ac_power_w` | nullable positive int64 | Customer-declared AC intake power ceiling in watts. |
| `max_dc_power_w` | nullable positive int64 | Customer-declared DC intake power ceiling in watts. |
| `version` | positive int64, response only | Starts at 1 and increases for a real update. Legacy records get 1; same-content PATCH does not increment. |
| `expected_version` | optional positive int64, PATCH input only | Compare-and-swap guard: stale version gives `409 vehicle_version_conflict`; omission retains legacy PATCH semantics. Explicit null or nonpositive is `400 invalid_vehicle_version`. |

The original `vehicle_number` (required, 1-50 chars), `vehicle_type`, `vehicle_make`, `vehicle_model`, `last_charged`, `date_added`, `created_at`, `updated_at`, list paging/filtering and customer authority remain unchanged. All new compatibility fields are individually **optional**; omit or supply null for unknown. `false` is an explicit declaration of non-support, not the same as unknown. `0` is invalid for positive Wh/W values, never coerced into unknown. `connector_types: []` is invalid; null/omit means unknown. The backend normalizes connector labels syntactically, without assuming a finite future connector catalog or claiming physical certification. No max power value can coexist with `supports_ac=false` or `supports_dc=false` for its respective mode; the other mode is independent. This is internal consistency validation, not a charger selection algorithm.

### Examples

Legacy create still works:

```json
{"vehicle_number":"WB02AB1234","vehicle_make":"Tata"}
```

Optional declared detail:

```json
{"vehicle_number":"WB02AB1234","battery_capacity_wh":50000,"connector_types":["CCS2","TYPE2"],"supports_ac":true,"supports_dc":true,"max_ac_power_w":11000,"max_dc_power_w":60000}
```

Versioned update:

```json
{"expected_version":3,"max_dc_power_w":null,"connector_types":["CCS2"]}
```

## Data and history

Forward migration `000082` adds only nullable columns to existing `vehicles`, plus `version bigint not null default 1` and checks on positivity and mode/connector structure. Existing records are **not backfilled with inferred values**, and the `(cpo_id,customer_id)` durable FK from migration 66 remains. Existing customer audit rows are emitted only for real mutations, listing names of changed fields rather than values. No new table, vehicle registry, or CoW entitlement requirement is added to the existing legacy vehicle API.

`BuildCowServiceContext` continues to enforce CoW availability, customer ownership, and exactly one saved or temporary vehicle and address. Both vehicle forms use the same compatibility validator; the returned `CowVehicleSnapshot` contains independent deep copies of all fields, `source: SAVED|TEMPORARY`, and `source_version` for a saved record. This builder is **not** a historical persistence guarantee on its own. The future ServiceJob transaction must durably freeze the snapshot and must not reconstruct it from an editable master row. A future promotion operation must not rewrite that frozen snapshot.

## Explicit non-goals and safety

No actual connector match, charger ability, HAL evidence, booking/dispatch, ServiceJob, payment, SOC or charging power calculation is implemented here. On future dispatch, physically compatible charger inventory, fresh HAL evidence and customer declarations must be reconciled by an explicit compatibility engine; unknowns do not prove compatibility. When a safety-critical fact remains unknown, defer eligibility rather than inventing a match. Raw customer input cannot select a CPO, customer, charger HAL operation or charging transaction.

## Acceptance boundary

Source review alone is not acceptance. Required: `gofmt`, focused unit and route/OpenAPI parity, `go test -p 1 -count=1 ./...`, `go vet -p 1 ./...`, `go build -p 1 ./...`, documentation verifier (PowerShell), `git diff --check`, and replay of migrations plus focused lifecycle, tenant isolation, direct SQL constraint, stale-update and snapshot tests in a **disposable** PostgreSQL database. Existing VPS2 release is independent and must not be restarted/migrated as part of source inspection. Publication, live migration and physical charger acceptance each require separate authorization/evidence.

## Release evidence boundary (27 November 2026)

The owner-provided terminal output verifies source formatting/full Go tests/vet/build/docs/route-OpenAPI parity and fresh private PG17 migration-through-82/focused tests/down-guard/cleanup. The complete local per-step logs have not been directly reviewed here. This is not authenticated customer HTTP E2E, populated PostgreSQL 19 migration acceptance, hardware/charger verification, or evidence of live VPS2 deployment.
