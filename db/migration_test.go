package db

import (
	"slices"
	"strings"
	"testing"
)

func TestEmbeddedMigrationsArePresentAndOrdered(t *testing.T) {
	t.Parallel()

	names, err := embeddedMigrationNames()
	if err != nil {
		t.Fatalf("discover embedded migrations: %v", err)
	}
	if len(names) == 0 {
		t.Fatal("expected at least one embedded migration")
	}
	if !slices.IsSorted(names) {
		t.Fatalf("migrations are not ordered: %v", names)
	}

	for _, name := range names {
		body, err := migrationFiles.ReadFile("migrations/" + name)
		if err != nil {
			t.Fatalf("read migration %q: %v", name, err)
		}
		if len(body) == 0 {
			t.Fatalf("migration %q is empty", name)
		}

		downName, err := matchingDownMigration(name)
		if err != nil {
			t.Fatalf("find down migration for %q: %v", name, err)
		}
		downBody, err := migrationFiles.ReadFile("migrations/" + downName)
		if err != nil {
			t.Fatalf("read down migration %q: %v", downName, err)
		}
		if len(downBody) == 0 {
			t.Fatalf("down migration %q is empty", downName)
		}
	}
}

func TestMatchingDownMigrationRejectsInvalidVersion(t *testing.T) {
	t.Parallel()

	if _, err := matchingDownMigration("000001_cms_schema.sql"); err == nil {
		t.Fatal("expected invalid up migration name to fail")
	}
}

func TestCommercialTaxAndChargerHubPrerequisiteMigrationIsGuarded(t *testing.T) {
	t.Parallel()

	upBody, err := migrationFiles.ReadFile("migrations/000038_separate_tariff_gst_and_require_charger_hub.up.sql")
	if err != nil {
		t.Fatalf("read commercial-tax migration: %v", err)
	}
	downBody, err := migrationFiles.ReadFile("migrations/000038_separate_tariff_gst_and_require_charger_hub.down.sql")
	if err != nil {
		t.Fatalf("read commercial-tax rollback migration: %v", err)
	}
	upSQL, downSQL := string(upBody), string(downBody)
	for _, required := range []string{
		"WHERE gst_id IS NOT NULL",
		"DROP COLUMN gst_id",
		"chargers_customer_visibility_requires_hub",
		"chargers_active_requires_hub",
		"ALTER COLUMN customer_visibility SET DEFAULT FALSE",
	} {
		if !strings.Contains(upSQL, required) {
			t.Errorf("migration is missing %q", required)
		}
	}
	if !strings.Contains(downSQL, "ADD COLUMN gst_id uuid") || !strings.Contains(downSQL, "fk_tariffs_gst") {
		t.Error("rollback does not restore the nullable legacy tariff GST shape")
	}
}

func TestHubSanctionLoadMigrationPreservesNonNegativeInvariant(t *testing.T) {
	t.Parallel()

	upBody, err := migrationFiles.ReadFile("migrations/000016_add_sanction_load_to_hubs.up.sql")
	if err != nil {
		t.Fatalf("read sanction-load migration: %v", err)
	}
	downBody, err := migrationFiles.ReadFile("migrations/000016_add_sanction_load_to_hubs.down.sql")
	if err != nil {
		t.Fatalf("read sanction-load rollback migration: %v", err)
	}

	if !strings.Contains(string(upBody), "chk_hubs_sanction_load") ||
		!strings.Contains(string(upBody), "CHECK (sanction_load >= 0)") {
		t.Fatal("sanction-load migration does not enforce a non-negative value")
	}
	if !strings.Contains(string(downBody), "DROP COLUMN sanction_load") {
		t.Fatal("sanction-load rollback does not remove the added column")
	}
	if !strings.Contains(string(upBody), "ALTER COLUMN hub_id DROP NOT NULL") {
		t.Fatal("sanction-load migration does not enable independent charger creation")
	}
	if !strings.Contains(string(downBody), "ALTER COLUMN hub_id SET NOT NULL") ||
		!strings.Contains(string(downBody), "independent chargers exist") {
		t.Fatal("sanction-load rollback does not protect independent chargers")
	}
}

