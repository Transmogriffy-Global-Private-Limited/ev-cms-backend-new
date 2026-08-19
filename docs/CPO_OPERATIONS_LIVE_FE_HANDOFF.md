
### `GET /api/v1/cpo/operations/fleet`

This endpoint provides a high-level, aggregated snapshot of the entire charger fleet for the authenticated CPO. It's designed to power dashboard widgets and overview cards.

The response is a `CpoFleetView` object containing counts of chargers by connection status, connectors by availability status, and the number of currently active charging sessions.

#### Example Response

```json
{
  "chargers": {
    "total": 50,
    "connected": 45,
    "disconnected": 5
  },
  "connectors": {
    "total": 100,
    "available": 80,
    "charging": 15,
    "faulted": 2,
    "unavailable": 3
  },
  "active_sessions": 15
}
```

### `GET /api/v1/cpo/operations/chargers/{charger_id}`

This returns a detailed, combined view of a single charger, identified by its public 6-character `charger_id`. The response object, `CpoChargerWithLiveState`, merges the administrative data (like name, model, hub assignment) with the live operational data (`live_state`) for both the charger and each of its connectors.

Use this endpoint to populate a detailed view for a specific charger. The `live_state` object may be absent if the CMS has no runtime information for that charger. The `freshness` field within `live_state` is crucial for the UI to indicate if the data is recent or potentially outdated.

#### Example Response

A `200 OK` response with a `CpoChargerWithLiveState` payload. Note the different states across the charger and its connectors.

```json
{
  "id": "a1b2c3d4-e5f6-4a7b-8c9d-0e1f2a3b4c5d",
  "cpo_id": "c821a013-5041-42f7-80c8-aa153cf9d455",
  "charger_id": "chg123",
  "ocpp_identity": "transev-chg123",
  "vendor": "Transev",
  "model": "Triton-150",
  "serial_number": "SN-987654321",
  "max_power_kw": 150,
  "status": "ACTIVE",
  "ocpp_version": "1.6j",
  "charger_name": "Main Street Fast Charger",
  "number_of_connectors": 2,
  "protocol": "OCPP",
  "twenty_four_seven_open_status": true,
  "charger_connection_url_ws": "ws://ocpp.transev.site/ocpp/transev-chg123",
  "charger_connection_url_wss": "wss://ocpp.transev.site/ocpp/transev-chg123",
  "assigned": true,
  "hub_id": "h1b2c3d4-e5f6-4a7b-8c9d-0e1f2a3b4c5d",
  "created_at": "2026-01-15T10:00:00Z",
  "updated_at": "2026-07-20T14:30:00Z",
  "connectors": [
    {
      "id": "b1c2d3e4-f5a6-4b7c-8d9e-1f2a3b4c5d6e",
      "cpo_id": "c821a013-5041-42f7-80c8-aa153cf9d455",
      "charger_id": "a1b2c3d4-e5f6-4a7b-8c9d-0e1f2a3b4c5d",
      "connector_number": 1,
      "connector_type": "CCS-2",
      "connector_total_capacity": 150,
      "status": "ACTIVE",
      "created_at": "2026-01-15T10:00:00Z",
      "updated_at": "2026-01-15T10:00:00Z",
      "live_state": {
        "availability": "AVAILABLE",
        "availability_changed_at": "2026-08-13T10:15:00Z",
        "last_ocpp_status": "Available",
        "last_ocpp_status_at": "2026-08-13T10:15:00Z",
        "freshness": "FRESH"
      }
    },
    {
      "id": "c2d3e4f5-a6b7-4c8d-9e0f-2a3b4c5d6e7f",
      "cpo_id": "c821a013-5041-42f7-80c8-aa153cf9d455",
      "charger_id": "a1b2c3d4-e5f6-4a7b-8c9d-0e1f2a3b4c5d",
      "connector_number": 2,
      "connector_type": "CCS-2",
      "connector_total_capacity": 150,
      "status": "ACTIVE",
      "created_at": "2026-01-15T10:00:00Z",
      "updated_at": "2026-01-15T10:00:00Z",
      "live_state": {
        "availability": "UNAVAILABLE",
        "availability_changed_at": "2026-08-13T09:30:00Z",
        "last_ocpp_status": "Charging",
        "last_ocpp_status_at": "2026-08-13T09:30:00Z",
        "freshness": "FRESH"
      }
    }
  ],
  "live_state": {
    "connection_status": "ONLINE",
    "connection_changed_at": "2026-08-13T08:00:00Z",
    "freshness": "FRESH"
  }
}
```

