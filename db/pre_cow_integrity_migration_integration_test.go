package db

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// These tests use an isolated transaction-local schema on an EXPLICITLY
// DISPOSABLE PostgreSQL target. Nothing is committed to the target database.
// TEST_DATABASE_URL must never point at development, staging, or production.
func preCoWTestTx(t *testing.T) (context.Context, *sql.Tx) {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is not set (disposable PostgreSQL required)")
	}
	ctx := context.Background()
	_, connection, err := Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = connection.Close() })
	tx, err := connection.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tx.Rollback() })
	schema := "pre_cow_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	mustSQL(t, ctx, tx, "CREATE SCHEMA "+schema+"; SET LOCAL search_path TO "+schema+", pg_catalog")
	return ctx, tx
}
func mustSQL(t *testing.T, ctx context.Context, tx *sql.Tx, query string, args ...any) {
	t.Helper()
	if _, err := tx.ExecContext(ctx, query, args...); err != nil {
		t.Fatalf("SQL failed: %v", err)
	}
}
func expectRejected(t *testing.T, ctx context.Context, tx *sql.Tx, query string, args ...any) {
	t.Helper()
	mustSQL(t, ctx, tx, "SAVEPOINT expected_rejection")
	_, err := tx.ExecContext(ctx, query, args...)
	if err == nil {
		t.Fatalf("SQL unexpectedly accepted a forbidden state: %s", query)
	}
	mustSQL(t, ctx, tx, "ROLLBACK TO SAVEPOINT expected_rejection")
	mustSQL(t, ctx, tx, "RELEASE SAVEPOINT expected_rejection")
}
func applyPreCoWMigration(t *testing.T, ctx context.Context, tx *sql.Tx, version string) {
	t.Helper()
	body, err := migrationFiles.ReadFile("migrations/" + version + ".up.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, string(body)); err != nil {
		t.Fatalf("apply isolated migration %s: %v", version, err)
	}
}
func checkPreCoWDownRefuses(t *testing.T, ctx context.Context, tx *sql.Tx, version string) {
	t.Helper()
	body, err := migrationFiles.ReadFile("migrations/" + version + ".down.sql")
	if err != nil {
		t.Fatal(err)
	}
	expectRejected(t, ctx, tx, string(body))
}

