# WI-20261127 CoW Run 2B: existing customer EV compatibility

**Status:** Locally source-accepted and focused PG17-accepted on 2026-11-27; Git release and VPS2 deployment separately pending. Baseline `4ba588c8324bddc3a2f06405eae0eb075d403422`; deployed source currently `2a28228fa04aee365b5a93e7c812419102f84d95`.

## Goal and bounded ownership

Extend the **existing** CMS `vehicles` persisted master, its five customer endpoints, and Run 2A copied-value `BuildCowServiceContext` snapshot. No parallel vehicle registry. New metadata is customer-declared, not verified by HAL or BMS. Null/omitted is unknown; no assumption from name/model. Separate service vehicles and mobile chargers remain different asset domains.

## Invariants

- CPO/customer identity always from session; composite FK and owner predicates remain.
- Previous HTTP fields and list order/filter/cursors untouched, legacy PATCH without expected_version still works.
- No change to hard-delete until historic ServiceJobs are actually present; future job snapshots are independent.
- All six fields nullable, battery/power positive, mode/power contradictions rejected; `connector_types` normalized, unique and bounded, but not claimed as a verified hardware match.
- PATCH field presence means absent retains, null clears. Version increments only on actual change; optional expected_version fences stale editors; updates use row lock and transaction/audit.
- Temporary EV and saved EV share validator, snapshot copies are immutable values, not pointers into saved state.
- Migration 82 is additive, no fabricated backfill, DOWN refuses silent data loss.

## Claimed files

Existing: `src/models/vehicle.go`, `src/customerauth/vehicles.go`, `src/customerauth/cow_service_context.go`, canonical OpenAPI and five documentation files. New: `src/customerauth/vehicle_compatibility.go`, two tests, migration 82 up/down, this work item, standalone customer vehicle contract. See installer exact manifest, 2026-11-27.

## Verification, recovery and release gates

1. Offline installer checks exact base Git HEAD+branch, cleanliness, original blob identity and all patch anchors before writing; backups outside repo; no `git` mutators, DB or network.
2. Run gofmt, unit/route/API tests, full test/vet/build, docs verifier, diff and dependency review; no skipped check counted as pass.
3. Disposable PG17 (never VPS2) fresh migration 1-82, replay, legacy CRUD, column nulls, direct invalid SQL, ownership and CAS, snapshot deep-copy, concurrency/retry. Keep full evidence.
4. Explicit human review of candidate, normal commit/push only after approval, then independently planned live additive migration with backups and operational acceptance, if separately authorized.
5. Unverified obligations: authenticated run1/2A E2E, SMTP, HAL/physical charging, final stopped-state backup off-VPS, worker stress. These are not closed by this source patch.

## 2026-11-27 owner-executed acceptance (pre-publication)

- Source: `RUN2B_VERIFY=PASS changed_paths=16` and `RUN2B_SOURCE_GATES=PASS` after Go 1.25.3 tests/vet/build, documentation and Gin/OpenAPI route parity; `git diff --check` passed.
- Private PostgreSQL 17: all 80 up migrations through `000082`, replay, four required focused tests, six named vehicle checks, ownership FK, legacy unknown/null persistence, SQL rejection cases and guarded DOWN refusal passed; runner printed `COW_RUN2B_PG17_ACCEPTANCE=PASS; private cluster removed=True`.
- User-supplied PowerShell transcripts support these statuses; detailed local `SUMMARY.json` and step logs are not independently inspected, and the private fresh DB is **not** a populated PostgreSQL 19 upgrade simulation.
- Historical authenticated customer E2E, concurrent two-writer PATCH stress, SMTP/HAL/physical charging, final stopped-state backup off-VPS, worker stress and real VPS2 release remain open.
- Source-ready item archived as required by repository workflow; archive is **not** a claim of VPS2 deployment.
