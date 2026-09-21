package customerauth

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func TestNormalizeCustomerSessionRatingRequest(t *testing.T) {
	one, five := 1, 5
	for _, request := range []CustomerSessionRatingRequest{
		{OverallRating: 1},
		{OverallRating: 5, StationRating: &one, ChargerRating: &five},
	} {
		if _, err := normalizeCustomerSessionRatingRequest(request); err != nil {
			t.Fatalf("valid request rejected: %v", err)
		}
	}
	for _, test := range []struct {
		request CustomerSessionRatingRequest
		code    string
	}{
		{CustomerSessionRatingRequest{OverallRating: 0}, "invalid_overall_rating"},
		{CustomerSessionRatingRequest{OverallRating: 6}, "invalid_overall_rating"},
		{CustomerSessionRatingRequest{OverallRating: 4, StationRating: intPointerForRatingTest(0)}, "invalid_station_rating"},
		{CustomerSessionRatingRequest{OverallRating: 4, ChargerRating: intPointerForRatingTest(6)}, "invalid_charger_rating"},
		{CustomerSessionRatingRequest{OverallRating: 4, Reason: stringPointerForRatingTest(strings.Repeat("a", customerRatingReasonMaxRunes+1))}, "invalid_reason"},
	} {
		_, err := normalizeCustomerSessionRatingRequest(test.request)
		apiError, ok := err.(*APIError)
		if !ok || apiError.Status != http.StatusBadRequest || apiError.Code != test.code {
			t.Errorf("request=%+v error=%v, want 400 %s", test.request, err, test.code)
		}
	}
	request, err := normalizeCustomerSessionRatingRequest(CustomerSessionRatingRequest{OverallRating: 4, Reason: stringPointerForRatingTest(" \t\n ")})
	if err != nil || request.Reason != nil {
		t.Fatalf("blank reason was not canonicalized: %+v err=%v", request, err)
	}
}

func TestCustomerSessionRatingBodyIsStrict(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodPut, "/", bytes.NewBufferString(`{"overall_rating":5,"customer_id":"`+uuid.NewString()+`"}`))
	context.Set(principalContextKey, Principal{CPOID: uuid.New(), CustomerID: uuid.New()})
	context.Params = gin.Params{{Key: "session_id", Value: uuid.NewString()}}
	(&Handler{}).putChargingSessionRating(context)
	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), `"code":"invalid_request"`) {
		t.Fatalf("unknown-field response status=%d body=%s", recorder.Code, recorder.Body.String())
	}

	recorder = httptest.NewRecorder()
	context, _ = gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodPut, "/", bytes.NewBufferString(`{"overall_rating":5}{}`))
	context.Set(principalContextKey, Principal{CPOID: uuid.New(), CustomerID: uuid.New()})
	context.Params = gin.Params{{Key: "session_id", Value: uuid.NewString()}}
	(&Handler{}).putChargingSessionRating(context)
	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), `"code":"invalid_request"`) {
		t.Fatalf("multiple-body response status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestCustomerSessionRatingAppIDGuard(t *testing.T) {
	gin.SetMode(gin.TestMode)
	principal := Principal{CPOID: uuid.New(), CustomerID: uuid.New(), CPOAppID: "cpo_rating_test"}
	for _, test := range []struct {
		name   string
		appID  string
		status int
		code   string
	}{
		{name: "missing", status: http.StatusBadRequest, code: "missing_cpo_app_id"},
		{name: "wrong", appID: "cpo_other", status: http.StatusForbidden, code: "cpo_app_id_mismatch"},
	} {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			context, _ := gin.CreateTestContext(recorder)
			context.Request = httptest.NewRequest(http.MethodPut, "/", nil)
			if test.appID != "" {
				context.Request.Header.Set(CPOAppIDHeader, test.appID)
			}
			context.Set(principalContextKey, principal)
			RequireAppID()(context)
			var response struct {
				Error struct {
					Code string `json:"code"`
				} `json:"error"`
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil || recorder.Code != test.status || response.Error.Code != test.code || !context.IsAborted() {
				t.Fatalf("status=%d body=%s aborted=%t err=%v", recorder.Code, recorder.Body.String(), context.IsAborted(), err)
			}
		})
	}
}

func intPointerForRatingTest(value int) *int          { return &value }
func stringPointerForRatingTest(value string) *string { return &value }