func TestPreCoWNoticeAndGSTIntegrityWithPostgreSQL(t *testing.T) {
	ctx, tx := preCoWTestTx(t)
	mustSQL(t, ctx, tx, `
CREATE TABLE gsts(cpo_id uuid NOT NULL,id uuid NOT NULL,PRIMARY KEY(cpo_id,id));
CREATE TABLE hubs(id uuid PRIMARY KEY,cpo_id uuid NOT NULL,gst_id uuid,
    CONSTRAINT fk_hubs_gst FOREIGN KEY(cpo_id,gst_id) REFERENCES gsts(cpo_id,id) ON DELETE SET NULL);
CREATE TABLE platform_announcements(id uuid PRIMARY KEY,audience varchar(20) NOT NULL);
CREATE TABLE platform_announcement_cpos(announcement_id uuid NOT NULL REFERENCES platform_announcements(id) ON DELETE CASCADE,
    cpo_id uuid NOT NULL,PRIMARY KEY(announcement_id,cpo_id));
CREATE TABLE platform_notifications(id uuid PRIMARY KEY,announcement_id uuid NOT NULL REFERENCES platform_announcements(id) ON DELETE CASCADE,
    recipient_user_id uuid NOT NULL,cpo_id uuid,
    CONSTRAINT uq_platform_notifications_recipient_scope UNIQUE(announcement_id,recipient_user_id,cpo_id));`)
	cpoA, cpoB, gst, hub := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	platform, targeted, recipient := uuid.New(), uuid.New(), uuid.New()
	mustSQL(t, ctx, tx, `INSERT INTO gsts VALUES ($1,$2)`, cpoA, gst)
	mustSQL(t, ctx, tx, `INSERT INTO hubs VALUES ($1,$2,$3)`, hub, cpoA, gst)
	mustSQL(t, ctx, tx, `INSERT INTO platform_announcements VALUES ($1,'PLATFORM'),($2,'CPO')`, platform, targeted)
	mustSQL(t, ctx, tx, `INSERT INTO platform_announcement_cpos VALUES ($1,$2)`, targeted, cpoA)
	mustSQL(t, ctx, tx, `INSERT INTO platform_notifications(id,announcement_id,recipient_user_id,cpo_id) VALUES ($1,$2,$3,NULL)`, uuid.New(), platform, recipient)
	mustSQL(t, ctx, tx, `INSERT INTO platform_notifications VALUES ($1,$2,$3,$4)`, uuid.New(), targeted, recipient, cpoA)
	applyPreCoWMigration(t, ctx, tx, "000078_pre_cow_notice_and_gst_integrity")
	// Enforce the intentional deferred target FK immediately in this test.
	mustSQL(t, ctx, tx, `SET CONSTRAINTS fk_platform_notifications_announcement_target IMMEDIATE`)
	expectRejected(t, ctx, tx, `INSERT INTO platform_notifications(id,announcement_id,recipient_user_id,cpo_id) VALUES ($1,$2,$3,NULL)`, uuid.New(), platform, recipient)
	expectRejected(t, ctx, tx, `INSERT INTO platform_notifications VALUES ($1,$2,$3,$4)`, uuid.New(), platform, uuid.New(), cpoA)
	expectRejected(t, ctx, tx, `INSERT INTO platform_notifications(id,announcement_id,recipient_user_id,cpo_id) VALUES ($1,$2,$3,NULL)`, uuid.New(), targeted, uuid.New())
	expectRejected(t, ctx, tx, `INSERT INTO platform_notifications VALUES ($1,$2,$3,$4)`, uuid.New(), targeted, uuid.New(), cpoB)
	expectRejected(t, ctx, tx, `UPDATE platform_announcements SET audience='PLATFORM' WHERE id=$1`, targeted)
	expectRejected(t, ctx, tx, `DELETE FROM gsts WHERE cpo_id=$1 AND id=$2`, cpoA, gst)
	// The existing operator's explicit Hub-GST unassignment remains usable.
	mustSQL(t, ctx, tx, `UPDATE hubs SET gst_id=NULL WHERE id=$1`, hub)
	mustSQL(t, ctx, tx, `DELETE FROM gsts WHERE cpo_id=$1 AND id=$2`, cpoA, gst)
	checkPreCoWDownRefuses(t, ctx, tx, "000078_pre_cow_notice_and_gst_integrity")
}