func TestInitialMigrationContainsCompleteCMSDomain(t *testing.T) {
	t.Parallel()

	upBody, err := migrationFiles.ReadFile("migrations/000001_cms_schema.up.sql")
	if err != nil {
		t.Fatalf("read initial up migration: %v", err)
	}
	downBody, err := migrationFiles.ReadFile("migrations/000001_cms_schema.down.sql")
	if err != nil {
		t.Fatalf("read initial down migration: %v", err)
	}

	tables := []string{
		"users",
		"user_settings",
		"platform_admins",
		"cpos",
		"cpo_memberships",
		"user_groups",
		"customers",
		"hubs",
		"chargers",
		"connectors",
		"user_group_hubs",
		"user_group_chargers",
		"customer_favorite_hubs",
		"customer_favorite_chargers",
		"gsts",
		"tariffs",
		"wallets",
		"charging_sessions",
		"wallet_transactions",
		"payments",
		"audit_logs",
	}

	upSQL := strings.ReplaceAll(string(upBody), "\r\n", "\n")
	downSQL := strings.ReplaceAll(string(downBody), "\r\n", "\n")
	for _, table := range tables {
		if !strings.Contains(upSQL, "CREATE TABLE "+table) {
			t.Errorf("up migration does not create table %q", table)
		}
		if !strings.Contains(downSQL, "DROP TABLE IF EXISTS "+table) {
			t.Errorf("down migration does not drop table %q", table)
		}
	}
	if !strings.Contains(upSQL, "hub_id uuid,\n    charger_id varchar(6)") {
		t.Fatal("initial migration does not allow a charger to be created without a hub")
	}
}

func TestWalletRazorpayRechargeMigrationContainsDurableProviderRecords(t *testing.T) {
	t.Parallel()

	upBody, err := migrationFiles.ReadFile("migrations/000022_wallet_razorpay_recharge.up.sql")
	if err != nil {
		t.Fatalf("read Razorpay recharge up migration: %v", err)
	}
	downBody, err := migrationFiles.ReadFile("migrations/000022_wallet_razorpay_recharge.down.sql")
	if err != nil {
		t.Fatalf("read Razorpay recharge down migration: %v", err)
	}
	upSQL := string(upBody)
	downSQL := string(downBody)
	for _, table := range []string{
		"wallet_recharge_orders",
		"wallet_recharge_payments",
		"wallet_recharge_refunds",
	} {
		if !strings.Contains(upSQL, "CREATE TABLE "+table) {
			t.Errorf("up migration does not create table %q", table)
		}
		if !strings.Contains(downSQL, "DROP TABLE IF EXISTS "+table) {
			t.Errorf("down migration does not drop table %q", table)
		}
	}
	for _, column := range []string{
		"provider_order_id",
		"provider_payment_id",
		"provider_refund_id",
		"provider_payload",
		"payment_signature",
		"recharge_order_id",
	} {
		if !strings.Contains(upSQL, column) {
			t.Errorf("up migration does not retain provider field %q", column)
		}
	}
}

func TestAuthMigrationContainsCredentialBoundary(t *testing.T) {
	t.Parallel()

	upBody, err := migrationFiles.ReadFile("migrations/000002_auth_credentials.up.sql")
	if err != nil {
		t.Fatalf("read auth up migration: %v", err)
	}
	downBody, err := migrationFiles.ReadFile("migrations/000002_auth_credentials.down.sql")
	if err != nil {
		t.Fatalf("read auth down migration: %v", err)
	}

	tables := []string{
		"auth_challenges",
		"auth_sessions",
		"auth_refresh_tokens",
		"mail_outbox",
		"auth_rate_limits",
		"cpo_integrations",
	}
	upSQL := string(upBody)
	downSQL := string(downBody)
	for _, table := range tables {
		if !strings.Contains(upSQL, "CREATE TABLE "+table) {
			t.Errorf("up migration does not create table %q", table)
		}
		if !strings.Contains(downSQL, "DROP TABLE IF EXISTS "+table) {
			t.Errorf("down migration does not drop table %q", table)
		}
	}

	for _, column := range []string{
		"mfa_enabled",
		"password_changed_at",
		"failed_login_attempts",
		"locked_until",
		"last_login_at",
	} {
		if !strings.Contains(upSQL, "ADD COLUMN "+column) {
			t.Errorf("up migration does not add users.%s", column)
		}
		if !strings.Contains(downSQL, "DROP COLUMN IF EXISTS "+column) {
			t.Errorf("down migration does not remove users.%s", column)
		}
	}
}

