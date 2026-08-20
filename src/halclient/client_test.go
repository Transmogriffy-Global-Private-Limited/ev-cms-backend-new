package halclient

import (
	"context"
	"encoding/json"
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

func TestRequeueFactUsesExactAuthenticatedRecoveryContract(t *testing.T) {
	factID, correlationID := uuid.New(), uuid.New().String()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.Path != "/v1/facts/"+factID.String()+"/requeue" || request.Header.Get("Authorization") != "Bearer test" || request.Header.Get("X-Correlation-ID") != correlationID {
			t.Fatalf("unexpected requeue request: method=%s path=%s", request.Method, request.URL.Path)
		}
		_ = json.NewEncoder(writer).Encode(map[string]any{"fact_id": factID, "status": "PENDING"})
	}))
	defer server.Close()
	client := New(config.HAL{BaseURL: server.URL, CMSBearerToken: "test", RequestTimeout: time.Second})
	if err := client.RequeueFact(context.Background(), factID, correlationID); err != nil {
		t.Fatal(err)
	}
	if err := client.RequeueFact(context.Background(), uuid.Nil, correlationID); !errors.Is(err, ErrInvalidFactID) {
		t.Fatalf("zero fact ID error=%v", err)
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

func TestGetTransactionByStartIntentRejectsMalformedSuccessfulResponse(t *testing.T) {
	intentID, transactionID, commandID := uuid.New(), uuid.New(), uuid.New()
	base := map[string]any{
		"hal_transaction_id": transactionID, "cms_start_intent_id": intentID, "cms_command_id": commandID,
		"cpo_id": uuid.New(), "cms_charger_id": uuid.New(), "cms_connector_id": uuid.New(),
		"charger_ocpp_identity": "charger-1", "ocpp_connector_number": 1, "ocpp_transaction_id": 1,
		"actual_started_at": time.Now().UTC(), "meter_start_wh": 0,
	}
	for _, test := range []struct {
		name  string
		alter func(map[string]any)
		body  any
	}{
		{name: "missing transaction", body: map[string]any{}},
		{name: "zero identity", alter: func(v map[string]any) { v["hal_transaction_id"] = uuid.Nil }},
		{name: "wrong intent", alter: func(v map[string]any) { v["cms_start_intent_id"] = uuid.New() }},
		{name: "zero transaction", alter: func(v map[string]any) { v["ocpp_transaction_id"] = 0 }},
		{name: "zero connector", alter: func(v map[string]any) { v["ocpp_connector_number"] = 0 }},
		{name: "blank charger identity", alter: func(v map[string]any) { v["charger_ocpp_identity"] = " " }},
		{name: "zero started timestamp", alter: func(v map[string]any) { v["actual_started_at"] = time.Time{} }},
		{name: "negative meter start", alter: func(v map[string]any) { v["meter_start_wh"] = -1 }},
	} {
		t.Run(test.name, func(t *testing.T) {
			body := test.body
			if body == nil {
				transaction := make(map[string]any, len(base))
				for key, value := range base {
					transaction[key] = value
				}
				test.alter(transaction)
				body = map[string]any{"transaction": transaction}
			}
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) { _ = json.NewEncoder(writer).Encode(body) }))
			defer server.Close()
			_, err := New(config.HAL{BaseURL: server.URL, CMSBearerToken: "test", RequestTimeout: time.Second}).GetTransactionByStartIntent(context.Background(), intentID)
			if !errors.Is(err, ErrInvalidTransactionResponse) {
				t.Fatalf("error=%v, want ErrInvalidTransactionResponse", err)
			}
		})
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

