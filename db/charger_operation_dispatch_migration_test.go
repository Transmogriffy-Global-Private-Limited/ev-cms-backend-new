package db

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestChargerOperationDispatchMigrationPreservesLegacyUncertaintyWithPostgreSQL(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	_, database, err := Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	schema := "dispatch_migration_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	// A transaction-local schema models the pre-70 row shape without touching
	// the caller's application tables or migration ledger.
	if _, err := tx.ExecContext(ctx, `CREATE SCHEMA `+schema+`; SET LOCAL search_path TO `+schema+`;
 CREATE TABLE charger_operations (
 id uuid PRIMARY KEY, trace_id uuid, cpo_id uuid, charger_id uuid, connector_id uuid,
 kind text, parameters jsonb, correlation_id text, idempotency_key text, request_digest text,
 state text CONSTRAINT charger_operations_state_check CHECK(state IN ('PERSISTED','HAL_ACCEPTED','OCPP_CONFIRMED','RECONCILIATION_REQUIRED','CONFIRMED_ABSENT')),
 failure_category text,completed_at timestamptz,updated_at timestamptz);
 INSERT INTO charger_operations(id,state,completed_at) VALUES(gen_random_uuid(),'PERSISTED',NULL),(gen_random_uuid(),'RECONCILIATION_REQUIRED',now());`); err != nil {
		t.Fatal(err)
	}
	up, err := migrationFiles.ReadFile("migrations/000070_durable_charger_operation_dispatch.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, string(up)); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM charger_operations WHERE state='RECONCILIATION_REQUIRED' AND completed_at IS NULL AND delivery_attempted_at IS NULL AND dispatch_charger_identity IS NULL`).Scan(&count); err != nil || count != 2 {
		t.Fatalf("legacy conversion count=%d err=%v", count, err)
	}
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM charger_operations WHERE failure_category='legacy_delivery_unknown'`).Scan(&count); err != nil || count != 1 {
		t.Fatal("legacy absence was invented")
	}
	if _, err := tx.ExecContext(ctx, "SAVEPOINT invalid_insert"); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO charger_operations(id,state) VALUES(gen_random_uuid(),'PERSISTED')`); err == nil {
		t.Fatal("unfrozen operation accepted")
	}
	if _, err := tx.ExecContext(ctx, "ROLLBACK TO SAVEPOINT invalid_insert"); err != nil {
		t.Fatal(err)
	}
	down, err := migrationFiles.ReadFile("migrations/000070_durable_charger_operation_dispatch.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, string(down)); err == nil {
		t.Fatal("unsafe rollback accepted")
	}
}
