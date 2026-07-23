package db

import (
	"context"
	"strings"
	"testing"
)

func TestOpenRequiresDatabaseURL(t *testing.T) {
	t.Parallel()

	gormDB, sqlDB, err := Open(context.Background(), "")
	if err == nil || !strings.Contains(err.Error(), "DATABASE_URL") {
		t.Fatalf("got error %v, want missing DATABASE_URL error", err)
	}
	if gormDB != nil || sqlDB != nil {
		t.Fatal("missing DATABASE_URL must not return database handles")
	}
}