func TestCommandResponsesRequireSemanticContract(t *testing.T) {
	requestedID, halID := uuid.New(), uuid.New()
	updatedAt := time.Date(2026, time.August, 20, 12, 0, 0, 0, time.UTC).Format(time.RFC3339)
	valid := map[string]any{"hal_command_id": halID.String(), "cms_command_id": requestedID.String(), "kind": "START", "state": "OCPP_ACCEPTED", "hal_transaction_id": nil, "ocpp_transaction_id": nil, "updated_at": updatedAt}
	differentID := uuid.New()
	zeroTransaction := uuid.Nil.String()
	for _, test := range []struct {
		name  string
		body  any
		valid bool
	}{
		{name: "valid snake case", body: valid, valid: true},
		{name: "old Go field names", body: map[string]any{"HALCommandID": halID.String(), "CMSCommandID": requestedID.String(), "Kind": "START", "State": "OCPP_ACCEPTED", "UpdatedAt": updatedAt}},
		{name: "missing command", body: nil},
		{name: "zero HAL identity", body: map[string]any{"hal_command_id": uuid.Nil.String(), "cms_command_id": requestedID.String(), "kind": "START", "state": "OCPP_ACCEPTED", "updated_at": updatedAt}},
		{name: "missing CMS identity", body: map[string]any{"hal_command_id": halID.String(), "kind": "START", "state": "OCPP_ACCEPTED", "updated_at": updatedAt}},
		{name: "different CMS identity", body: map[string]any{"hal_command_id": halID.String(), "cms_command_id": differentID.String(), "kind": "START", "state": "OCPP_ACCEPTED", "updated_at": updatedAt}},
		{name: "invalid kind", body: map[string]any{"hal_command_id": halID.String(), "cms_command_id": requestedID.String(), "kind": "OTHER", "state": "OCPP_ACCEPTED", "updated_at": updatedAt}},
		{name: "invalid state", body: map[string]any{"hal_command_id": halID.String(), "cms_command_id": requestedID.String(), "kind": "START", "state": "ACCEPTED", "updated_at": updatedAt}},
		{name: "zero optional transaction identity", body: map[string]any{"hal_command_id": halID.String(), "cms_command_id": requestedID.String(), "kind": "START", "state": "MATERIALIZED", "hal_transaction_id": zeroTransaction, "updated_at": updatedAt}},
		{name: "zero optional OCPP transaction identity", body: map[string]any{"hal_command_id": halID.String(), "cms_command_id": requestedID.String(), "kind": "START", "state": "MATERIALIZED", "ocpp_transaction_id": 0, "updated_at": updatedAt}},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writer.Header().Set("Content-Type", "application/json")
				response := map[string]any{}
				if test.body != nil {
					response["command"] = test.body
				}
				_ = json.NewEncoder(writer).Encode(response)
			}))
			defer server.Close()

			command, err := New(config.HAL{BaseURL: server.URL, CMSBearerToken: "test", RequestTimeout: time.Second}).Start(context.Background(), StartCommand{CMSCommandID: requestedID}, "test-correlation")
			if test.valid {
				if err != nil || command.HALCommandID != halID || command.CMSCommandID != requestedID {
					t.Fatalf("command=%+v err=%v", command, err)
				}
				return
			}
			if !errors.Is(err, ErrInvalidCommandResponse) {
				t.Fatalf("error=%v, want ErrInvalidCommandResponse", err)
			}
		})
	}
}

func TestGetCommandRejectsMalformedSuccessfulResponse(t *testing.T) {
	requestedID := uuid.New()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte(`{"command":{"HALCommandID":"` + uuid.NewString() + `","CMSCommandID":"` + requestedID.String() + `","Kind":"START","State":"OCPP_ACCEPTED","UpdatedAt":"2026-08-20T12:00:00Z"}}`))
	}))
	defer server.Close()

	_, err := New(config.HAL{BaseURL: server.URL, CMSBearerToken: "test", RequestTimeout: time.Second}).GetCommand(context.Background(), requestedID)
	if !errors.Is(err, ErrInvalidCommandResponse) {
		t.Fatalf("error=%v, want ErrInvalidCommandResponse", err)
	}
}
