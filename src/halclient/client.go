// Package halclient owns the CMS side of the authenticated HAL v1 boundary.
package halclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/config"
	"github.com/google/uuid"
)

var (
	ErrUnavailable                = errors.New("HAL v1 service is unavailable")
	ErrMissingCorrelationID       = errors.New("HAL v1 mutation requires a non-zero canonical UUID correlation ID")
	ErrInvalidCommandResponse     = errors.New("HAL v1 command response violates the service contract")
	ErrInvalidTransactionResponse = errors.New("HAL v1 transaction response violates the service contract")
	ErrInvalidFactID              = errors.New("HAL v1 fact ID must be a non-zero canonical UUID")
)

// CommandResponseError deliberately carries only the failed invariant. The
// response body can contain service data and must not be propagated to logs.
type CommandResponseError struct{ invariant string }

func (err *CommandResponseError) Error() string {
	return "HAL v1 command response violates the service contract: " + err.invariant
}

func (err *CommandResponseError) Unwrap() error { return ErrInvalidCommandResponse }

// TransactionResponseError deliberately keeps provider payloads out of logs.
// A syntactically valid response is not authoritative transaction truth unless
// every cross-service identity and OCPP invariant is present.
type TransactionResponseError struct{ invariant string }

func (err *TransactionResponseError) Error() string {
	return "HAL v1 transaction response violates the service contract: " + err.invariant
}

func (err *TransactionResponseError) Unwrap() error { return ErrInvalidTransactionResponse }

type HTTPError struct {
	Status int
	Code   string
}

func (err *HTTPError) Error() string { return fmt.Sprintf("HAL v1 returned HTTP %d", err.Status) }

type Client struct {
	baseURL string
	bearer  string
	http    *http.Client
}

func New(cfg config.HAL) *Client {
	return &Client{
		baseURL: strings.TrimRight(cfg.BaseURL, "/"), bearer: cfg.CMSBearerToken,
		http: &http.Client{Timeout: cfg.RequestTimeout},
	}
}

func (client *Client) Available() bool {
	return client != nil && client.baseURL != "" && client.bearer != ""
}

type ConnectorMapping struct {
	CMSConnectorID      uuid.UUID `json:"cms_connector_id"`
	OCPPConnectorNumber int       `json:"ocpp_connector_number"`
}

type ChargerMapping struct {
	CPOID               uuid.UUID          `json:"cpo_id"`
	CMSChargerID        uuid.UUID          `json:"cms_charger_id"`
	ChargerOCPPIdentity string             `json:"charger_ocpp_identity"`
	ExpectedSerial      string             `json:"expected_serial,omitempty"`
	Enabled             bool               `json:"enabled"`
	Connectors          []ConnectorMapping `json:"connectors"`
}

type StartCommand struct {
	TraceID             uuid.UUID `json:"trace_id"`
	CMSCommandID        uuid.UUID `json:"cms_command_id"`
	CMSStartIntentID    uuid.UUID `json:"cms_start_intent_id"`
	CPOID               uuid.UUID `json:"cpo_id"`
	CustomerID          uuid.UUID `json:"customer_id"`
	CMSChargerID        uuid.UUID `json:"cms_charger_id"`
	CMSConnectorID      uuid.UUID `json:"cms_connector_id"`
	ChargerOCPPIdentity string    `json:"charger_ocpp_identity"`
	OCPPConnectorNumber int       `json:"ocpp_connector_number"`
	IDTag               string    `json:"id_tag"`
	CredentialExpiresAt time.Time `json:"credential_expires_at"`
	CommandExpiresAt    time.Time `json:"command_expires_at"`
	LimitType           string    `json:"limit_type"`
	EnergyLimitWh       int64     `json:"energy_limit_wh"`
	EnergyLimitSource   string    `json:"energy_limit_source"`
	MaxDurationSeconds  int64     `json:"max_duration_seconds"`
	DurationLimitSource string    `json:"duration_limit_source"`
}

