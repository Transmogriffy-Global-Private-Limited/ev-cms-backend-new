package chargingtrace

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func TestEnvelopeDigestIsStableAndExcludesItsOwnDigest(t *testing.T) {
	envelope := testEnvelope(t)
	first, err := Digest(envelope)
	if err != nil {
		t.Fatal(err)
	}
	envelope.ImmutableContentSHA256 = "different-but-excluded"
	second, err := Digest(envelope)
	if err != nil {
		t.Fatal(err)
	}
	if first != second || len(first) != 64 {
		t.Fatalf("digest first=%q second=%q", first, second)
	}
}

func TestTraceIngressUsesDedicatedBearerAndStrictJSONBoundary(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	RegisterIngressRoutes(router.Group("/v1"), NewIngestor(nil, "trace-bearer"))
	for _, testcase := range []struct {
		name, bearer string
		want         int
		unknown      bool
	}{
		{"missing", "", http.StatusUnauthorized, false},
		{"fact bearer is not trace bearer", "fact-bearer", http.StatusUnauthorized, false},
		{"trace bearer reaches storage boundary", "trace-bearer", http.StatusInternalServerError, false},
		{"unknown field", "trace-bearer", http.StatusBadRequest, true},
	} {
		t.Run(testcase.name, func(t *testing.T) {
			envelope := testEnvelope(t)
			body, err := json.Marshal(envelope)
			if err != nil {
				t.Fatal(err)
			}
			if testcase.unknown {
				body = []byte(strings.TrimSuffix(string(body), "}") + `,"unexpected":true}`)
			}
			recording := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, "/v1/hal-trace-events", bytes.NewReader(body))
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Idempotency-Key", envelope.EventID.String())
			if testcase.bearer != "" {
				request.Header.Set("Authorization", "Bearer "+testcase.bearer)
			}
			router.ServeHTTP(recording, request)
			if recording.Code != testcase.want {
				t.Fatalf("status=%d body=%s", recording.Code, recording.Body.String())
			}
		})
	}
}

func TestTraceIngressSanitizesNetworkDataAllowlist(t *testing.T) {
	data := sanitize(models.JSONB{"meter_wh": int64(101028), "status": "Available", "connector_id": 1, "id_tag": "never", "authorization": "never"})
	if data["meter_wh"] != int64(101028) || data["status"] != "Available" || data["connector_id"] != 1 {
		t.Fatalf("safe data=%#v", data)
	}
	if _, ok := data["id_tag"]; ok {
		t.Fatalf("unsafe id_tag retained: %#v", data)
	}
	if _, ok := data["authorization"]; ok {
		t.Fatalf("unsafe authorization retained: %#v", data)
	}
}

func testEnvelope(t *testing.T) Envelope {
	t.Helper()
	ocpp := int64(654321)
	envelope := Envelope{
		SchemaVersion:       1,
		TraceID:             uuid.MustParse("2d2e4e72-859d-45c5-92dc-56fb9c7c2e25"),
		EventID:             uuid.New(),
		CPOID:               uuid.MustParse("02a62d77-bbe9-4d52-9bdf-1e3e4d82b4cf"),
		OCPPTransactionID:   &ocpp,
		ChargerOCPPIdentity: "CP-01",
		OCPPConnectorNumber: 1,
		Source:              "CHARGER",
		Target:              "HAL",
		Category:            "METER",
		Protocol:            "OCPP1.6",
		Phase:               "CHARGING",
		Summary:             "Accepted transaction meter observation",
		OccurredAt:          time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC),
		Data:                models.JSONB{"meter_wh": int64(101028)},
	}
	digest, err := Digest(envelope)
	if err != nil {
		t.Fatal(err)
	}
	envelope.ImmutableContentSHA256 = digest
	return envelope
}
