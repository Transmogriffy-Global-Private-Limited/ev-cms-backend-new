# WI-20260916-charger-operation-recovery

Status: Complete (broad-suite failures documented)
Owner: Codex

## Scope

CMS durable charger-operation dispatch only. Shared handler/recovery dispatcher,
immutable dispatch inputs, fenced pre-delivery leases, committed delivery marker,
exact-ID reconciliation, bounded lifecycle worker, additive migration70,
operation-state contracts and crash-boundary tests. No HAL modifications,
authentication, invoice, financial or customer behavior, and no HAL code change.

Overlaps the CMS reliability surface of WI-20260904-cpo-charger-operations;
its paired-HAL/hardware acceptance remains separate. Existing legacy PERSISTED
rows must be treated as delivery-uncertain, never blindly replayed.

## Verification

See the final verification and deployment record below. Commit/push authorized.

## Implementation and safety review

| Required invariant | Evidence |
|---|---|
| PERSISTED means unattempted | Only admission/rejected pre-delivery snapshot produces it; migration quarantines legacy rows |
| CLAIMED means unattempted | PostgreSQL lease and UUID claim token; no HAL call in claim path |
| Attempt commits before HAL | Shared dispatcher calls mark transaction, checks commit success, then calls HAL; deferred-commit-failure test proves no send |
| Attempt is never replayed | Claim predicate excludes attempted rows; database guard prevents requeue; transport body rewind/redirect disabled |
| One dispatcher | Request and worker both invoke `dispatchChargerOperation` |
| Pre-delivery crash recovers | Tests A/B/I/H/J: restart, expired leases, stale-owner fencing, concurrent idempotency |
| Possible delivery reconciles | Tests C/D/E/F/G: exact CMS-ID GET, result/absence, no second POST |
| Inputs cannot drift | Frozen mapping identity/connector number plus existing operation fields; immutable DB guard and changed-mapping test |
| Concurrent workers cannot duplicate | Transactional row claims, attempt-token fence, shared PostgreSQL lease clock and race tests |
| Scope preserved | No HAL repo, auth, invoice, customer or financial changes; migration and state contracts are additive |

HAL raw PERSISTED/DELIVERY_ATTEMPTED map to CMS HAL_ACCEPTED. Nonterminal
completion timestamps are cleared. Reconciliation uses a UUID reservation to
fence stale results; scheduling uses the database clock across instances.
Migration 70 has a safe rollback refusal while operation history exists.

The first transport-error test detected Go retrying a rewindable idempotent
POST. Disabling rewind and redirects at the dedicated operation adapter fixed
that actual duplicate external attempt; the regression test retains coverage.

## Remote-main integration

The user explicitly requested incorporating advanced remote main. Fetched
`a3ce2ca` (hub-unassignment endpoint and charger-deletion cleanup), fast-forwarded
the working branch, then restored this task patch with a clean OpenAPI merge.
Those remote implementations remain intact. The docs verifier's operation count
was updated from 245 to 246 for the newly merged endpoint.

## Verification progress

Focused disposable-PostgreSQL crash-boundary, concurrency, immutable-envelope,
legacy-migration and rollback tests passed. Dedicated HAL transport no-replay
and redirect tests passed. Poison-claim isolation, in-flight cancellation,
history-state parsing and OpenAPI state tests passed. Final merged-tree focused
checks and one broad test/vet/build pass are recorded below. The original
implementation checks used only disposable PostgreSQL and a loopback HAL stub;
the later live CMS deployment is recorded under Publication below.

## Final verification

- PASS: focused charger-operation recovery/history, HAL transport, migration and
  OpenAPI tests against a dedicated disposable PostgreSQL 17 database and
  loopback HTTP stub. Combined-tree focused tests passed after incorporating
  remote main, including database-clock lease fencing.
- PASS: `go vet -p 1 ./...` and `go build -p 1 ./...`, one broad pass each.
- PASS: documentation verifier with 246 operations, and whitespace review.
- Broad `go test -p 1 ./...` ran once with PostgreSQL enabled. It is not a clean
  pass; exact failures are retained below. This task's
  recovery behavior is independently covered by the focused passing tests.
- The original focused recovery suite used disposable PostgreSQL and HAL stubs;
  no physical charger/OCPP effect was exercised. The live CMS rollout later
  performed read-only exact-ID reconciliation only. A pre-change baseline suite
  was not rerun, so broad failures are identified by observed output and scope,
  not presented as independently baseline-reproduced.

### Exact broad-suite failures

