package customerauth

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	cmsmiddleware "github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/middleware"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type capturedChargingController struct {
	startCorrelationID string
	stopCorrelationID  string
}

func (controller *capturedChargingController) StartCharging(_ context.Context, _ Principal, _ ChargingStartRequest, correlationID string) (ChargingStartResponse, error) {
	controller.startCorrelationID = correlationID
	return ChargingStartResponse{}, nil
}

func (controller *capturedChargingController) StopCharging(_ context.Context, _ Principal, _ uuid.UUID, _ ChargingStopRequest, correlationID string) error {
	controller.stopCorrelationID = correlationID
	return nil
}

func TestChargingHandlersUseCanonicalMiddlewareRequestID(t *testing.T) {
	for _, test := range []struct {
		name            string
		path            string
		clientRequestID string
		correlationID   func(*capturedChargingController) string
	}{
		{
			name:          "start without client request ID",
			path:          "/charging",
			correlationID: func(controller *capturedChargingController) string { return controller.startCorrelationID },
		},
		{
			name:          "stop without client request ID",
			path:          "/charging/" + uuid.NewString() + "/stop",
			correlationID: func(controller *capturedChargingController) string { return controller.stopCorrelationID },
		},
		{
			name:            "client request ID does not replace CMS identity",
			path:            "/charging",
			clientRequestID: "client-supplied-id",
			correlationID:   func(controller *capturedChargingController) string { return controller.startCorrelationID },
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			controller := &capturedChargingController{}
			handler := &Handler{charging: controller}
			principal := Principal{CPOID: uuid.New(), CustomerID: uuid.New()}
			router := gin.New()
			router.Use(cmsmiddleware.RequestLogger(io.Discard, false))
			router.POST("/charging", func(ctx *gin.Context) {
				ctx.Set(principalContextKey, principal)
				handler.startCharging(ctx)
			})
			router.POST("/charging/:session_id/stop", func(ctx *gin.Context) {
				ctx.Set(principalContextKey, principal)
				handler.stopCharging(ctx)
			})

			request := httptest.NewRequest(http.MethodPost, test.path, strings.NewReader(`{}`))
			request.Header.Set("Content-Type", "application/json")
			if test.clientRequestID != "" {
				request.Header.Set(cmsmiddleware.RequestIDHeader, test.clientRequestID)
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)

			if response.Code != http.StatusAccepted {
				t.Fatalf("status = %d, want %d", response.Code, http.StatusAccepted)
			}
			canonicalID := response.Header().Get(cmsmiddleware.RequestIDHeader)
			correlationID := test.correlationID(controller)
			if canonicalID == "" || correlationID != canonicalID {
				t.Fatalf("service correlation ID = %q, response request ID = %q", correlationID, canonicalID)
			}
			if test.clientRequestID != "" && correlationID == test.clientRequestID {
				t.Fatalf("service correlation ID trusted client request ID %q", test.clientRequestID)
			}
		})
	}
}

func TestChargingRequestCorrelationFallsBackToServerGeneratedIdentity(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/charging", nil)
	ctx.Request.Header.Set(cmsmiddleware.RequestIDHeader, "client-supplied-id")

	if correlationID := requestCorrelationID(ctx); correlationID == "" || correlationID == "client-supplied-id" {
		t.Fatalf("fallback correlation ID = %q", correlationID)
	}
}