func TestCompleteSuperadminMigrationContainsAuthorityMailAndNotifications(t *testing.T) {
	t.Parallel()

	upBody, err := migrationFiles.ReadFile("migrations/000014_complete_superadmin_surface.up.sql")
	if err != nil {
		t.Fatalf("read complete Superadmin up migration: %v", err)
	}
	downBody, err := migrationFiles.ReadFile("migrations/000014_complete_superadmin_surface.down.sql")
	if err != nil {
		t.Fatalf("read complete Superadmin down migration: %v", err)
	}
	upSQL := string(upBody)
	downSQL := string(downBody)
	for _, table := range []string{"platform_announcements", "platform_notifications"} {
		if !strings.Contains(upSQL, "CREATE TABLE "+table) {
			t.Errorf("up migration does not create %s", table)
		}
		if !strings.Contains(downSQL, "DROP TABLE IF EXISTS "+table) {
			t.Errorf("down migration does not remove %s", table)
		}
	}
	for _, column := range []string{"is_active", "status_reason", "status_changed_at", "status_changed_by_user_id", "updated_at"} {
		if !strings.Contains(upSQL, "ADD COLUMN "+column) {
			t.Errorf("up migration does not add platform_admins.%s", column)
		}
		if !strings.Contains(downSQL, "DROP COLUMN "+column) {
			t.Errorf("down migration does not remove platform_admins.%s", column)
		}
	}
	for _, required := range []string{"PLATFORM_ADMIN_INVITE", "PLATFORM_ADMIN_GRANTED", "CANCELED", "ix_platform_notifications_recipient_created"} {
		if !strings.Contains(upSQL, required) {
			t.Errorf("up migration is missing %q", required)
		}
	}
}

func TestTariffEffectiveDatesMigrationUsesTenantScopedTimestamptzExclusion(t *testing.T) {
	t.Parallel()

	upBody, err := migrationFiles.ReadFile("migrations/000015_tariff_effective_dates.up.sql")
	if err != nil {
		t.Fatalf("read tariff effective-dates up migration: %v", err)
	}
	downBody, err := migrationFiles.ReadFile("migrations/000015_tariff_effective_dates.down.sql")
	if err != nil {
		t.Fatalf("read tariff effective-dates down migration: %v", err)
	}
	upSQL := string(upBody)
	downSQL := string(downBody)
	for _, required := range []string{
		"start_date timestamptz",
		"end_date timestamptz",
		"btree_gist",
		"tstzrange",
		"tariffs_active_effective_period_exclusion",
		"charger_id IS NOT DISTINCT FROM older.charger_id",
		"user_group_id IS NOT DISTINCT FROM older.user_group_id",
	} {
		if !strings.Contains(upSQL, required) {
			t.Errorf("up migration is missing %q", required)
		}
	}
	for _, required := range []string{
		"tariffs_active_effective_period_exclusion",
		"tariffs_effective_dates_check",
		"DROP COLUMN IF EXISTS end_date",
		"DROP COLUMN IF EXISTS start_date",
	} {
		if !strings.Contains(downSQL, required) {
			t.Errorf("down migration is missing %q", required)
		}
	}
}

func TestTariffTargetingCorrectionMigrationMakesOneExplicitTarget(t *testing.T) {
	t.Parallel()

	upBody, err := migrationFiles.ReadFile("migrations/000037_correct_tariff_targeting.up.sql")
	if err != nil {
		t.Fatalf("read tariff-targeting migration: %v", err)
	}
	downBody, err := migrationFiles.ReadFile("migrations/000037_correct_tariff_targeting.down.sql")
	if err != nil {
		t.Fatalf("read tariff-targeting rollback migration: %v", err)
	}
	upSQL := string(upBody)
	downSQL := string(downBody)
	for _, required := range []string{
		"ALTER COLUMN hub_id DROP NOT NULL",
		"assigned_to = 'usergroup'::tariff_assignment_type",
		"assigned_to = 'charger'::tariff_assignment_type",
		"assigned_to = 'hub'::tariff_assignment_type",
		"tariffs_exactly_one_target",
		"tariffs_target_matches_assigned_to",
		"FOREIGN KEY (cpo_id, charger_id)",
		"REFERENCES chargers(cpo_id, id)",
		"tariffs_active_effective_period_exclusion",
	} {
		if !strings.Contains(upSQL, required) {
			t.Errorf("up migration is missing %q", required)
		}
	}
	for _, required := range []string{
		"cannot roll back tariff targeting",
		"ALTER COLUMN hub_id SET NOT NULL",
		"FOREIGN KEY (cpo_id, hub_id, charger_id)",
		"tariffs_exactly_one_target",
		"tariffs_target_matches_assigned_to",
	} {
		if !strings.Contains(downSQL, required) {
			t.Errorf("down migration is missing %q", required)
		}
	}
}