### `GET /api/v1/cpo/operations/events`

This is the REST-based event replay endpoint. It allows the client to "catch up" on any events it may have missed while it was disconnected or inactive. This pattern is a standard for realtime data across the platform.

**Query Parameters:**
- `after_id=<number>`: Exclusive cursor. Returns events with an ID greater than this value. Start with `0` for the first call.
- `limit=<number>`: Number of events to return, from 1 to 100. Default is 50.

The response is a `CpoOperationalEventPage`. The client should process the `events` array in order, then use `next_cursor` as the `after_id` for the subsequent request if `has_more` is true. This process is repeated until the client is fully caught up, at which point it can connect to the SSE stream.

#### Example Response

```json
{
  "events": [
    {
      "id": 1001,
      "type": "cpo.charger.connection_changed",
      "cpo_id": "c821a013-5041-42f7-80c8-aa153cf9d455",
      "resource_type": "CHARGER",
      "resource_id": "a1b2c3d4-e5f6-4a7b-8c9d-0e1f2a3b4c5d",
      "data": {
        "new_status": "OFFLINE"
      },
      "occurred_at": "2026-08-13T10:20:00Z"
    },
    {
      "id": 1002,
      "type": "cpo.connector.ocpp_status_changed",
      "cpo_id": "c821a013-5041-42f7-80c8-aa153cf9d455",
      "resource_type": "CONNECTOR",
      "resource_id": "c2d3e4f5-a6b7-4c8d-9e0f-2a3b4c5d6e7f",
      "data": {
        "charger_id": "a1b2c3d4-e5f6-4a7b-8c9d-0e1f2a3b4c5d",
        "new_status": "Finishing"
      },
      "occurred_at": "2026-08-13T10:21:00Z"
    }
  ],
  "next_cursor": 1002,
  "has_more": true
}
```

### `GET /api/v1/cpo/operations/realtime/stream`

This endpoint provides a long-lived Server-Sent Events (SSE) stream for low-latency notifications of state changes.

**Crucial Implementation Detail:** You **must** use `fetch()` with a `ReadableStream` to consume this endpoint. The native browser `EventSource` API does not support sending the required `Authorization` and `X-CPO-App-ID` headers.

The stream sends events that should be treated as invalidation hints. When an event for a specific charger or connector is received, the frontend should refetch its authoritative state using the `/operations/chargers/{resource_id}` endpoint.

## Operational Event Types

The realtime stream and replay endpoint will send events with different `type` values. The frontend should use the `type`, `resource_type`, and `resource_id` to determine which data to invalidate and refetch. For events related to a `CONNECTOR` or `CHARGING_SESSION`, the `data` payload will include the parent `charger_id` to simplify refetching.

| Event Type | Resource Type | `data` payload includes | Invalidation Target(s) | Description |
| :--- | :--- | :--- | :--- | :--- |
| `cpo.charger.connection_changed` | `CHARGER` | `{ "new_status": "ONLINE" \| "OFFLINE" }` | `GET /operations/chargers/{resource_id}`<br>`GET /operations/fleet` | A charger has connected to or disconnected from the HAL. |
| `cpo.connector.availability_changed` | `CONNECTOR` | `{ "charger_id": "...", "new_availability": "AVAILABLE" \| "UNAVAILABLE" }` | `GET /operations/chargers/{data.charger_id}`<br>`GET /operations/fleet` | A connector's simplified availability has changed. |
| `cpo.connector.ocpp_status_changed` | `CONNECTOR` | `{ "charger_id": "...", "new_status": "..." }` | `GET /operations/chargers/{data.charger_id}`<br>`GET /operations/fleet` | A connector's raw OCPP status has changed (e.g., to `Charging`, `Faulted`). |
| `cpo.charging_session.started` | `CHARGING_SESSION` | `{ "charger_id": "...", "connector_id": "..." }` | `GET /operations/chargers/{data.charger_id}`<br>`GET /operations/fleet` | A new charging session has started on a connector. |
| `cpo.charging_session.stopped` | `CHARGING_SESSION` | `{ "charger_id": "...", "connector_id": "..." }` | `GET /operations/chargers/{data.charger_id}`<br>`GET /operations/fleet` | A charging session has ended. |

## Realtime, Replay, and Recovery Workflow

A robust frontend should implement the following logic to ensure its state is always consistent with the backend. This workflow is a standard pattern used for realtime updates across the platform, including in the SuperAdmin interface.

### Sequence Diagram

This diagram illustrates the complete flow from initial load to handling live events and recovering from a disconnection.

