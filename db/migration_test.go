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
