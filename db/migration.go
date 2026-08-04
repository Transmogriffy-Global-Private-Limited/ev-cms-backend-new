package db

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strings"
)

//go:embed migrations/*.up.sql migrations/*.down.sql
var migrationFiles embed.FS

const migrationLockName = "ev_cms_schema_migrations"

func ApplyMigrations(ctx context.Context, database *sql.DB) error {
	return withMigrationLock(ctx, database, func(connection *sql.Conn) error {
		if err := ensureMigrationTable(ctx, connection); err != nil {
			return err
		}

		names, err := embeddedMigrationNames()
		if err != nil {
			return err
		}

		for _, name := range names {
			if err := applyMigration(ctx, connection, name); err != nil {
				return err
			}
		}
		return nil
	})
}

// RollbackLastMigration executes the matching down migration for the most
// recently applied version. It is intentionally not called by service startup.
func RollbackLastMigration(ctx context.Context, database *sql.DB) error {
	return withMigrationLock(ctx, database, func(connection *sql.Conn) error {
		if err := ensureMigrationTable(ctx, connection); err != nil {
			return err
		}

		var upName string
		err := connection.QueryRowContext(
			ctx,
			"SELECT version FROM schema_migrations ORDER BY version DESC LIMIT 1",
		).Scan(&upName)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("find latest migration: %w", err)
		}

		downName, err := matchingDownMigration(upName)
		if err != nil {
			return err
		}
		body, err := migrationFiles.ReadFile("migrations/" + downName)
		if err != nil {
			return fmt.Errorf("read rollback migration %s: %w", downName, err)
		}
		if len(strings.TrimSpace(string(body))) == 0 {
			return fmt.Errorf("rollback migration %s is empty", downName)
		}

		transaction, err := connection.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("begin rollback %s: %w", downName, err)
		}
		defer transaction.Rollback()

		if _, err := transaction.ExecContext(ctx, string(body)); err != nil {
			return fmt.Errorf("execute rollback %s: %w", downName, err)
		}
		if _, err := transaction.ExecContext(
			ctx,
			"DELETE FROM schema_migrations WHERE version = $1",
			upName,
		); err != nil {
			return fmt.Errorf("remove migration record %s: %w", upName, err)
		}
		if err := transaction.Commit(); err != nil {
			return fmt.Errorf("commit rollback %s: %w", downName, err)
		}
		return nil
	})
}

func withMigrationLock(
	ctx context.Context,
	database *sql.DB,
	operation func(*sql.Conn) error,
) error {
	connection, err := database.Conn(ctx)
	if err != nil {
		return fmt.Errorf("acquire migration connection: %w", err)
	}
	defer connection.Close()

	if _, err := connection.ExecContext(
		ctx,
		"SELECT pg_advisory_lock(hashtext($1))",
		migrationLockName,
	); err != nil {
		return fmt.Errorf("acquire migration lock: %w", err)
	}
	defer func() {
		_, _ = connection.ExecContext(
			context.Background(),
			"SELECT pg_advisory_unlock(hashtext($1))",
			migrationLockName,
		)
	}()

	return operation(connection)
}

func ensureMigrationTable(ctx context.Context, connection *sql.Conn) error {
	if _, err := connection.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version varchar(255) PRIMARY KEY,
			applied_at timestamptz NOT NULL DEFAULT now()
		)
	`); err != nil {
		return fmt.Errorf("create schema migrations table: %w", err)
	}
	return nil
}

func embeddedMigrationNames() ([]string, error) {
	entries, err := fs.ReadDir(migrationFiles, "migrations")
	if err != nil {
		return nil, fmt.Errorf("read embedded migrations: %w", err)
	}

	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".up.sql") {
			continue
		}
		names = append(names, entry.Name())
	}
	sort.Strings(names)

	return names, nil
}

func matchingDownMigration(upName string) (string, error) {
	if !strings.HasSuffix(upName, ".up.sql") {
		return "", fmt.Errorf("migration version %q is not an up migration", upName)
	}

	downName := strings.TrimSuffix(upName, ".up.sql") + ".down.sql"
	if _, err := fs.Stat(migrationFiles, "migrations/"+downName); err != nil {
		return "", fmt.Errorf("matching down migration for %s: %w", upName, err)
	}
	return downName, nil
}

func applyMigration(
	ctx context.Context,
	connection *sql.Conn,
	name string,
) error {
	transaction, err := connection.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin migration %s: %w", name, err)
	}
	defer transaction.Rollback()

	var applied bool
	if err := transaction.QueryRowContext(
		ctx,
		"SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version = $1)",
		name,
	).Scan(&applied); err != nil {
		return fmt.Errorf("check migration %s: %w", name, err)
	}
	if applied {
		return transaction.Commit()
	}

	body, err := migrationFiles.ReadFile("migrations/" + name)
	if err != nil {
		return fmt.Errorf("read migration %s: %w", name, err)
	}
	if len(strings.TrimSpace(string(body))) == 0 {
		return fmt.Errorf("migration %s is empty", name)
	}

	if _, err := transaction.ExecContext(ctx, string(body)); err != nil {
		return fmt.Errorf("execute migration %s: %w", name, err)
	}
	if _, err := transaction.ExecContext(
		ctx,
		"INSERT INTO schema_migrations (version) VALUES ($1)",
		name,
	); err != nil {
		return fmt.Errorf("record migration %s: %w", name, err)
	}

	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("commit migration %s: %w", name, err)
	}
	return nil
}
