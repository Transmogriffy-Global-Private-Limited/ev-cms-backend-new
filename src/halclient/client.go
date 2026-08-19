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
	ErrUnavailable          = errors.New("HAL v1 service is unavailable")
	ErrMissingCorrelationID = errors.New("HAL v1 mutation requires a non-empty correlation ID")
)

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
	Enabled             bool               `json:"enabled"`
	Connectors          []ConnectorMapping `json:"connectors"`
}

type StartCommand struct {
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
	EnergyLimitWh       int64     `json:"energy_limit_wh"`
	MaxDurationSeconds  int64     `json:"max_duration_seconds"`
}

type StopCommand struct {
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

func (client *Client) SyncMapping(ctx context.Context, mapping ChargerMapping, correlationID string) error {
	return client.mutate(ctx, http.MethodPut, "/v1/mappings/chargers/"+mapping.CMSChargerID.String(), mapping.CMSChargerID.String(), correlationID, mapping, nil)
}

func (client *Client) Start(ctx context.Context, command StartCommand, correlationID string) (Command, error) {
	var response Command
	err := client.mutate(ctx, http.MethodPost, "/v1/remote-commands/start", command.CMSCommandID.String(), correlationID, command, &response)
	return response, err
}

func (client *Client) Stop(ctx context.Context, command StopCommand, correlationID string) (Command, error) {
	var response Command
	err := client.mutate(ctx, http.MethodPost, "/v1/remote-commands/stop", command.CMSCommandID.String(), correlationID, command, &response)
	return response, err
}

func (client *Client) GetCommand(ctx context.Context, id uuid.UUID) (Command, error) {
	var command Command
	if err := client.request(ctx, http.MethodGet, "/v1/remote-commands?cms_command_id="+url.QueryEscape(id.String()), "", "", nil, &command); err != nil {
		return Command{}, err
	}
	return command, nil
}

func (client *Client) mutate(ctx context.Context, method, path, idempotency, correlation string, body any, target any) error {
	correlation = strings.TrimSpace(correlation)
	if correlation == "" {
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
		return nil
	}
	if err := json.Unmarshal(wrapper.Command, target); err != nil {
		return fmt.Errorf("decode HAL v1 command: %w", err)
	}
	return nil
}