```mermaid
sequenceDiagram
    participant FE as Frontend
    participant LS as Local Storage
    participant BE as Backend REST API
    participant SSE as Backend SSE Stream

    FE->>+LS: Get last_event_id
    LS-->>-FE: last_event_id (or 0)

    loop Catch-up via Replay
        FE->>+BE: GET /events?after_id={last_event_id}
        BE-->>-FE: { events, next_cursor, has_more }
        FE->>FE: Process events, invalidate UI data
        FE->>+LS: Store next_cursor as last_event_id
        LS-->>-FE: ack
        break if !has_more
    end

    FE->>+SSE: GET /realtime/stream (Last-Event-ID: {last_event_id})
    SSE-->>-FE: Stream connection opens

    loop Process Live Events
        SSE-->>FE: event(id, type, resource_id, data)
        FE->>FE: Invalidate UI data for resource
        FE->>+BE: GET /chargers/{id} or /fleet
        BE-->>-FE: Updated data
        FE->>FE: Update UI with new data
        FE->>+LS: Store event.id as last_event_id
        LS-->>-FE: ack
    end

    Note over FE,SSE: Network disconnects...

    FE->>FE: Reconnection logic (exponential backoff)
    FE->>FE: Restart from "Catch-up via Replay"

```

1.  **Initial Load**: When the application loads, fetch the necessary data from the REST endpoints (`/fleet`, `/chargers/{id}`, etc.).
2.  **Catch-up via Replay**: Retrieve the last processed event ID from persistent local storage. Use the `/events` endpoint to fetch all events that have occurred since that ID, processing them in order until `has_more` is false.
3.  **Connect to SSE Stream**: Once caught up, connect to the `/realtime/stream` endpoint, providing the last processed event ID in the `Last-Event-ID` header.
    d. After the event is successfully processed (i.e., the refetch is complete), persist the new `event.id` as the last processed ID.
5.  **Handle Disconnection**: If the SSE stream closes (due to network issues, token expiry, or browser tab suspension), restart the process from step 2 (Catch-up via Replay). Use a bounded exponential backoff strategy for reconnection attempts.

## UI/UX Status Mapping Guide

The combination of a resource's administrative status, connection status, live availability, and data freshness determines its true operational state. The UI should prioritize these statuses to give the operator a clear and accurate picture.

Here is a suggested mapping of API data to user-facing UI elements (colors, icons, text).

| Priority | Condition | Suggested UI (Color, Icon, Text) | Explanation |
| :--- | :--- | :--- | :--- |
| 1 | `status` is `UNDERMAINTENANCE` or `SUSPENDED` | 🟠 Orange, Wrench/Pause Icon, "Maintenance" / "Suspended" | The administrative status overrides any live data. The charger is intentionally offline. |
| 2 | `status` is `INACTIVE` or `DECOMMISSIONED` | ⚫ Black/Grey, Power-off Icon, "Inactive" / "Decommissioned" | The charger is not operational from a business perspective. |
| 3 | `live_state` is missing | ⚪ Grey, Question Mark Icon, "Unknown" | The CMS has never received any live data for this charger. |
| 4 | `live_state.connection_status` is `OFFLINE` | ⚪ Grey, Offline/Cloud-slash Icon, "Offline" | The charger is not connected to the HAL. |
| 5 | `live_state.freshness` is `STALE` | 🟡 Yellow, Clock Icon, "Status Unknown" or "Last seen X ago" | The data is outdated. The charger might be online, but the CMS can't be sure. |
| 6 | `live_state.last_ocpp_status` is `Faulted` | 🔴 Red, Warning/Exclamation Icon, "Faulted" | The charger is online but has reported a fault. This is a high-priority issue. |
| 7 | `live_state.last_ocpp_status` is `Charging` | 🔵 Blue, Bolt/Charging Icon, "Charging" | The connector is online and actively charging a vehicle. |
| 8 | `live_state.last_ocpp_status` is `Preparing` or `Finishing` | 🔵 Blue, Hourglass Icon, "Preparing" / "Finishing" | The connector is in a transitional state. |
| 9 | `live_state.availability` is `AVAILABLE` | 🟢 Green, Checkmark/Plug Icon, "Available" | The connector is online and ready to be used. |
| 10 | `live_state.availability` is `UNAVAILABLE` (and not covered above) | ⚪ Grey, Block/Stop Icon, "Unavailable" | The connector is online but unavailable for other reasons (e.g., reserved, operator action). |

### Authenticated SSE Client Example

This TypeScript function demonstrates how to consume the authenticated SSE stream using `fetch`.

