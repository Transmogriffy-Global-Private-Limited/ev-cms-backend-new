package cpo

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/auth"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/constants"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type customerRatingsRepositoryStub struct {
	Repository
	cpoID uuid.UUID
	query CustomerRatingListQuery
	rows  []models.CustomerRating
	err   error
}

func (r *customerRatingsRepositoryStub) ListCustomerRatings(
	_ context.Context,
	cpoID uuid.UUID,
	query CustomerRatingListQuery,
) ([]models.CustomerRating, error) {
	r.cpoID, r.query = cpoID, query
	return r.rows, r.err
}

func parseCustomerRatingsForTest(t *testing.T, rawQuery string) (CustomerRatingListQuery, bool, *httptest.ResponseRecorder) {
	t.Helper()
	writer := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(writer)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/v1/cpo/customer-ratings?"+rawQuery, nil)
	query, ok := parseCustomerRatingListQuery(ctx)
	return query, ok, writer
}

func TestParseCustomerRatingListQuery(t *testing.T) {
	gin.SetMode(gin.TestMode)
	id := uuid.New()
	query, ok, recorder := parseCustomerRatingsForTest(t,
		"limit=25&before=2026-09-17T12:00:00Z&before_id="+id.String()+
			"&customer_id="+id.String()+"&charger_id="+id.String()+"&hub_id="+id.String()+
			"&session_id="+id.String()+"&min_overall_rating=2&max_overall_rating=5&has_review=true&min_station_rating=2&max_station_rating=4&min_charger_rating=3&max_charger_rating=5&sort_by=station_rating&sort_order=asc&cursor_value=4&cursor_id="+id.String())
	if !ok || recorder.Code != http.StatusOK || query.Limit != 25 || query.Before == nil || query.BeforeID == nil {
		t.Fatalf("query=%+v ok=%t status=%d body=%s", query, ok, recorder.Code, recorder.Body.String())
	}
	if query.CustomerID == nil || *query.CustomerID != id || query.ChargerID == nil || *query.ChargerID != id ||
		query.HubID == nil || *query.HubID != id || query.SessionID == nil || *query.SessionID != id ||
		query.MinOverall == nil || *query.MinOverall != 2 || query.MaxOverall == nil || *query.MaxOverall != 5 {
		t.Fatalf("typed filters were not parsed: %+v", query)
	}
	if query.HasReview == nil || !*query.HasReview || query.MinStation == nil || *query.MinStation != 2 || query.MaxCharger == nil || *query.MaxCharger != 5 || query.CursorID == nil || query.SortBy != "station_rating" {
		t.Fatalf("extended filters were not parsed: %+v", query)
	}

	for _, raw := range []string{
		"before_id=" + id.String(),
		"before=nope&before_id=" + id.String(),
		"customer_id=bad",
		"charger_id=00000000-0000-0000-0000-000000000000",
		"limit=lots",
		"max_overall_rating=x",
	} {
		if _, ok, recorder := parseCustomerRatingsForTest(t, raw); ok || recorder.Code != http.StatusBadRequest {
			t.Errorf("query %q: ok=%t status=%d, want false/400", raw, ok, recorder.Code)
		}
	}
}

func TestListCustomerRatingsValidatesAndBuildsPage(t *testing.T) {
	cpoID := uuid.New()
	customerID, chargerID := uuid.New(), uuid.New()
	createdOne := time.Date(2026, 9, 17, 10, 0, 0, 0, time.UTC)
	createdTwo := createdOne.Add(time.Minute)
	rows := []models.CustomerRating{
		{ID: uuid.New(), CPOID: cpoID, CustomerID: customerID, Customer: models.Customer{ID: customerID, FullName: "A Customer", Email: "a@example.test"}, ChargerID: chargerID, Charger: models.Charger{ID: chargerID, ChargerID: "ab1234", ChargerName: "North DC"}, OverallRating: 4, CreatedAt: createdTwo, UpdatedAt: createdTwo},
		{ID: uuid.New(), CPOID: cpoID, CustomerID: customerID, ChargerID: chargerID, OverallRating: 5, CreatedAt: createdOne, UpdatedAt: createdOne},
		{ID: uuid.New(), CPOID: cpoID, CustomerID: customerID, ChargerID: chargerID, OverallRating: 3, CreatedAt: createdOne.Add(-time.Minute), UpdatedAt: createdOne.Add(-time.Minute)},
	}
	repository := &customerRatingsRepositoryStub{rows: rows}
	service := &Service{repository: repository}
	principal := auth.Principal{Scope: constants.AuthScopeCPO, CPOID: &cpoID}

	for _, query := range []CustomerRatingListQuery{
		{Limit: maxListLimit + 1},
		{Before: &createdOne},
		{MinOverall: intPointerForTest(0)},
		{MaxOverall: intPointerForTest(6)},
		{MinOverall: intPointerForTest(5), MaxOverall: intPointerForTest(4)},
	} {
		if _, err := service.ListCustomerRatings(context.Background(), principal, query); err == nil {
			t.Errorf("invalid query accepted: %+v", query)
		}
	}

	response, err := service.ListCustomerRatings(context.Background(), principal, CustomerRatingListQuery{Limit: 2})
	if err != nil || len(response.Ratings) != 2 || !response.HasMore || response.NextBefore == nil || response.NextBeforeID == nil {
		t.Fatalf("response=%+v err=%v", response, err)
	}
	if repository.cpoID != cpoID || repository.query.Limit != 2 || response.Ratings[0].CustomerName != "A Customer" ||
		response.Ratings[0].CustomerEmail != "a@example.test" || response.Ratings[0].ChargerCode != "ab1234" ||
		!response.NextBefore.Equal(createdOne) || *response.NextBeforeID != rows[1].ID {
		t.Fatalf("tenant/query/page projection mismatch: repo=%+v response=%+v", repository, response)
	}
}

func intPointerForTest(value int) *int { return &value }
