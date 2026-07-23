package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/db"
	"github.com/joho/godotenv"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	if err := godotenv.Load(); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("load .env: %w", err)
	}

	direction := flag.String("direction", "up", "migration direction: up or down")
	flag.Parse()

	ctx, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stop()

	_, sqlDB, err := db.Open(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		return err
	}
	defer sqlDB.Close()

	switch *direction {
	case "up":
		if err := db.ApplyMigrations(ctx, sqlDB); err != nil {
			return fmt.Errorf("apply migrations: %w", err)
		}
	case "down":
		if err := db.RollbackLastMigration(ctx, sqlDB); err != nil {
			return fmt.Errorf("roll back latest migration: %w", err)
		}
	default:
		return fmt.Errorf("unsupported migration direction %q", *direction)
	}

	log.Printf("migration direction %s completed", *direction)
	return nil
}