- `TestMailOutboxTemplateConstraintAcceptsApplicationCatalogueWithPostgreSQL`: migration_integration_test.go:36: clean up mail-outbox catalogue rows: sql: database is closed
- `TestCPOAdministrativeAppIDLoginWithPostgreSQL`: login_appid_integration_test.go:107: OTP status=401 want=200
- `TestChargingSessionListRepositoryWithPostgreSQL`: charging_session_list_integration_test.go:145: duration asc traversed 2 rows, reference 5
- `TestCPOCapabilityRoutesReachServicesWithPostgreSQL`: integration_test.go:251: create CPO refresh token: ERROR: new row for relation "auth_refresh_tokens" violates check constraint "chk_auth_refresh_token_hash" (SQLSTATE 23514)
- `TestCPOProvisioningAndFirstAdminLifecycleWithPostgreSQL`: integration_test.go:758: load CPO_MEMBERSHIP_ASSIGNED mail job: record not found
- `TestCPOSuperadminDependencyLifecycleWithPostgreSQL`: integration_test.go:977: load correlated onboarding mail: record not found
- `TestCPOAdminProfileAndNetworkConfigurationWithPostgreSQL`: integration_test.go:1606: delete referenced charger got <nil>, want charger_in_use
- `TestAssignChargerToHubTenantScope`: integration_test.go:1867: create hub: invalid_state
- `TestHubTariffLifecycleWithPostgreSQL`: integration_test.go:2058: create hub: invalid_state
- `TestHubTariffCreationRejectsInvalidEnums`: integration_test.go:2364: create hub: invalid_state
- `TestCustomerChargeabilityProjectionWithPostgreSQL`: chargeability_integration_test.go:37: create hub: ERROR: customer-visible hub requires one enabled unbounded hub tariff (SQLSTATE 23514)
- `TestChargingStartAdmissionWithPostgreSQL`: charging_admission_integration_test.go:64: create hub: ERROR: customer-visible hub requires one enabled unbounded hub tariff (SQLSTATE 23514)
- `TestUserAppTariffTargetPrecedenceWithPostgreSQL`: charging_admission_integration_test.go:213: create hub: ERROR: customer-visible hub requires one enabled unbounded hub tariff (SQLSTATE 23514)
- `TestCustomerPriceRejectsInvalidPersistedHubGSTWithPostgreSQL`: charging_admission_integration_test.go:276: create hub: ERROR: customer-visible hub requires one enabled unbounded hub tariff (SQLSTATE 23514)
- `TestConnectionFactSequenceRejectsStaleObservationWithPostgreSQL`: charging_facts_integration_test.go:29: create hub: ERROR: customer-visible hub requires one enabled unbounded hub tariff (SQLSTATE 23514)
- `TestChargingSessionConnectorOccupancyWithPostgreSQL`: charging_occupancy_integration_test.go:32: create hub: ERROR: customer-visible hub requires one enabled unbounded hub tariff (SQLSTATE 23514)
- `TestChargingStartReconciliationWithPostgreSQL`: charging_reconciliation_integration_test.go:94: create hub: ERROR: customer-visible hub requires one enabled unbounded hub tariff (SQLSTATE 23514)
- `TestMaterializedSessionCompletionReconciliationWithPostgreSQL`: charging_reconciliation_integration_test.go:412: create hub: ERROR: customer-visible hub requires one enabled unbounded hub tariff (SQLSTATE 23514)
- `TestMaterializedSessionReconciliationCursorWithPostgreSQL`: charging_reconciliation_integration_test.go:541: create hub: ERROR: customer-visible hub requires one enabled unbounded hub tariff (SQLSTATE 23514)
- `TestCustomerSignupLifecycleWithPostgreSQL`: integration_test.go:241: created password mismatch: matches=false err=<nil>
- `TestListAdministratorsWithPostgreSQL`: integration_test.go:89: next active page returned two administrators, expected only the lower same-timestamp administrator (synthetic identity dump omitted).
- `TestSupportWorkflowWithPostgreSQL`: integration_test.go:76: ticket creation did not enqueue exactly the eligible CPO confirmations

The failures are in unchanged test/behavior surfaces outside this recovery slice.
The charger-deletion expectation reflects the incorporated remote-main change;
it was preserved rather than rewritten here. The auth OTP failure is recorded
without assuming its cause. No broad-suite rerun or unrelated fix was made.
All new recovery tests passed in the focused combined-tree run; the full run
reported no failures in those tests. Logs remain in Git administrative task
context. The disposable PostgreSQL cluster was stopped after verification.

## Publication

Authorized publication targets are `anubhab-work` and `main`, preserving remote
main ancestry with no force push. Source revision `8cd65ae` was rehosted on the
development VPS with migration 70 after a verified custom-format dump. The old
binary and dump are under
`/root/evcmsnew-backups/pre-000070-20260916T094521Z/`. The down migration refuses
while operation history exists; use a forward correction, not the old binary or
deletion of records. Live CMS/HAL exact-ID lookup results and route/worker
verification are recorded in `docs/PROJECT_STATE.md` and the hosting guide.
Physical-charger acceptance remains unperformed.