func TestCPOProvisioningMigrationContainsAppIdentityAndOnboarding(t *testing.T) {
	t.Parallel()

	upBody, err := migrationFiles.ReadFile("migrations/000003_cpo_provisioning.up.sql")
	if err != nil {
		t.Fatalf("read CPO provisioning up migration: %v", err)
	}
	downBody, err := migrationFiles.ReadFile("migrations/000003_cpo_provisioning.down.sql")
	if err != nil {
		t.Fatalf("read CPO provisioning down migration: %v", err)
	}
	upSQL := string(upBody)
	downSQL := string(downBody)
	for _, column := range []string{
		"must_change_password",
		"app_id",
		"app_id_mode",
		"app_id_updated_at",
	} {
		if !strings.Contains(upSQL, "ADD COLUMN "+column) {
			t.Errorf("up migration does not add %s", column)
		}
		if !strings.Contains(downSQL, "DROP COLUMN IF EXISTS "+column) {
			t.Errorf("down migration does not remove %s", column)
		}
	}
	for _, template := range []string{
		"CPO_ADMIN_WELCOME",
		"CPO_MEMBERSHIP_ASSIGNED",
		"PASSWORD_CHANGE_REMINDER",
	} {
		if !strings.Contains(upSQL, template) {
			t.Errorf("up migration does not allow mail template %s", template)
		}
	}
}

func TestCustomerSignupMigrationContainsDurableChallenge(t *testing.T) {
	t.Parallel()

	upBody, err := migrationFiles.ReadFile("migrations/000004_customer_signup.up.sql")
	if err != nil {
		t.Fatalf("read customer signup up migration: %v", err)
	}
	downBody, err := migrationFiles.ReadFile("migrations/000004_customer_signup.down.sql")
	if err != nil {
		t.Fatalf("read customer signup down migration: %v", err)
	}
	upSQL := string(upBody)
	downSQL := string(downBody)
	if !strings.Contains(upSQL, "CREATE TABLE customer_signup_challenges") {
		t.Error("up migration does not create customer_signup_challenges")
	}
	if !strings.Contains(downSQL, "DROP TABLE IF EXISTS customer_signup_challenges") {
		t.Error("down migration does not drop customer_signup_challenges")
	}
	if !strings.Contains(upSQL, "CUSTOMER_SIGNUP_OTP") {
		t.Error("up migration does not allow CUSTOMER_SIGNUP_OTP")
	}
}

func TestCustomerAuthenticationMigrationContainsSessionScope(t *testing.T) {
	t.Parallel()

	upBody, err := migrationFiles.ReadFile("migrations/000005_customer_authentication.up.sql")
	if err != nil {
		t.Fatalf("read customer authentication up migration: %v", err)
	}
	downBody, err := migrationFiles.ReadFile("migrations/000005_customer_authentication.down.sql")
	if err != nil {
		t.Fatalf("read customer authentication down migration: %v", err)
	}
	upSQL := string(upBody)
	downSQL := string(downBody)
	for _, required := range []string{
		"ADD COLUMN customer_id",
		"scope = 'CUSTOMER'",
		"CUSTOMER_LOGIN_2FA",
		"CUSTOMER_PASSWORD_RESET",
		"CUSTOMER_LOGIN_OTP",
		"CUSTOMER_PASSWORD_RESET_OTP",
	} {
		if !strings.Contains(upSQL, required) {
			t.Errorf("customer authentication up migration missing %q", required)
		}
	}
	if !strings.Contains(downSQL, "DROP COLUMN customer_id") {
		t.Error("customer authentication down migration does not remove customer_id")
	}
}

func TestCPOScopedCustomerAccountMigrationSeparatesAdministrativeIdentity(t *testing.T) {
	t.Parallel()

	upBody, err := migrationFiles.ReadFile("migrations/000020_cpo_scoped_customer_accounts.up.sql")
	if err != nil {
		t.Fatalf("read CPO-local customer account migration: %v", err)
	}
	downBody, err := migrationFiles.ReadFile("migrations/000020_cpo_scoped_customer_accounts.down.sql")
	if err != nil {
		t.Fatalf("read CPO-local customer rollback migration: %v", err)
	}
	upSQL := string(upBody)
	downSQL := string(downBody)
	for _, required := range []string{
		"requires an empty customers table",
		"DROP COLUMN IF EXISTS user_id",
		"CREATE UNIQUE INDEX uq_cpo_customer_email",
		"CREATE UNIQUE INDEX uq_customers_cpo_id_identity ON customers (cpo_id, id)",
		"chk_customers_failed_login_attempts",
		"CREATE TABLE customer_auth_challenges",
		"CREATE TABLE customer_auth_sessions",
		"CREATE TABLE customer_auth_refresh_tokens",
		"fk_customer_auth_challenge_account",
		"fk_customer_auth_session_account",
	} {
		if !strings.Contains(upSQL, required) {
			t.Errorf("CPO-local customer account migration missing %q", required)
		}
	}
	if !strings.Contains(downSQL, "CPO-local credentials cannot be reconstructed") {
		t.Fatal("customer-account rollback does not fail safely when customer data exists")
	}
}

