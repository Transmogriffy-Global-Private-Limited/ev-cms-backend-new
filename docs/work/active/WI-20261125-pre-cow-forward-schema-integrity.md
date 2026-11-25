# WI-20261125: Pre-CoW additive schema integrity repairs

**Status:** Prepared source candidate only. No checkout, branch, DB or deployment changes. Not verified in PostgreSQL.
**Baseline:** `anubhab-work` at `0f6e8b2648ef51c0c31f00eda9b394d6c5636e96` (2026-11-25).
**Scope:** two forward-only migrations 000078/000079, additive DB-gated regression tests, read-only preflight. Does not create CoW tables or privileges.

## Causal defects and remedy

- `000035` composite FK `ON DELETE SET NULL` can attempt to null a mandatory tenant key. Set in-use tax profile deletion to RESTRICT; CPO already has an explicit UnassignGSTFromHub operation. Do not confuse removal of a tax profile from a Hub with deletion of legal history.
- `000050` Platform notification rows with NULL CPO bypass original 3-column uniqueness. Add partial unique index plus parent audience guard and target-specific composite FK; ensure CPO notices do not leak across recipients.
- `000028` (and adjacent `000061`, `000069`) have single-column cross-tenant child references. Add validated composite FKs for customer/connector/charger/tariff/wallet/holds/operations/HAL/invoices and matching Customer ownership. No update of existing rows.
- Historical `000056` may have lost DENY precedence in one-to-many override conversion; cannot infer lost intent, preserve independent provenance/review gate. Do not add arbitrary bans/revoke all legitimate ALLOW rows.
- `000055` earlier-version compatibility and `000064` customer EV vs fleet Vehicle are outside scope, by design. This is not blanket CoW completion.

## Before acceptance

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

**Not achieved by this package:** PostgreSQL execution, data reconciliation, full release, CoW authorization, historical DENY restoration, deploy or physical charger acceptance.
