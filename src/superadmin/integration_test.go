package superadmin

import (
	"context"
	"fmt"
	"os"
	"sort"
	"testing"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/db"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/auth"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/constants"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
	"github.com/google/uuid"
)

func TestListAdministratorsWithPostgreSQL(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}

	ctx := context.Background()
	database, sqlDB, err := db.Open(ctx, databaseURL)
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	defer sqlDB.Close()
	if err := db.ApplyMigrations(ctx, sqlDB); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	now := time.Date(2200, time.August, 11, 12, 0, 0, 0, time.UTC)
	ids := []uuid.UUID{uuid.New(), uuid.New()}
	sort.Slice(ids, func(i, j int) bool { return ids[i].String() < ids[j].String() })
	lowID, highID := ids[0], ids[1]
	newestID, inactiveID := uuid.New(), uuid.New()

	seed := func(id uuid.UUID, email, fullName string, identityActive, identityVerified, authorityActive bool, createdAt time.Time) {
		t.Helper()
		user := models.User{
			ID: id, Email: email, PasswordHash: "test-password-hash", FullName: fullName,
			IsActive: identityActive, IsVerified: identityVerified, PasswordChangedAt: now,
			CreatedAt: createdAt, UpdatedAt: createdAt,
		}
		if err := database.Create(&user).Error; err != nil {
			t.Fatalf("seed user %s: %v", email, err)
		}
		admin := models.PlatformAdmin{
			UserID: id, IsActive: authorityActive, StatusReason: "test authority",
			StatusChangedAt: createdAt, CreatedAt: createdAt, UpdatedAt: createdAt,
		}
		if err := database.Create(&admin).Error; err != nil {
			t.Fatalf("seed platform administrator %s: %v", email, err)
		}
	}

	unique := uuid.NewString()
	seed(newestID, fmt.Sprintf("newest-%s@example.com", unique), "Newest", true, true, true, now.Add(time.Hour))
	seed(highID, fmt.Sprintf("high-%s@example.com", unique), "High", true, false, true, now)
	seed(lowID, fmt.Sprintf("low-%s@example.com", unique), "Low", false, true, true, now)
	seed(inactiveID, fmt.Sprintf("inactive-%s@example.com", unique), "Inactive", true, false, false, now.Add(-time.Hour))

	service := NewService(database, nil, nil, false)
	principal := auth.Principal{Scope: constants.AuthScopePlatform}
	activePage, err := service.ListAdministrators(ctx, principal, AdministratorQuery{PageQuery: PageQuery{Limit: 2}})
	if err != nil {
		t.Fatalf("list active platform administrators: %v", err)
	}
	if !activePage.HasMore || len(activePage.Administrators) != 2 {
		t.Fatalf("active page = %+v, want two rows with another page", activePage)
	}
	if activePage.Administrators[0].UserID != newestID || activePage.Administrators[1].UserID != highID {
		t.Fatalf("active order = [%s, %s], want [%s, %s]", activePage.Administrators[0].UserID, activePage.Administrators[1].UserID, newestID, highID)
	}
	if activePage.Administrators[1].Email != fmt.Sprintf("high-%s@example.com", unique) || activePage.Administrators[1].FullName != "High" || activePage.Administrators[1].IdentityVerified {
		t.Fatalf("joined user projection = %+v, want High user projection", activePage.Administrators[1])
	}
	if activePage.NextBefore == nil || activePage.NextBeforeID == nil {
		t.Fatal("active page omitted next cursor")
	}

	nextPage, err := service.ListAdministrators(ctx, principal, AdministratorQuery{PageQuery: PageQuery{Limit: 2, Before: activePage.NextBefore, BeforeID: activePage.NextBeforeID}})
	if err != nil {
		t.Fatalf("list active platform administrators after cursor: %v", err)
	}
	if nextPage.HasMore || len(nextPage.Administrators) != 1 || nextPage.Administrators[0].UserID != lowID {
		t.Fatalf("next active page = %+v, want only lower same-timestamp administrator", nextPage)
	}
	if nextPage.Administrators[0].IdentityActive || !nextPage.Administrators[0].IdentityVerified {
		t.Fatalf("next joined identity projection = %+v, want inactive verified identity", nextPage.Administrators[0])
	}

	allPage, err := service.ListAdministrators(ctx, principal, AdministratorQuery{PageQuery: PageQuery{Limit: 10}, IncludeInactive: true})
	if err != nil {
		t.Fatalf("list all platform administrators: %v", err)
	}
	if allPage.HasMore || len(allPage.Administrators) != 4 || allPage.Administrators[3].UserID != inactiveID || allPage.Administrators[3].AuthorityActive {
		t.Fatalf("include-inactive page = %+v, want all four administrators including inactive authority", allPage)
	}
}