func TestPlatformOperationsMigrationContainsDurableEventsAndWorkers(t *testing.T) {
	t.Parallel()

	upBody, err := migrationFiles.ReadFile("migrations/000006_platform_operations.up.sql")
	if err != nil {
		t.Fatalf("read platform operations up migration: %v", err)
	}
	downBody, err := migrationFiles.ReadFile("migrations/000006_platform_operations.down.sql")
	if err != nil {
		t.Fatalf("read platform operations down migration: %v", err)
	}
	upSQL := string(upBody)
	downSQL := string(downBody)
	for _, table := range []string{"platform_events", "worker_instances"} {
		if !strings.Contains(upSQL, "CREATE TABLE "+table) {
			t.Errorf("platform operations up migration does not create %s", table)
		}
		if !strings.Contains(downSQL, "DROP TABLE IF EXISTS "+table) {
			t.Errorf("platform operations down migration does not remove %s", table)
		}
	}
	for _, required := range []string{
		"GENERATED ALWAYS AS IDENTITY",
		"expires_at",
		"last_heartbeat_at",
		"uq_worker_instances_identity",
	} {
		if !strings.Contains(upSQL, required) {
			t.Errorf("platform operations migration missing %q", required)
		}
	}
}

func TestWorkerCurrentInstanceMigrationPreservesHistoryAndOneCurrentProjection(t *testing.T) {
	t.Parallel()

	upBody, err := migrationFiles.ReadFile("migrations/000039_make_worker_current_instance_explicit.up.sql")
	if err != nil {
		t.Fatalf("read worker-current migration: %v", err)
	}
	downBody, err := migrationFiles.ReadFile("migrations/000039_make_worker_current_instance_explicit.down.sql")
	if err != nil {
		t.Fatalf("read worker-current rollback: %v", err)
	}
	upSQL, downSQL := string(upBody), string(downBody)
	for _, required := range []string{
		"ADD COLUMN is_current boolean NOT NULL DEFAULT false",
		"PARTITION BY worker_name",
		"uq_worker_instances_current_worker",
		"WHERE is_current = true",
	} {
		if !strings.Contains(upSQL, required) {
			t.Errorf("worker-current migration missing %q", required)
		}
	}
	if !strings.Contains(downSQL, "DROP INDEX IF EXISTS uq_worker_instances_current_worker") ||
		!strings.Contains(downSQL, "DROP COLUMN IF EXISTS is_current") {
		t.Error("worker-current rollback does not remove only the new projection shape")
	}
}

func TestTariffPriceRenameMigrationPreservesTheExistingColumn(t *testing.T) {
	t.Parallel()

	upBody, err := migrationFiles.ReadFile("migrations/000040_rename_tariff_price_per_unit.up.sql")
	if err != nil {
		t.Fatalf("read tariff-price rename migration: %v", err)
	}
	downBody, err := migrationFiles.ReadFile("migrations/000040_rename_tariff_price_per_unit.down.sql")
	if err != nil {
		t.Fatalf("read tariff-price rename rollback: %v", err)
	}
	upSQL, downSQL := string(upBody), string(downBody)
	for _, required := range []string{
		"RENAME COLUMN price_per_kwh TO price_per_unit",
		"RENAME CONSTRAINT chk_tariffs_price TO chk_tariffs_price_per_unit",
	} {
		if !strings.Contains(upSQL, required) {
			t.Errorf("tariff-price migration missing %q", required)
		}
	}
	if strings.Contains(upSQL, "DROP COLUMN") ||
		!strings.Contains(downSQL, "RENAME COLUMN price_per_unit TO price_per_kwh") {
		t.Error("tariff-price migration is not a reversible data-preserving rename")
	}
}

