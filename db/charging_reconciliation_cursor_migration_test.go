package db

import (
	"strings"
	"testing"
)

func TestChargingReconciliationCursorMigrationKeepsSchedulingSeparateFromSessionTruth(t *testing.T) {
	up, err := migrationFiles.ReadFile("migrations/000067_add_charging_reconciliation_cursor.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	down, err := migrationFiles.ReadFile("migrations/000067_add_charging_reconciliation_cursor.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		"CREATE TABLE charging_reconciliation_cursors",
		"last_session_id uuid NULL",
		"CREATE INDEX idx_charging_sessions_open_hal_reconciliation_cursor",
		"status IN ('ACTIVE', 'STOP_PENDING', 'RECONCILIATION_REQUIRED')",
	} {
		if !strings.Contains(string(up), required) {
			t.Fatalf("up migration missing %q", required)
		}
	}
	for _, forbidden := range []string{"ALTER TABLE charging_sessions", "UPDATE charging_sessions", "DELETE FROM charging_sessions"} {
		if strings.Contains(string(up), forbidden) {
			t.Fatalf("scheduler migration must not mutate session truth: %q", forbidden)
		}
	}
	if !strings.Contains(string(down), "DROP TABLE IF EXISTS charging_reconciliation_cursors") || !strings.Contains(string(down), "DROP INDEX IF EXISTS idx_charging_sessions_open_hal_reconciliation_cursor") {
		t.Fatalf("down migration does not remove only the cursor scheduler state: %s", down)
	}
}
