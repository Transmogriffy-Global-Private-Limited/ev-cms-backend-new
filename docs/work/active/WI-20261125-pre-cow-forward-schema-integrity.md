# WI-20261125: Pre-CoW additive schema integrity repairs

**Status (2026-11-26):** Source checkpoint committed/pushed. Fresh disposable PostgreSQL 17 migration-specific acceptance passed; overall PG-backed Go suite remains failing (11 individual tests in last observed run). Not deployed; populated-database upgrades and physical HAL acceptance unverified.
**Source:** `anubhab-work` at `f4f96cbc5f8e0dc652970981fc3b0c60cdd0e31f` (2026-11-25), direct child of baseline `0f6e8b2648ef51c0c31f00eda9b394d6c5636e96`.
**Scope:** two forward-only migrations 000078/000079, additive DB-gated regression tests, read-only preflight. Does not create CoW tables or privileges.

## Causal defects and remedy

- `000035` composite FK `ON DELETE SET NULL` can attempt to null a mandatory tenant key. Set in-use tax profile deletion to RESTRICT; CPO already has an explicit UnassignGSTFromHub operation. Do not confuse removal of a tax profile from a Hub with deletion of legal history.
- `000050` Platform notification rows with NULL CPO bypass original 3-column uniqueness. Add partial unique index plus parent audience guard and target-specific composite FK; ensure CPO notices do not leak across recipients.
- `000028` (and adjacent `000061`, `000069`) have single-column cross-tenant child references. Add validated composite FKs for customer/connector/charger/tariff/wallet/holds/operations/HAL/invoices and matching Customer ownership. No update of existing rows.
- Historical `000056` may have lost DENY precedence in one-to-many override conversion; cannot infer lost intent, preserve independent provenance/review gate. Do not add arbitrary bans/revoke all legitimate ALLOW rows.
- `000055` earlier-version compatibility and `000064` customer EV vs fleet Vehicle are outside scope, by design. This is not blanket CoW completion.

## Historical preparation and remaining release gates

The checklist below was drafted before the actual fresh-database acceptance. Its execution was subsequently demonstrated on an isolated Windows PostgreSQL 17 cluster: all 77 migrations through 000079 applied, 18 targeted FKs validated, read-only pre/post checks, replay, guarded DOWN refusal and both dedicated PG integration tests passed. This does not establish safety on populated historical data, the whole Go suite or physical HAL operation.

## Before any populated-database upgrade

- Confirm no new migrations already exist and that no parallel owner claims these surfaces.
- Review on real Git checkout; ensure docs and consumers are synchronized with the new server-side invariants.
- Run count-only preflight on intended snapshot, then FULL disposable-DB fresh & existing-77 upgrade rehearsals; test negative FK cases and positive charging/invoice/notification workflows.
- Inspect additional wallet transaction/session ownership and diagnostic trace invariants before claiming full platform-wide tenant hardening.
- Confirm 000075-000077 runtime PostgreSQL evidence and HAL physical Stop as a separate pre-CoW gate; no accidental worker restart that runs migrations on a real target.
- Preserve migration row/data history; stage/commit/push only with distinct authorization.

## Done criteria

1. `000078` and `000079` up scripts execute and validate against fresh and realistic populated PostgreSQL, with no original data modification; deliberate down refusal tested.
2. New DB-gated regression tests pass, then broad repo tests, vet, build and contracts/docs checks pass.
3. Historical permission provenance/`chargers.operations` review and additional financial/trace boundary review either closed with evidence or explicitly deferred as owner blockers.
4. Source/work-item/Project State/AI Changelog/SCHEMA/FE handoffs and live `schema_migrations` match what is claimed.

**Achieved:** migration-specific execution on fresh disposable PostgreSQL 17, with replay, read-only checks, guarded DOWN and the two dedicated integration tests.

**Still open:** realistic populated-database upgrade and data reconciliation, historical `000056` DENY provenance, full PG-backed Go suite (last observed 11 test failures), CoW authorization, deployment and physical charger acceptance. The isolated acceptance evidence does not imply live-schema changes.
