package support

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/auth"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func TestSupportStatusTransitionGraph(t *testing.T) {
	t.Parallel()
	cases := []struct {
		from string
		to   string
		want bool
	}{
		{"OPEN", "OPEN", true},
		{"OPEN", "IN_PROGRESS", true},
		{"OPEN", "RESOLVED", true},
		{"OPEN", "CLOSED", true},
		{"IN_PROGRESS", "OPEN", false},
		{"IN_PROGRESS", "RESOLVED", true},
		{"IN_PROGRESS", "CLOSED", true},
		{"RESOLVED", "OPEN", true},
		{"RESOLVED", "IN_PROGRESS", false},
		{"RESOLVED", "CLOSED", true},
		{"CLOSED", "OPEN", true},
		{"CLOSED", "IN_PROGRESS", false},
		{"UNKNOWN", "OPEN", false},
	}
	for _, test := range cases {
		t.Run(test.from+"_to_"+test.to, func(t *testing.T) {
			if got := validTransition(test.from, test.to); got != test.want {
				t.Fatalf("validTransition(%q, %q) = %t, want %t", test.from, test.to, got, test.want)
			}
		})
	}
}

func TestNormalizeSupportListQuery(t *testing.T) {
	t.Parallel()
	before := time.Date(2026, 8, 26, 10, 0, 0, 0, time.UTC)
	id := uuid.New()
	query, err := normalizeListQuery(ListQuery{Before: &before, BeforeID: &id, Status: " resolved ", Search: " reference "})
	if err != nil {
		t.Fatalf("normalizeListQuery valid query: %v", err)
	}
	if query.Limit != 20 || query.Status != "RESOLVED" || query.Search != "reference" {
		t.Fatalf("normalized query = %#v", query)
	}
	for _, query := range []ListQuery{
		{Limit: 101},
		{Before: &before},
		{Status: "PENDING"},
		{Search: strings.Repeat("x", 101)},
	} {
		if _, err := normalizeListQuery(query); err == nil {
			t.Fatalf("normalizeListQuery(%#v) accepted invalid query", query)
		}
	}
}

func TestDecodeSupportRequestStrictly(t *testing.T) {
	gin.SetMode(gin.TestMode)
	valid := `{"subject":"Charging question","body":"Please help."}`
	for _, body := range []string{
		`{"subject":"Question","body":"Help","unexpected":true}`,
		`[{"subject":"Question","body":"Help"}]`,
		`null`,
		valid + valid,
		strings.Repeat("x", 32*1024+1),
	} {
		recorder := httptest.NewRecorder()
		context, _ := gin.CreateTestContext(recorder)
		context.Request = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
		var request CreateRequest
		if decode(context, &request) {
			t.Fatalf("decode accepted invalid body %q", body[:min(len(body), 64)])
		}
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("invalid body returned %d, want %d", recorder.Code, http.StatusBadRequest)
		}
	}
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(valid))
	var request CreateRequest
	if !decode(context, &request) || request.Subject != "Charging question" || request.Body != "Please help." {
		t.Fatalf("decode rejected or corrupted one valid JSON object: %#v", request)
	}
}

func TestSupportListQueryParsesCursorAndFilters(t *testing.T) {
	gin.SetMode(gin.TestMode)
	before := "2026-08-26T10:00:00Z"
	id := uuid.New()
	cpoID := uuid.New()
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodGet, "/?limit=25&before="+before+"&before_id="+id.String()+"&status=in_progress&cpo_id="+cpoID.String()+"&q=charger", nil)
	query, err := listQuery(context)
	if err != nil {
		t.Fatalf("listQuery: %v", err)
	}
	if query.Limit != 25 || query.Before == nil || query.BeforeID == nil || query.Status != "IN_PROGRESS" || query.CPOID == nil || query.Search != "charger" {
		t.Fatalf("parsed query = %#v", query)
	}

	recorder = httptest.NewRecorder()
	context, _ = gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodGet, "/?before="+before, nil)
	_, err = listQuery(context)
	apiError, ok := err.(*auth.APIError)
	if !ok || apiError.Code != "invalid_cursor" {
		t.Fatalf("one-sided cursor error = %#v, want invalid_cursor", err)
	}
}

func min(left, right int) int {
	if left < right {
		return left
	}
	return right
}
