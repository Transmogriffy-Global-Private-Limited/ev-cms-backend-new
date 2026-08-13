# CPO Live Operations: Frontend Constants

This document enumerates the constants required by a frontend developer when integrating the CPO Live Operations APIs, as detailed in `CPO_OPERATIONS_LIVE_FE_HANDOFF.md`.

## 1. API Paths

These are the relative paths for the API endpoints.

- `/api/v1/cpo/operations/fleet`
- `/api/v1/cpo/operations/chargers/{charger_id}`
- `/api/v1/cpo/operations/events`
- `/api/v1/cpo/operations/realtime/stream`

## 2. HTTP Headers

- `Authorization`: `Bearer <token>` (For all requests)
- `X-CPO-App-ID`: `<app_id>` (For all requests)
- `Last-Event-ID`: `<event_id>` (For the SSE stream to resume)

## 3. Query Parameters

- `after_id`: (For `/events` endpoint)
- `limit`: (For `/events` endpoint)

## 4. Status and State Constants

These string literals are defined in the `TypeScript Contract` section of the handoff document.

### Charger Connection Status
*(from `HalChargerRuntime.connection_status`)*
- `ONLINE`
- `OFFLINE`
- `UNKNOWN`

### Data Freshness
*(from `HalChargerRuntime.freshness` and `HalConnectorRuntime.freshness`)*
- `FRESH`
- `STALE`

### Live Connector Availability (from `live_state.availability`)
*(This is the simplified availability status for a single connector.)*
- `AVAILABLE`
- `UNAVAILABLE`

### Common OCPP Connector Statuses (from `live_state.last_ocpp_status`)
*(This is the raw status reported by the charger. The API returns this as a string. The following are common values used by the backend to derive aggregated fleet statuses. This is not an exhaustive list of all possible OCPP statuses.)*
- `Available`
- `Preparing`
- `Charging`
- `Finishing`
- `Faulted`

### Aggregated Fleet View Connector Statuses
*(These are the keys used in the `GET /api/v1/cpo/operations/fleet` response to provide counts of connectors in different states. They are derived from the live OCPP statuses.)*
- `available`
- `charging`
- `faulted`
- `unavailable`

### Administrative Status (Charger & Connector)
*(from `CpoChargerWithLiveState.status` and `CpoConnectorWithLiveState.status`)*
- `ACTIVE`
- `INACTIVE`
- `SUSPENDED`
- `UNDERMAINTENANCE`
- `DECOMMISSIONED`

## 5. Realtime Event Constants

### Resource Types
*(from `CpoOperationalEvent.resource_type`)*
- `CHARGER`
- `CONNECTOR`
- `CHARGING_SESSION`

## 6. API Error Codes

These codes are found in the `error.code` field of the JSON error envelope.

- `unauthorized` (HTTP 401)
- `forbidden` (HTTP 403)
- `cpo_app_id_mismatch` (HTTP 403)
- `charger_not_found` (HTTP 404)
- `realtime_cursor_expired` (HTTP 409)