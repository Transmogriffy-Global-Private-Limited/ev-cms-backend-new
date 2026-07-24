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