type StopCommand struct {
	TraceID                uuid.UUID `json:"trace_id"`
	CMSCommandID           uuid.UUID `json:"cms_command_id"`
	CMSChargingSessionID   uuid.UUID `json:"cms_charging_session_id"`
	CPOID                  uuid.UUID `json:"cpo_id"`
	CustomerID             uuid.UUID `json:"customer_id"`
	CMSChargerID           uuid.UUID `json:"cms_charger_id"`
	CMSConnectorID         uuid.UUID `json:"cms_connector_id"`
	ChargerOCPPIdentity    string    `json:"charger_ocpp_identity"`
	OCPPConnectorNumber    int       `json:"ocpp_connector_number"`
	HALTransactionID       uuid.UUID `json:"hal_transaction_id"`
	OCPPTransactionID      int64     `json:"ocpp_transaction_id"`
	RequestedStopInitiator string    `json:"requested_stop_initiator"`
	RequestedStopReason    string    `json:"requested_stop_reason"`
	CommandExpiresAt       time.Time `json:"command_expires_at"`
}

type Command struct {
	HALCommandID      uuid.UUID  `json:"hal_command_id"`
	CMSCommandID      uuid.UUID  `json:"cms_command_id"`
	Kind              string     `json:"kind"`
	State             string     `json:"state"`
	HALTransactionID  *uuid.UUID `json:"hal_transaction_id"`
	OCPPTransactionID *int64     `json:"ocpp_transaction_id"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

// Transaction is the exact authoritative start truth returned by HAL's
// service-only reconciliation lookup. CMS never derives these identities from
// a charger, command response, or timeout.
type Transaction struct {
	HALTransactionID    uuid.UUID `json:"hal_transaction_id"`
	CMSStartIntentID    uuid.UUID `json:"cms_start_intent_id"`
	CMSCommandID        uuid.UUID `json:"cms_command_id"`
	CPOID               uuid.UUID `json:"cpo_id"`
	CMSChargerID        uuid.UUID `json:"cms_charger_id"`
	CMSConnectorID      uuid.UUID `json:"cms_connector_id"`
	ChargerOCPPIdentity string    `json:"charger_ocpp_identity"`
	OCPPConnectorNumber int       `json:"ocpp_connector_number"`
	OCPPTransactionID   int64     `json:"ocpp_transaction_id"`
	ActualStartedAt     time.Time `json:"actual_started_at"`
	MeterStartWh        int64     `json:"meter_start_wh"`
}

func (client *Client) SyncMapping(ctx context.Context, mapping ChargerMapping, correlationID string) error {
	return client.mutate(ctx, http.MethodPut, "/v1/mappings/chargers/"+mapping.CMSChargerID.String(), mapping.CMSChargerID.String(), correlationID, mapping, nil)
}

func (client *Client) Start(ctx context.Context, command StartCommand, correlationID string) (Command, error) {
	var response Command
	err := client.mutate(ctx, http.MethodPost, "/v1/remote-commands/start", command.CMSCommandID.String(), correlationID, command, &response)
	if err == nil {
		err = validateCommand(response, command.CMSCommandID, "START")
	}
	return response, err
}

func (client *Client) Stop(ctx context.Context, command StopCommand, correlationID string) (Command, error) {
	var response Command
	err := client.mutate(ctx, http.MethodPost, "/v1/remote-commands/stop", command.CMSCommandID.String(), correlationID, command, &response)
	if err == nil {
		err = validateCommand(response, command.CMSCommandID, "STOP")
	}
	return response, err
}

func (client *Client) GetCommand(ctx context.Context, id uuid.UUID) (Command, error) {
	var command Command
	if err := client.request(ctx, http.MethodGet, "/v1/remote-commands?cms_command_id="+url.QueryEscape(id.String()), "", "", nil, &command); err != nil {
		return Command{}, err
	}
	if err := validateCommand(command, id, ""); err != nil {
		return Command{}, err
	}
	return command, nil
}

func (client *Client) GetTransactionByStartIntent(ctx context.Context, id uuid.UUID) (Transaction, error) {
	var wrapper struct {
		Transaction *Transaction `json:"transaction"`
	}
	if err := client.requestJSON(ctx, http.MethodGet, "/v1/transactions?cms_start_intent_id="+url.QueryEscape(id.String()), nil, &wrapper); err != nil {
		return Transaction{}, err
	}
	if wrapper.Transaction == nil {
		return Transaction{}, invalidTransactionResponse("missing transaction object")
	}
	if err := validateTransaction(*wrapper.Transaction, id); err != nil {
		return Transaction{}, err
	}
	return *wrapper.Transaction, nil
}

// RequeueFact invokes HAL's narrow operator recovery socket. It never creates
// a fact or rewrites its immutable payload; HAL accepts only a durable exact
// fact in RECONCILIATION_REQUIRED state.
func (client *Client) RequeueFact(ctx context.Context, factID uuid.UUID, correlationID string) error {
	if factID == uuid.Nil {
		return ErrInvalidFactID
	}
	correlationID = strings.TrimSpace(correlationID)
	parsed, err := uuid.Parse(correlationID)
	if err != nil || parsed == uuid.Nil || parsed.String() != correlationID {
		return ErrMissingCorrelationID
	}
	if !client.Available() {
		return ErrUnavailable
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, client.baseURL+"/v1/facts/"+factID.String()+"/requeue", nil)
	if err != nil {
		return fmt.Errorf("create HAL fact requeue request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+client.bearer)
	req.Header.Set("X-Correlation-ID", correlationID)
	response, err := client.http.Do(req)
	if err != nil {
		return fmt.Errorf("send HAL fact requeue request: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		var errorBody struct {
			Error string `json:"error"`
		}
		_ = json.NewDecoder(io.LimitReader(response.Body, 32*1024)).Decode(&errorBody)
		return &HTTPError{Status: response.StatusCode, Code: errorBody.Error}
	}
	var result struct {
		FactID uuid.UUID `json:"fact_id"`
		Status string    `json:"status"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 32*1024)).Decode(&result); err != nil {
		return fmt.Errorf("decode HAL fact requeue response: %w", err)
	}
	if result.FactID != factID || result.Status != "PENDING" {
		return invalidCommandResponse("fact requeue response does not confirm the requested pending fact")
	}
	return nil
}