func TestTariffEnergyUnitAndHubGSTMigrationPreservesStoredValues(t *testing.T) {
	t.Parallel()

	upBody, err := migrationFiles.ReadFile("migrations/000042_correct_tariff_energy_unit_and_hub_gst_uniqueness.up.sql")
	if err != nil {
		t.Fatalf("read tariff-energy migration: %v", err)
	}
	downBody, err := migrationFiles.ReadFile("migrations/000042_correct_tariff_energy_unit_and_hub_gst_uniqueness.down.sql")
	if err != nil {
		t.Fatalf("read tariff-energy rollback: %v", err)
	}
	upSQL, downSQL := string(upBody), string(downBody)
	for _, required := range []string{
		"ALTER TYPE units RENAME VALUE 'watt/hour' TO 'kwh'",
		"uq_hubs_cpo_gst_id",
		"WHERE gst_id IS NOT NULL",
	} {
		if !strings.Contains(upSQL, required) {
			t.Errorf("tariff-energy migration missing %q", required)
		}
	}
	if strings.Contains(upSQL, "UPDATE tariffs") || strings.Contains(upSQL, "price_per_unit =") {
		t.Error("tariff-energy migration must not rewrite stored commercial prices")
	}
	if !strings.Contains(downSQL, "ALTER TYPE units RENAME VALUE 'kwh' TO 'watt/hour'") ||
		!strings.Contains(downSQL, "DROP INDEX IF EXISTS uq_hubs_cpo_gst_id") {
		t.Error("tariff-energy rollback does not restore only the prior enum and uniqueness shape")
	}
}

func TestSubscriptionMigrationContainsVersionedLifecycle(t *testing.T) {
	t.Parallel()

	upBody, err := migrationFiles.ReadFile("migrations/000007_subscriptions.up.sql")
	if err != nil {
		t.Fatalf("read subscriptions up migration: %v", err)
	}
	downBody, err := migrationFiles.ReadFile("migrations/000007_subscriptions.down.sql")
	if err != nil {
		t.Fatalf("read subscriptions down migration: %v", err)
	}
	upSQL := string(upBody)
	downSQL := string(downBody)
	for _, table := range []string{
		"subscription_plans",
		"subscription_plan_versions",
		"subscription_plan_entitlements",
		"cpo_subscriptions",
		"cpo_subscription_history",
		"cpo_entitlement_overrides",
	} {
		if !strings.Contains(upSQL, "CREATE TABLE "+table) {
			t.Errorf("subscription up migration does not create %s", table)
		}
		if !strings.Contains(downSQL, "DROP TABLE IF EXISTS "+table) {
			t.Errorf("subscription down migration does not remove %s", table)
		}
	}
	for _, required := range []string{
		"uq_cpo_subscriptions_current",
		"reject_published_subscription_version_mutation",
		"reject_published_entitlement_mutation",
		"CPO_SUBSCRIPTION_CHANGED",
		"idempotency_key",
	} {
		if !strings.Contains(upSQL, required) {
			t.Errorf("subscription migration missing %q", required)
		}
	}
}

func TestPlatformBillingMigrationContainsExactImmutableRecords(t *testing.T) {
	t.Parallel()

	upBody, err := migrationFiles.ReadFile("migrations/000008_platform_billing.up.sql")
	if err != nil {
		t.Fatalf("read platform billing up migration: %v", err)
	}
	downBody, err := migrationFiles.ReadFile("migrations/000008_platform_billing.down.sql")
	if err != nil {
		t.Fatalf("read platform billing down migration: %v", err)
	}
	upSQL := string(upBody)
	downSQL := string(downBody)
	for _, table := range []string{
		"cpo_billing_accounts",
		"platform_invoices",
		"platform_invoice_lines",
		"platform_payments",
		"platform_payment_allocations",
	} {
		if !strings.Contains(upSQL, "CREATE TABLE "+table) {
			t.Errorf("platform billing up migration does not create %s", table)
		}
		if !strings.Contains(downSQL, "DROP TABLE IF EXISTS "+table) {
			t.Errorf("platform billing down migration does not remove %s", table)
		}
	}
	for _, required := range []string{
		"unit_amount_minor",
		"protect_issued_platform_invoice",
		"protect_platform_invoice_line",
		"uq_platform_invoices_idempotency",
		"uq_platform_payments_idempotency",
		"CPO_PLATFORM_INVOICE_ISSUED",
	} {
		if !strings.Contains(upSQL, required) {
			t.Errorf("platform billing migration missing %q", required)
		}
	}
}

