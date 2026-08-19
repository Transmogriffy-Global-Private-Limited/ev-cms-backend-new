package halclient

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/config"
	"github.com/google/uuid"
)

func TestMutationsSendCMSCorrelationAndIdempotencyHeaders(t *testing.T) {
	var requestCount atomic.Int32
	var correlationID, idempotencyKey string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requestCount.Add(1)
		correlationID = request.Header.Get("X-Correlation-ID")
		idempotencyKey = request.Header.Get("Idempotency-Key")
		writer.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := New(config.HAL{BaseURL: server.URL, CMSBearerToken: "test"})
	chargerID := uuid.New()
	if err := client.SyncMapping(context.Background(), ChargerMapping{
		CPOID: uuid.New(), CMSChargerID: chargerID, ChargerOCPPIdentity: "charger-1", Enabled: true,
		Connectors: []ConnectorMapping{{CMSConnectorID: uuid.New(), OCPPConnectorNumber: 1}},
	}, "cms-request-id"); err != nil {
		t.Fatalf("sync mapping: %v", err)
	}
	if requestCount.Load() != 1 || correlationID != "cms-request-id" || idempotencyKey != chargerID.String() {
		t.Fatalf("headers/calls = correlation %q idempotency %q calls %d", correlationID, idempotencyKey, requestCount.Load())
	}
}

func TestGetTransactionByStartIntentDecodesAuthoritativeHALTruth(t *testing.T) {
	transactionID, intentID, commandID := uuid.New(), uuid.New(), uuid.New()
	cpoID, chargerID, connectorID := uuid.New(), uuid.New(), uuid.New()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/transactions" || request.URL.Query().Get("cms_start_intent_id") != intentID.String() {
			t.Fatalf("unexpected lookup %s", request.URL.String())
		}
		if request.Header.Get("Authorization") != "Bearer test" {
			t.Fatal("missing HAL service authentication")
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"transaction":{"hal_transaction_id":"` + transactionID.String() + `","cms_start_intent_id":"` + intentID.String() + `","cms_command_id":"` + commandID.String() + `","cpo_id":"` + cpoID.String() + `","cms_charger_id":"` + chargerID.String() + `","cms_connector_id":"` + connectorID.String() + `","charger_ocpp_identity":"charger-1","ocpp_connector_number":1,"ocpp_transaction_id":42,"actual_started_at":"2026-08-19T10:00:00Z","meter_start_wh":100}}`))
	}))
	defer server.Close()

	transaction, err := New(config.HAL{BaseURL: server.URL, CMSBearerToken: "test", RequestTimeout: time.Second}).GetTransactionByStartIntent(context.Background(), intentID)
	if err != nil {
		t.Fatal(err)
	}
	if transaction.HALTransactionID != transactionID || transaction.CMSStartIntentID != intentID || transaction.CMSCommandID != commandID || transaction.OCPPTransactionID != 42 || transaction.MeterStartWh != 100 {
		t.Fatalf("decoded transaction = %#v", transaction)
	}
}

func TestMutationsRejectEmptyCorrelationBeforeSendingHTTP(t *testing.T) {
	var requestCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		requestCount.Add(1)
	}))
	defer server.Close()

	client := New(config.HAL{BaseURL: server.URL, CMSBearerToken: "test"})
	for _, test := range []struct {
		name   string
		mutate func() error
	}{
		{
			name: "mapping",
			mutate: func() error {
				return client.SyncMapping(context.Background(), ChargerMapping{CMSChargerID: uuid.New()}, " ")
			},
		},
		{
			name: "start",
			mutate: func() error {
				_, err := client.Start(context.Background(), StartCommand{CMSCommandID: uuid.New()}, "")
				return err
			},
		},
		{
			name: "stop",
			mutate: func() error {
				_, err := client.Stop(context.Background(), StopCommand{CMSCommandID: uuid.New()}, "")
				return err
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := test.mutate(); !errors.Is(err, ErrMissingCorrelationID) {
				t.Fatalf("error = %v, want ErrMissingCorrelationID", err)
			}
		})
	}
	if requestCount.Load() != 0 {
		t.Fatalf("HTTP mutations = %d, want 0", requestCount.Load())
	}
}