func TestPreCoWChargingTenantIntegrityWithPostgreSQL(t *testing.T) {
	ctx, tx := preCoWTestTx(t)
	mustSQL(t, ctx, tx, `
CREATE TABLE customers(cpo_id uuid NOT NULL,id uuid NOT NULL,UNIQUE(cpo_id,id));
CREATE TABLE chargers(cpo_id uuid NOT NULL,id uuid NOT NULL,UNIQUE(cpo_id,id));
CREATE TABLE connectors(cpo_id uuid NOT NULL,charger_id uuid NOT NULL,id uuid NOT NULL,
    UNIQUE(cpo_id,charger_id,id));
CREATE TABLE tariffs(cpo_id uuid NOT NULL,id uuid NOT NULL,UNIQUE(cpo_id,id));
CREATE TABLE wallets(cpo_id uuid NOT NULL,customer_id uuid NOT NULL,id uuid NOT NULL,
    UNIQUE(cpo_id,id));
CREATE TABLE charging_start_intents(id uuid PRIMARY KEY,cpo_id uuid NOT NULL,customer_id uuid NOT NULL,
    charger_id uuid NOT NULL,connector_id uuid NOT NULL,wallet_id uuid NOT NULL,tariff_id uuid NOT NULL,
    materialized_session_id uuid);
CREATE TABLE charging_sessions(id uuid PRIMARY KEY,cpo_id uuid NOT NULL,customer_id uuid NOT NULL,
    start_intent_id uuid,UNIQUE(cpo_id,id));
CREATE TABLE wallet_holds(cpo_id uuid NOT NULL,wallet_id uuid NOT NULL,start_intent_id uuid NOT NULL);
CREATE TABLE hal_command_records(cpo_id uuid NOT NULL,start_intent_id uuid,charging_session_id uuid);
CREATE TABLE hal_charger_mappings(cpo_id uuid NOT NULL,cms_charger_id uuid NOT NULL);
CREATE TABLE hal_charger_runtime(cpo_id uuid NOT NULL,cms_charger_id uuid NOT NULL);
CREATE TABLE hal_connector_runtime(cpo_id uuid NOT NULL,cms_charger_id uuid NOT NULL,cms_connector_id uuid NOT NULL);
CREATE TABLE charger_operations(cpo_id uuid NOT NULL,charger_id uuid NOT NULL,connector_id uuid);
CREATE TABLE charging_session_invoices(cpo_id uuid NOT NULL,customer_id uuid NOT NULL,session_id uuid NOT NULL);`)
	a, b := uuid.New(), uuid.New()
	customerA, customerA2, customerB := uuid.New(), uuid.New(), uuid.New()
	chargerA, chargerB := uuid.New(), uuid.New()
	connectorA, connectorB := uuid.New(), uuid.New()
	walletA, walletA2, tariffA := uuid.New(), uuid.New(), uuid.New()
	intentA, sessionA := uuid.New(), uuid.New()
	mustSQL(t, ctx, tx, `INSERT INTO customers VALUES ($1,$2),($1,$3),($4,$5)`, a, customerA, customerA2, b, customerB)
	mustSQL(t, ctx, tx, `INSERT INTO chargers VALUES ($1,$2),($3,$4)`, a, chargerA, b, chargerB)
	mustSQL(t, ctx, tx, `INSERT INTO connectors VALUES ($1,$2,$3),($4,$5,$6)`, a, chargerA, connectorA, b, chargerB, connectorB)
	mustSQL(t, ctx, tx, `INSERT INTO tariffs VALUES ($1,$2)`, a, tariffA)
	mustSQL(t, ctx, tx, `INSERT INTO wallets VALUES ($1,$2,$3),($1,$4,$5)`, a, customerA, walletA, customerA2, walletA2)
	mustSQL(t, ctx, tx, `INSERT INTO charging_start_intents VALUES ($1,$2,$3,$4,$5,$6,$7,NULL)`, intentA, a, customerA, chargerA, connectorA, walletA, tariffA)
	mustSQL(t, ctx, tx, `INSERT INTO charging_sessions VALUES ($1,$2,$3,$4)`, sessionA, a, customerA, intentA)
	mustSQL(t, ctx, tx, `INSERT INTO wallet_holds VALUES ($1,$2,$3)`, a, walletA, intentA)
	mustSQL(t, ctx, tx, `INSERT INTO hal_command_records VALUES ($1,$2,$3)`, a, intentA, sessionA)
	mustSQL(t, ctx, tx, `INSERT INTO hal_charger_mappings VALUES ($1,$2)`, a, chargerA)
	mustSQL(t, ctx, tx, `INSERT INTO hal_charger_runtime VALUES ($1,$2)`, a, chargerA)
	mustSQL(t, ctx, tx, `INSERT INTO hal_connector_runtime VALUES ($1,$2,$3)`, a, chargerA, connectorA)
	mustSQL(t, ctx, tx, `INSERT INTO charger_operations VALUES ($1,$2,$3)`, a, chargerA, connectorA)
	mustSQL(t, ctx, tx, `INSERT INTO charging_session_invoices VALUES ($1,$2,$3)`, a, customerA, sessionA)
	applyPreCoWMigration(t, ctx, tx, "000079_pre_cow_charging_tenant_integrity")
	expectRejected(t, ctx, tx, `INSERT INTO charging_start_intents VALUES ($1,$2,$3,$4,$5,$6,$7,NULL)`, uuid.New(), a, customerB, chargerA, connectorA, walletA, tariffA)
	expectRejected(t, ctx, tx, `INSERT INTO charging_start_intents VALUES ($1,$2,$3,$4,$5,$6,$7,NULL)`, uuid.New(), a, customerA, chargerA, connectorA, walletA2, tariffA)
	expectRejected(t, ctx, tx, `INSERT INTO wallet_holds VALUES ($1,$2,$3)`, a, walletA2, intentA)
	expectRejected(t, ctx, tx, `INSERT INTO hal_charger_runtime VALUES ($1,$2)`, a, chargerB)
	expectRejected(t, ctx, tx, `INSERT INTO hal_connector_runtime VALUES ($1,$2,$3)`, a, chargerA, connectorB)
	expectRejected(t, ctx, tx, `INSERT INTO charging_session_invoices VALUES ($1,$2,$3)`, a, customerA2, sessionA)
	expectRejected(t, ctx, tx, `INSERT INTO charger_operations VALUES ($1,$2,NULL)`, a, chargerB)
	checkPreCoWDownRefuses(t, ctx, tx, "000079_pre_cow_charging_tenant_integrity")
}
