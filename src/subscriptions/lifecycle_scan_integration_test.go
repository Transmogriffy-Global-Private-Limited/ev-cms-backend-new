package subscriptions

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/constants"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/testsupport"
	"github.com/google/uuid"
)

func TestLifecycleCursorProcessesBeyondFirstFiveHundred(t *testing.T) {
	database := repairTestDatabase(t)
	now := time.Now().UTC().Truncate(time.Microsecond)
	principal := repairTestActor(t, database, now)
	svc := NewService(database, nil)
	svc.now = func() time.Time { return now }
	version := repairTestPlan(t, svc, principal)
	const total = 525
	cpos := make([]models.CPO, 0, total)
	subs := make([]models.CPOSubscription, 0, total)
	ids := make([]uuid.UUID, 0, total)
	for i := 0; i < total; i++ {
		cpo := models.CPO{ID: uuid.New(), Slug: "scan-" + uuid.NewString()[:8], BusinessName: fmt.Sprintf("Scan CPO %d", i), CompanyType: constants.CPOCompanyTypeCompany, GSTIN: testsupport.ValidGSTIN("19"), Address: "Test Road", City: "Kolkata", State: "West Bengal", Pincode: "700001", Status: constants.CPOStatusActive, StatusReason: "Fixture", StatusChangedAt: now, AppID: "cpo_dummy_" + uuid.NewString(), AppIDMode: constants.CPOAppIDModeDummy, AppIDUpdatedAt: now, CreatedAt: now, UpdatedAt: now}
		cpos = append(cpos, cpo)
		ids = append(ids, cpo.ID)
		subs = append(subs, models.CPOSubscription{ID: uuid.New(), CPOID: cpo.ID, PlanVersionID: version, Status: "ACTIVE", StartsAt: now.AddDate(0, -1, 0), CurrentPeriodStartsAt: now.AddDate(0, -1, 0), CurrentPeriodEndsAt: now.Add(-time.Hour), CurrentPeriodSequence: 1, CreatedBy: principal.UserID, CreatedAt: now, UpdatedAt: now})
	}
	if err := database.CreateInBatches(cpos, 100).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.CreateInBatches(subs, 100).Error; err != nil {
		t.Fatal(err)
	}
	// Allow a few cycles for other eligible fixture records from the same
	// disposable database; cursor persists across every cycle and wraps.
	for pass := 0; pass < 8; pass++ {
		if err := svc.processLifecycle(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	var expired int64
	if err := database.Model(&models.CPOSubscription{}).Where("cpo_id IN ? AND status = ?", ids, "EXPIRED").Count(&expired).Error; err != nil {
		t.Fatal(err)
	}
	if expired != total {
		t.Fatalf("only %d/%d subscriptions expired", expired, total)
	}
	var eventCount int64
	if err := database.Model(&models.CPOSubscriptionLifecycleEvent{}).Where("subscription_id IN ? AND kind = ?", subscriptionIDs(subs), "EXPIRED").Count(&eventCount).Error; err != nil {
		t.Fatal(err)
	}
	if eventCount != total {
		t.Fatalf("expiry event count %d, expected %d", eventCount, total)
	}
	var progress lifecycleScanCursor
	if err := database.First(&progress, "worker_name = ?", lifecycleWorkerName).Error; err != nil {
		t.Fatal(err)
	}
	if progress.ProcessedTotal < total {
		t.Fatalf("cursor progress %d less than %d", progress.ProcessedTotal, total)
	}
}

func subscriptionIDs(subs []models.CPOSubscription) []uuid.UUID {
	ids := make([]uuid.UUID, 0, len(subs))
	for _, s := range subs {
		ids = append(ids, s.ID)
	}
	return ids
}