func TestCommercialRetirementMigrationArchivesWithoutDeletingData(t *testing.T) {
	t.Parallel()

	upBody, err := migrationFiles.ReadFile(
		"migrations/000009_retire_subscriptions_and_platform_billing.up.sql",
	)
	if err != nil {
		t.Fatalf("read commercial retirement up migration: %v", err)
	}
	downBody, err := migrationFiles.ReadFile(
		"migrations/000009_retire_subscriptions_and_platform_billing.down.sql",
	)
	if err != nil {
		t.Fatalf("read commercial retirement down migration: %v", err)
	}
	upSQL := string(upBody)
	downSQL := string(downBody)
	if !strings.Contains(upSQL, "CREATE SCHEMA retired_commercial") {
		t.Error("commercial retirement must preserve tables in an archive schema")
	}
	for _, table := range []string{
		"subscription_plans",
		"cpo_subscriptions",
		"cpo_entitlement_overrides",
		"cpo_billing_accounts",
		"platform_invoices",
		"platform_payments",
	} {
		if !strings.Contains(upSQL, "ALTER TABLE "+table) ||
			!strings.Contains(upSQL, "SET SCHEMA retired_commercial") {
			t.Errorf("retirement migration does not archive %s", table)
		}
		if !strings.Contains(
			downSQL,
			"ALTER TABLE retired_commercial."+table,
		) || !strings.Contains(downSQL, "SET SCHEMA public") {
			t.Errorf("retirement down migration does not restore %s", table)
		}
	}
	for _, required := range []string{
		"status IN ('PENDING', 'PROCESSING')",
		"'subscription-lifecycle', 'billing-maintenance'",
		"reported_status = 'DISABLED'",
	} {
		if !strings.Contains(upSQL, required) {
			t.Errorf("commercial retirement migration missing %q", required)
		}
	}
	if strings.Contains(upSQL, "DROP TABLE") {
		t.Error("commercial retirement must not drop historical data tables")
	}
}

func TestCPOSuperadminDependencyMigrationContainsRecoveryInvariants(t *testing.T) {
	t.Parallel()

	upBody, err := migrationFiles.ReadFile(
		"migrations/000010_cpo_superadmin_dependency.up.sql",
	)
	if err != nil {
		t.Fatalf("read CPO Superadmin dependency up migration: %v", err)
	}
	downBody, err := migrationFiles.ReadFile(
		"migrations/000010_cpo_superadmin_dependency.down.sql",
	)
	if err != nil {
		t.Fatalf("read CPO Superadmin dependency down migration: %v", err)
	}
	upSQL := string(upBody)
	downSQL := string(downBody)
	for _, required := range []string{
		"status_reason",
		"status_changed_at",
		"status_changed_by_user_id",
		"is_primary_admin",
		"uq_cpo_memberships_primary_admin",
		"ix_mail_outbox_cpo_user_created",
		"CPO_ONBOARDING_RESENT",
	} {
		if !strings.Contains(upSQL, required) {
			t.Errorf("CPO Superadmin dependency migration missing %q", required)
		}
	}
	for _, required := range []string{
		"cannot roll back CPO Superadmin dependency",
		"DROP COLUMN IF EXISTS is_primary_admin",
		"DROP COLUMN IF EXISTS status_reason",
	} {
		if !strings.Contains(downSQL, required) {
			t.Errorf(
				"CPO Superadmin dependency down migration missing %q",
				required,
			)
		}
	}
}

func TestCPORequiredRegistrationFieldsMigrationContainsDurableInvariants(t *testing.T) {
	t.Parallel()

	initialBody, err := migrationFiles.ReadFile("migrations/000001_cms_schema.up.sql")
	if err != nil {
		t.Fatalf("read initial CPO uniqueness migration: %v", err)
	}
	upBody, err := migrationFiles.ReadFile(
		"migrations/000011_cpo_required_registration_fields.up.sql",
	)
	if err != nil {
		t.Fatalf("read required CPO registration fields up migration: %v", err)
	}
	downBody, err := migrationFiles.ReadFile(
		"migrations/000011_cpo_required_registration_fields.down.sql",
	)
	if err != nil {
		t.Fatalf("read required CPO registration fields down migration: %v", err)
	}

	upSQL := string(upBody)
	downSQL := string(downBody)
	initialSQL := string(initialBody)
	for _, required := range []string{
		"CREATE UNIQUE INDEX uq_cpos_slug_normalized ON cpos (lower(slug))",
		"CREATE UNIQUE INDEX uq_cpos_gstin_normalized",
		"ON cpos (upper(gstin))",
	} {
		if !strings.Contains(initialSQL, required) {
			t.Errorf("CPO uniqueness migration missing %q", required)
		}
	}
	for _, required := range []string{
		"cannot require CPO GSTIN and address fields",
		"ALTER COLUMN gstin SET NOT NULL",
		"chk_cpos_address_not_blank",
		"chk_cpos_city_not_blank",
		"chk_cpos_state_not_blank",
		"chk_cpos_pincode_not_blank",
	} {
		if !strings.Contains(upSQL, required) {
			t.Errorf("required CPO registration fields migration missing %q", required)
		}
	}
	for _, required := range []string{
		"ALTER COLUMN gstin DROP NOT NULL",
		"ALTER COLUMN address SET DEFAULT ''",
		"gstin IS NULL OR gstin ~ '^[0-9A-Z]{15}$'",
	} {
		if !strings.Contains(downSQL, required) {
			t.Errorf("required CPO registration fields down migration missing %q", required)
		}
	}
}

