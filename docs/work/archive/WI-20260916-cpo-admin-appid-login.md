# WI-20260916-cpo-admin-appid-login

Status: Complete (broad-suite limitations recorded)
Owner: Codex

## Outcome and scope

Initial CPO administrative login selects its context through `X-CPO-App-ID`,
never a body CPO UUID. Password authentication and active membership in the
resolved active CPO grant authority. The OTP challenge stores the internal CPO ID;
verify/resend/refresh retain that binding and fresh request permissions remain.

Claimed surfaces: administrative auth, its CPO integration-test callers, OpenAPI,
administrative/CPO frontend documentation and project records. No customer auth,
invoice, charging, HAL, commercial/payment, deployment or migration changes.

## Verification and publication

Test missing/malformed/contradictory headers, rejected UUID input, multiple CPO
memberships, generic failures, infrastructure errors, challenge binding,
deactivation, resend/verify/refresh isolation, platform regression and fresh
permissions. Use an isolated local PostgreSQL cluster, never application data.
Focused tests precede one broad test/vet/build pass and documentation checks.
Commit/push authorized by the user after verification; deployment excluded.

## Authority audit

- Initial login no longer accepts a client CPO UUID. The unique `cpos.app_id`
  lookup selects context, then the existing exact user/CPO active-membership
  resolver grants authority. No first-membership selection or new constraint.
- Verify/resend use `AuthChallenge.CPOID`; refresh uses session CPOID.
  Protected routes use the validated principal and fresh access evaluator.
- Platform CPO resource IDs and onboarding URL placeholders remain resource or
  navigation metadata, not caller authority. Frontends configure the App ID;
  an onboarding CPO UUID must not be submitted to login.
- Existing stale auth-test wording treating OWNER as dormant was corrected:
  role changes invalidate stale tokens, while a fresh active OWNER may log in.

## Verification results

- PASS: focused `go test -p 1 ./src/auth -count=1` using a newly initialized,
  disposable PostgreSQL 17 cluster on loopback. Includes header/body validation,
  exact tenant binding, multi-membership selection, generic credential failures,
  infrastructure failure, OTP deactivation, resend/verify/refresh isolation,
  platform regression and fresh permission/role/membership evaluation.
- PASS: focused OpenAPI runtime-route/UI and administrative login schema tests.
- PASS: final documentation verifier and `git diff --check`.
- PASS: `go vet -p 1 ./...` and `go build -p 1 ./...` (one broad pass each).
- Focused CPO provisioning reached the migrated login/OTP/refresh calls, then
  failed looking for the old `CPO_MEMBERSHIP_ASSIGNED` template; unchanged
  production code queues `CPO_STAFF_EXISTING_IDENTITY`.
- Broad `go test -p 1 ./...` ran once with the disposable database enabled.
  It is not a clean pass. Failures outside this login change are listed below;
  no customer, charging, mail or migration behavior was changed to suppress them.

### Database-suite failures outside the login implementation

- `TestMailOutboxTemplateConstraintAcceptsApplicationCatalogueWithPostgreSQL`: migration_integration_test.go:36: clean up mail-outbox catalogue rows: sql: database is closed
- `TestChargingSessionListRepositoryWithPostgreSQL`: charging_session_list_integration_test.go:145: duration asc traversed 2 rows, reference 5
- `TestCPOCapabilityRoutesReachServicesWithPostgreSQL`: integration_test.go:251: create CPO refresh token: ERROR: new row for relation "auth_refresh_tokens" violates check constraint "chk_auth_refresh_token_hash" (SQLSTATE 23514)
- `TestCPOProvisioningAndFirstAdminLifecycleWithPostgreSQL`: integration_test.go:758: load CPO_MEMBERSHIP_ASSIGNED mail job: record not found
- `TestCPOSuperadminDependencyLifecycleWithPostgreSQL`: integration_test.go:977: load correlated onboarding mail: record not found
- `TestCPOAdminProfileAndNetworkConfigurationWithPostgreSQL`: integration_test.go:1613: dormant OWNER role was allowed to call a CPO operation
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
- `TestListAdministratorsWithPostgreSQL`: integration_test.go:89: next active page = {Administrators:[{UserID:246028f5-5428-47e1-b7e5-96464fd1b199 Email:low-f0f2079f-c913-48b8-adab-f75e51efad45@example.com FullName:Low IdentityActive:true IdentityVerified:true AuthorityActive:true StatusReason:test authority StatusChangedAt:2200-08-11 17:30:00 +0530 IST StatusChangedByID:<nil> CreatedAt:2200-08-11 17:30:00 +0530 IST UpdatedAt:2200-08-11 17:30:00 +0530 IST} {UserID:b377c858-6af2-4bdf-a473-dc0fcf661e4d Email:inactive-f0f2079f-c913-48b8-adab-f75e51efad45@example.com FullName:Inactive IdentityActive:true IdentityVerified:false AuthorityActive:true StatusReason:test authority StatusChangedAt:2200-08-11 16:30:00 +0530 IST StatusChangedByID:<nil> CreatedAt:2200-08-11 16:30:00 +0530 IST UpdatedAt:2200-08-11 16:30:00 +0530 IST}] NextBefore:2200-08-11 16:30:00 +0530 IST NextBeforeID:b377c858-6af2-4bdf-a473-dc0fcf661e4d HasMore:true}, want only lower same-timestamp administrator
- `TestSupportWorkflowWithPostgreSQL`: integration_test.go:76: ticket creation did not enqueue exactly the eligible CPO confirmations

These are observed failures in unchanged behaviors/fixtures; a separate clean
baseline database run was not performed. The full log is retained in the local
Git administrative task context. Real SMTP delivery and deployment were not
performed. The disposable test cluster is stopped after verification.

Implementation status: complete for the requested login slice, with the broad
suite limitations above retained explicitly. Publication uses the authorized
`anubhab-work` and `main` branches, without force-push.