func (client *Client) mutate(ctx context.Context, method, path, idempotency, correlation string, body any, target any) error {
	parsed, err := uuid.Parse(correlation)
	if err != nil || parsed == uuid.Nil || parsed.String() != correlation {
		return ErrMissingCorrelationID
	}
	return client.request(ctx, method, path, idempotency, correlation, body, target)
}

func (client *Client) request(ctx context.Context, method, path, idempotency, correlation string, body any, target any) error {
	if !client.Available() {
		return ErrUnavailable
	}
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal HAL v1 request: %w", err)
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, client.baseURL+path, reader)
	if err != nil {
		return fmt.Errorf("create HAL v1 request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+client.bearer)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if idempotency != "" {
		req.Header.Set("Idempotency-Key", idempotency)
	}
	if correlation != "" {
		req.Header.Set("X-Correlation-ID", correlation)
	}
	resp, err := client.http.Do(req)
	if err != nil {
		return fmt.Errorf("send HAL v1 request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var errorBody struct {
			Error string `json:"error"`
		}
		_ = json.NewDecoder(io.LimitReader(resp.Body, 32*1024)).Decode(&errorBody)
		return &HTTPError{Status: resp.StatusCode, Code: errorBody.Error}
	}
	if target == nil {
		return nil
	}
	// HAL wraps command responses in {"command": ...}.
	var wrapper struct {
		Command json.RawMessage `json:"command"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64*1024)).Decode(&wrapper); err != nil {
		return fmt.Errorf("decode HAL v1 response: %w", err)
	}
	if len(wrapper.Command) == 0 {
		return invalidCommandResponse("missing command object")
	}
	if err := json.Unmarshal(wrapper.Command, target); err != nil {
		return fmt.Errorf("decode HAL v1 command: %w", err)
	}
	return nil
}

func invalidCommandResponse(invariant string) error {
	return &CommandResponseError{invariant: invariant}
}

func invalidTransactionResponse(invariant string) error {
	return &TransactionResponseError{invariant: invariant}
}

func validateTransaction(transaction Transaction, expectedStartIntentID uuid.UUID) error {
	if transaction.HALTransactionID == uuid.Nil {
		return invalidTransactionResponse("hal_transaction_id must be a nonzero UUID")
	}
	if transaction.CMSStartIntentID == uuid.Nil {
		return invalidTransactionResponse("cms_start_intent_id must be a nonzero UUID")
	}
	if transaction.CMSStartIntentID != expectedStartIntentID {
		return invalidTransactionResponse("cms_start_intent_id does not match the requested start intent")
	}
	if transaction.CMSCommandID == uuid.Nil {
		return invalidTransactionResponse("cms_command_id must be a nonzero UUID")
	}
	if transaction.CPOID == uuid.Nil {
		return invalidTransactionResponse("cpo_id must be a nonzero UUID")
	}
	if transaction.CMSChargerID == uuid.Nil {
		return invalidTransactionResponse("cms_charger_id must be a nonzero UUID")
	}
	if transaction.CMSConnectorID == uuid.Nil {
		return invalidTransactionResponse("cms_connector_id must be a nonzero UUID")
	}
	if strings.TrimSpace(transaction.ChargerOCPPIdentity) == "" {
		return invalidTransactionResponse("charger_ocpp_identity is required")
	}
	if transaction.OCPPConnectorNumber < 1 {
		return invalidTransactionResponse("ocpp_connector_number must be positive")
	}
	if transaction.OCPPTransactionID < 1 {
		return invalidTransactionResponse("ocpp_transaction_id must be positive")
	}
	if transaction.ActualStartedAt.IsZero() {
		return invalidTransactionResponse("actual_started_at is required")
	}
	if transaction.MeterStartWh < 0 {
		return invalidTransactionResponse("meter_start_wh must not be negative")
	}
	return nil
}

func validateCommand(command Command, expectedCMSCommandID uuid.UUID, expectedKind string) error {
	if command.HALCommandID == uuid.Nil {
		return invalidCommandResponse("hal_command_id must be a nonzero UUID")
	}
	if command.CMSCommandID == uuid.Nil {
		return invalidCommandResponse("cms_command_id must be a nonzero UUID")
	}
	if command.CMSCommandID != expectedCMSCommandID {
		return invalidCommandResponse("cms_command_id does not match the requested command")
	}
	if command.Kind != "START" && command.Kind != "STOP" {
		return invalidCommandResponse("kind is not a supported command kind")
	}
	if expectedKind != "" && command.Kind != expectedKind {
		return invalidCommandResponse("kind does not match the requested command")
	}
	if !validCommandState(command.State) {
		return invalidCommandResponse("state is not a supported durable command state")
	}
	if command.UpdatedAt.IsZero() {
		return invalidCommandResponse("updated_at is required")
	}
	if command.HALTransactionID != nil && *command.HALTransactionID == uuid.Nil {
		return invalidCommandResponse("hal_transaction_id must not be a zero UUID")
	}
	if command.OCPPTransactionID != nil && *command.OCPPTransactionID < 1 {
		return invalidCommandResponse("ocpp_transaction_id must be positive when present")
	}
	return nil
}

func validCommandState(state string) bool {
	switch state {
	case "PERSISTED", "PENDING_DELIVERY", "DELIVERY_ATTEMPTED", "OCPP_ACCEPTED", "OCPP_REJECTED", "AMBIGUOUS", "MATERIALIZED", "SUPERSEDED":
		return true
	default:
		return false
	}
}

func (client *Client) requestJSON(ctx context.Context, method, path string, body any, target any) error {
	if !client.Available() {
		return ErrUnavailable
	}
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal HAL v1 request: %w", err)
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, client.baseURL+path, reader)
	if err != nil {
		return fmt.Errorf("create HAL v1 request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+client.bearer)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	response, err := client.http.Do(req)
	if err != nil {
		return fmt.Errorf("send HAL v1 request: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		var errorBody struct {
			Error string `json:"error"`
		}
		_ = json.NewDecoder(io.LimitReader(response.Body, 32*1024)).Decode(&errorBody)
		return &HTTPError{Status: response.StatusCode, Code: errorBody.Error}
	}
	if target == nil {
		return nil
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 64*1024)).Decode(target); err != nil {
		return fmt.Errorf("decode HAL v1 response: %w", err)
	}
	return nil
}