func TestManualSubscriptionRestoreMigrationPreservesBillingRetirement(t *testing.T) {
	t.Parallel()

	upBody, err := migrationFiles.ReadFile(
		"migrations/000012_restore_manual_subscriptions.up.sql",
	)
	if err != nil {
		t.Fatalf("read manual subscription restore up migration: %v", err)
	}
	downBody, err := migrationFiles.ReadFile(
		"migrations/000012_restore_manual_subscriptions.down.sql",
	)
	if err != nil {
		t.Fatalf("read manual subscription restore down migration: %v", err)
	}

	upSQL := strings.ReplaceAll(string(upBody), "\r\n", "\n")
	downSQL := strings.ReplaceAll(string(downBody), "\r\n", "\n")
	for _, table := range []string{
		"subscription_plans",
		"subscription_plan_versions",
		"subscription_plan_entitlements",
		"cpo_subscriptions",
		"cpo_subscription_history",
		"cpo_entitlement_overrides",
	} {
		if !strings.Contains(upSQL, "ALTER TABLE retired_commercial."+table) ||
			!strings.Contains(upSQL, "SET SCHEMA public") {
			t.Errorf("manual subscription restore does not restore %s", table)
		}
		if !strings.Contains(downSQL, "ALTER TABLE "+table+" SET SCHEMA retired_commercial") {
			t.Errorf("manual subscription restore down migration does not retire %s", table)
		}
	}
	for _, forbidden := range []string{
		"platform_invoices",
		"platform_payments",
		"cpo_billing_accounts",
		"required = TRUE",
	} {
		if strings.Contains(upSQL, forbidden) {
			t.Errorf("manual subscription restore must not reactivate billing or workers: %s", forbidden)
		}
	}
	for _, required := range []string{
		"trg_subscription_plan_versions_immutable",
		"trg_subscription_plan_entitlements_immutable",
		"manual_subscription_management_has_no_automatic_lifecycle",
	} {
		if !strings.Contains(upSQL, required) {
			t.Errorf("manual subscription restore migration missing %q", required)
		}
	}
}

func TestDormantEntitlementRetirementMigrationPreservesSubscriptionTables(t *testing.T) {
	t.Parallel()

	upBody, err := migrationFiles.ReadFile(
		"migrations/000013_retire_dormant_subscription_entitlements.up.sql",
	)
	if err != nil {
		t.Fatalf("read dormant entitlement retirement up migration: %v", err)
	}
	downBody, err := migrationFiles.ReadFile(
		"migrations/000013_retire_dormant_subscription_entitlements.down.sql",
	)
	if err != nil {
		t.Fatalf("read dormant entitlement retirement down migration: %v", err)
	}

	upSQL := strings.ReplaceAll(string(upBody), "\r\n", "\n")
	downSQL := strings.ReplaceAll(string(downBody), "\r\n", "\n")
	for _, table := range []string{
		"subscription_plan_entitlements",
		"cpo_entitlement_overrides",
	} {
		if !strings.Contains(upSQL, "ALTER TABLE "+table+"\n    SET SCHEMA retired_commercial") {
			t.Errorf("dormant entitlement retirement does not retire %s", table)
		}
		if !strings.Contains(downSQL, "ALTER TABLE retired_commercial."+table+"\n    SET SCHEMA public") {
			t.Errorf("dormant entitlement retirement down migration does not restore %s", table)
		}
	}
	for _, activeTable := range []string{
		"subscription_plans",
		"subscription_plan_versions",
		"cpo_subscriptions",
		"cpo_subscription_history",
	} {
		if strings.Contains(upSQL, "ALTER TABLE "+activeTable) {
			t.Errorf("dormant entitlement retirement must keep %s active", activeTable)
		}
	}
}
