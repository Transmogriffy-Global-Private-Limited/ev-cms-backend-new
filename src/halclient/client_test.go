package halclient

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

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
