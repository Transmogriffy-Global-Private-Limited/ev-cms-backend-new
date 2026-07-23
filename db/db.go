package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"os"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func Open(ctx context.Context, databaseURL string) (*gorm.DB, *sql.DB, error) {
	if databaseURL == "" {
		return nil, nil, errors.New("DATABASE_URL is required")
	}

	databaseLogger := logger.New(
		log.New(os.Stderr, "database ", log.LstdFlags),
		logger.Config{
			SlowThreshold:             time.Second,
			LogLevel:                  logger.Warn,
			IgnoreRecordNotFoundError: true,
			ParameterizedQueries:      true,
			Colorful:                  false,
		},
	)
	gormDB, err := gorm.Open(postgres.Open(databaseURL), &gorm.Config{
		Logger: databaseLogger,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("open PostgreSQL connection: %w", err)
	}

	sqlDB, err := gormDB.DB()
	if err != nil {
		return nil, nil, fmt.Errorf("access PostgreSQL connection pool: %w", err)
	}

	sqlDB.SetMaxOpenConns(20)
	sqlDB.SetMaxIdleConns(5)
	sqlDB.SetConnMaxIdleTime(5 * time.Minute)
	sqlDB.SetConnMaxLifetime(30 * time.Minute)

	pingContext, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := sqlDB.PingContext(pingContext); err != nil {
		_ = sqlDB.Close()
		return nil, nil, fmt.Errorf("ping PostgreSQL: %w", err)
	}

	return gormDB, sqlDB, nil
}
