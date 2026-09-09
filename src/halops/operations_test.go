package halops

import (
	"errors"
	"net"
	"testing"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/halclient"
	"github.com/google/uuid"
)

type timeoutError struct{}

func (timeoutError) Error() string   { return "timeout" }
func (timeoutError) Timeout() bool   { return true }
func (timeoutError) Temporary() bool { return true }

var _ net.Error = timeoutError{}

func TestMappingFailureDiagnosticKeepsOnlySafeOperationalEvidence(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name                   string
		cause                  error
		wantCategory, wantCode string
		wantStatus             any
	}{
		{name: "unavailable", cause: halclient.ErrUnavailable, wantCategory: "hal_unavailable"},
		{name: "timeout", cause: timeoutError{}, wantCategory: "timeout"},
		{name: "provider", cause: &halclient.HTTPError{Status: 409, Code: "mapping_conflict"}, wantCategory: "provider_http", wantCode: "mapping_conflict", wantStatus: 409},
		{name: "unsafe provider code discarded", cause: &halclient.HTTPError{Status: 400, Code: "body with secret"}, wantCategory: "provider_http", wantStatus: 400},
		{name: "invalid correlation", cause: halclient.ErrMissingCorrelationID, wantCategory: "invalid_correlation"},
		{name: "transport", cause: errors.New("transport"), wantCategory: "transport"},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			category, status, code, detail := mappingFailureDiagnostic(test.cause)
			if category != test.wantCategory || status != test.wantStatus || code != test.wantCode || detail == "" {
				t.Fatalf("diagnostic=(%q,%v,%q,%q)", category, status, code, detail)
			}
		})
	}
}

func TestCompletionEvidenceFromTransactionRequiresExactDurableTerminalEvidence(t *testing.T) {
	transactionID, intentID, commandID := uuid.New(), uuid.New(), uuid.New()
	completedAt := time.Date(2026, time.September, 9, 10, 0, 0, 0, time.UTC)
	meterStop := int64(140)
	base := halclient.Transaction{
		HALTransactionID:    transactionID,
		CMSStartIntentID:    intentID,
		CMSCommandID:        commandID,
		CPOID:               uuid.New(),
		CMSChargerID:        uuid.New(),
		CMSConnectorID:      uuid.New(),
		ChargerOCPPIdentity: "charger-1",
		OCPPConnectorNumber: 1,
		OCPPTransactionID:   42,
		ActualStartedAt:     completedAt.Add(-time.Hour),
		MeterStartWh:        100,
		StopState:           "COMPLETED",
		CompletedAt:         &completedAt,
		MeterStopWh:         &meterStop,
		OCPPStopReason:      "Local",
	}

	for _, test := range []struct {
		name          string
		alter         func(*halclient.Transaction)
		wantCompleted bool
		wantError     bool
	}{
		{name: "active is not terminal", alter: func(transaction *halclient.Transaction) { transaction.CompletedAt = nil }, wantCompleted: false},
		{name: "completed", wantCompleted: true},
		{name: "wrong terminal state", alter: func(transaction *halclient.Transaction) { transaction.StopState = "STOP_PENDING" }, wantError: true},
		{name: "missing stop meter", alter: func(transaction *halclient.Transaction) { transaction.MeterStopWh = nil }, wantError: true},
		{name: "decreasing stop meter", alter: func(transaction *halclient.Transaction) { meter := int64(99); transaction.MeterStopWh = &meter }, wantError: true},
		{name: "completion before start", alter: func(transaction *halclient.Transaction) {
			completed := transaction.ActualStartedAt.Add(-time.Second)
			transaction.CompletedAt = &completed
		}, wantError: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			transaction := base
			if test.alter != nil {
				test.alter(&transaction)
			}
			evidence, completed, err := completionEvidenceFromTransaction(transaction)
			if (err != nil) != test.wantError || completed != test.wantCompleted {
				t.Fatalf("evidence=%+v completed=%t err=%v", evidence, completed, err)
			}
			if completed && (evidence.HALTransactionID != transactionID || evidence.ActualCompletedAt != completedAt || evidence.MeterStopWh != meterStop) {
				t.Fatalf("completion evidence=%+v", evidence)
			}
		})
	}
}
