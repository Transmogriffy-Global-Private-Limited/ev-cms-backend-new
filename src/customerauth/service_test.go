package customerauth

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func TestNormalizePhone(t *testing.T) {
	t.Parallel()
	valid := "+919876543210"
	normalized, err := normalizePhone(&valid)
	if err != nil || normalized == nil || *normalized != valid {
		t.Fatalf("normalize valid phone: value=%v err=%v", normalized, err)
	}
	invalid := "9876 abc"
	if _, err := normalizePhone(&invalid); err == nil {
		t.Fatal("expected invalid phone to fail")
	}
}

func TestCustomerPrincipalHelpersAndAppIDGuard(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)
	userID := uuid.New()
	customerID := uuid.New()
	cpoID := uuid.New()
	principal := Principal{
		UserID: userID, CustomerID: customerID, CPOID: cpoID,
		CPOAppID: "cpo_dummy_1234567890abcdef",
	}
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	context.Request.Header.Set(CPOAppIDHeader, principal.CPOAppID)
	context.Set(principalContextKey, principal)

	if got, ok := CurrentUserID(context); !ok || got != userID {
		t.Fatalf("CurrentUserID=(%v,%v)", got, ok)
	}
	if got, ok := CurrentCustomerID(context); !ok || got != customerID {
		t.Fatalf("CurrentCustomerID=(%v,%v)", got, ok)
	}
	if got, ok := CurrentCPOID(context); !ok || got != cpoID {
		t.Fatalf("CurrentCPOID=(%v,%v)", got, ok)
	}
	if got, ok := CurrentCPOAppID(context); !ok || got != principal.CPOAppID {
		t.Fatalf("CurrentCPOAppID=(%q,%v)", got, ok)
	}
	RequireAppID()(context)
	if context.IsAborted() {
		t.Fatalf("matching app ID was rejected: %s", recorder.Body.String())
	}
}

func TestSignupValidators(t *testing.T) {
	t.Parallel()
	if !validEmail("person@example.com") {
		t.Fatal("expected valid email")
	}
	if validEmail("Person <person@example.com>") {
		t.Fatal("expected display-name email to fail")
	}
	if !validOTP("123456") || validOTP("12345x") {
		t.Fatal("OTP validation does not enforce six digits")
	}
}
