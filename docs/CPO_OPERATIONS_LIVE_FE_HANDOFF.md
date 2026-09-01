# CPO Live Operations Frontend Handoff

## Purpose

This document is for the frontend engineer implementing the CPO (Charge Point Operator) administrative dashboard, specifically for live operational views. It provides a complete guide to integrating the CPO-facing APIs for observing charger fleet status, detailed live charger state, and receiving realtime invalidation events.

The goal is to enable the creation of a responsive and accurate operational dashboard without needing to inspect backend Go source code. This document synthesizes information from the canonical OpenAPI contract, API guides, and architectural decision records.

## The CMS vs. HAL Boundary: What "Live" Means

It is critical to understand that the CMS (this backend) and the OCPP HAL (Hardware Abstraction Layer) are separate systems. The HAL manages direct communication with chargers, while the CMS owns the business and administrative logic.

The "Live Operations" APIs do **not** make synchronous calls to the HAL or the physical chargers. As stated in the administrative API contract, "Live operational REST snapshots are derived solely from committed CMS HAL projections; these reads never block on a HAL HTTP request."

- **REST is Authoritative**: The REST API endpoints are the source of truth for the current state known to the CMS.
- **SSE is for Invalidation**: The realtime Server-Sent Events (SSE) stream announces that a state has changed. It is a hint for the frontend to refetch the authoritative state from the REST endpoints. **Do not use the event payload to directly update the UI state.**
- **"Live" is a Projection**: The data from these APIs is a snapshot of the last known state committed to the CMS database. `freshness: "FRESH"` indicates the data is recent, while `freshness: "STALE"` means the last update from the HAL is older than the configured threshold. A stale state does not mean the charger is offline, only that the CMS has not heard from it recently.

## Authentication

All endpoints described in this document require an authenticated active CPO
membership, the matching `X-CPO-App-ID`, and `chargers.operations`. Roles are
default capability bundles only; the frontend must gate this surface from
`GET /api/v1/cpo/access/me` `effective_permissions`, not `role == ADMIN`.

Every request must include two headers:

1.  `Authorization: Bearer <access_token>`: The encrypted JWT for the CPO `ADMIN`.
2.  `X-CPO-App-ID: <current_app_id>`: The application identifier for the CPO.

**The `X-CPO-App-ID` header is crucial.** As explained in the identity and tenancy guide, it is **not a secret and does not authenticate a user**. It is routing and deployment identity metadata. The backend first establishes the CPO from the authenticated bearer token, then verifies that the `X-CPO-App-ID` header matches that CPO's currently configured `app_id`.

A `401 Unauthorized` error will be returned if the token is missing or invalid. A `403 Forbidden` error will be returned if the user is not a CPO `ADMIN` or if there is a `cpo_app_id_mismatch`.

## API Inventory

The following endpoints constitute the CPO Live Operations surface. They are all relative to the API base (e.g., `<https://dev-evcmsnew.transev.site/api/v1>`).

| Method and Path                                     | Auth      | Success                       | FE Purpose                                           |
| :-------------------------------------------------- | :-------- | :---------------------------- | :--------------------------------------------------- |
| `GET /api/v1/cpo/operations/fleet`                  | `chargers.operations` | `200 CpoFleetView`            | Aggregated dashboard overview of the charger fleet.  |
| `GET /api/v1/cpo/operations/chargers/{charger_id}`  | `chargers.operations` | `200 CpoChargerWithLiveState` | Detailed administrative and live state for one charger. |
| `GET /api/v1/cpo/operations/events`                 | `chargers.operations` | `200 CpoOperationalEventPage` | Durable event replay for catch-up and recovery.      |
| `GET /api/v1/cpo/operations/realtime/stream`        | `chargers.operations` | `200 text/event-stream`       | Low-latency event stream for UI invalidation.        |
| `GET /api/v1/cpo/operations/live-sessions`          | `chargers.operations` | `200 text/event-stream` | Primary full-snapshot live table: immediate `snapshot`, then `live_sessions` replacement frames. |
| `GET /api/v1/cpo/operations/live-sessions/snapshot` | `chargers.operations` | `200 LiveChargingSessionListResponse` | JSON recovery/keyset pagination when a non-stream read is needed. |
| `GET /api/v1/cpo/operations/live-sessions/events`   | `chargers.operations` | `200 CpoOperationalEventPage` | Advanced CHARGING_SESSION reconciliation cursor; not needed by the normal table. |
| `GET /api/v1/cpo/operations/live-sessions/realtime/stream` | `chargers.operations` | `200 text/event-stream` | Deprecated compatibility alias for the primary full-snapshot stream. |

## TypeScript Contract

These types describe the data structures for the CPO Live Operations APIs, synthesized from the OpenAPI specification and administrative API contracts.

```ts
export type UUID = string;
export type RFC3339 = string;

/**
 * Aggregated counts for the entire CPO charger fleet.
 * Used for top-level dashboard cards.
 */
export interface CpoFleetView {
  chargers: {
    total: number;
    connected: number;
    disconnected: number;
  };
  connectors: {
    total: number;
    available: number;
    charging: number;
    faulted: number;
    unavailable: number;
  };
  active_sessions: number;
}

/**
 * The live runtime state of a charger as known by the CMS,
 * projected from data received from the HAL.
 */
export interface HalChargerRuntime {
  connection_status: "ONLINE" | "OFFLINE" | "UNKNOWN";
  connection_changed_at: RFC3339;
  freshness: "FRESH" | "STALE";
}

/**
 * The live runtime state of a connector as known by the CMS,
 * projected from data received from the HAL.
 */
export interface HalConnectorRuntime {
  availability: "AVAILABLE" | "UNAVAILABLE";
  availability_changed_at: RFC3339;
  last_ocpp_status: string; // e.g., "Available", "Preparing", "Charging", "Faulted"
  last_ocpp_status_at: RFC3339;
  freshness: "FRESH" | "STALE";
}

/**
 * Represents a single connector with its administrative data
 * and live operational state merged.
 */
export interface CpoConnectorWithLiveState {
  // --- Administrative fields (from CMS database) ---
  id: UUID;
  cpo_id: UUID;
  charger_id: UUID;
  connector_number: number;
  connector_type: string;
  connector_total_capacity: number; // in kW
  status: "ACTIVE" | "INACTIVE" | "SUSPENDED" | "UNDERMAINTENANCE" | "DECOMMISSIONED";
  created_at: RFC3339;
  updated_at: RFC3339;

  // --- Live operational state (from HAL projection) ---
  live_state?: HalConnectorRuntime;
}

/**
 * The complete view of a charger, combining its administrative
 * configuration with its live operational state. This is the
 * payload for GET /api/v1/cpo/operations/chargers/{charger_id}.
 */
export interface CpoChargerWithLiveState {
  // --- Administrative fields (from CMS database) ---
  id: UUID; // Internal CMS UUID
  cpo_id: UUID;
  charger_id: string; // 6-char public ID
  ocpp_identity: string; // CMS/HAL mapping value, not for display
  vendor?: string;
  model?: string;
  serial_number: string;
  max_power_kw?: number;
  status: "INACTIVE" | "ACTIVE" | "SUSPENDED" | "UNDERMAINTENANCE" | "DECOMMISSIONED";
  ocpp_version: string;
  charger_name?: string;
  charger_host_name?: string;
  charger_host_phone_no?: string;
  charger_type?: string;
  segment?: string;
  sub_segment?: string;
  charger_image?: string; // Relative URL path
  charger_use_type?: string;
  number_of_connectors: number;
  parking?: string;
  protocol: string;
  twenty_four_seven_open_status: boolean;
  charger_connection_url_ws: string; // For charger provisioning
  charger_connection_url_wss: string; // For charger provisioning
  assigned: boolean; // Is the charger assigned to a hub?
  hub_id?: UUID;
  created_at: RFC3339;
  updated_at: RFC3339;

  // --- Connectors with their live state ---
  connectors: CpoConnectorWithLiveState[];

  // --- Live operational state (from HAL projection) ---
  live_state?: HalChargerRuntime;
}

/**
 * A single operational event, used for both replay and realtime.
 */
export interface CpoOperationalEvent {
  id: number; // Monotonically increasing integer
  type: string; // e.g., "cpo.charger.connection_changed"
  cpo_id: UUID;
  resource_type: "CHARGER" | "CONNECTOR" | "CHARGING_SESSION";
  resource_id: string; // UUID of the affected resource
  data: Record<string, unknown>;
  occurred_at: RFC3339;
}

/**
 * A page of operational events from the replay endpoint.
 */
export interface CpoOperationalEventPage {
  events: CpoOperationalEvent[];
  next_cursor: number;
  has_more: boolean;
}

/** A CMS-projected ongoing session; this is intentionally not billing or customer history. */
export interface LiveChargingSessionView {
  session_id: UUID;
  status: "ACTIVE" | "STOP_PENDING" | "RECONCILIATION_REQUIRED";
  started_at: RFC3339;
  duration_seconds: number; // elapsed at response.as_of; tick locally for a live clock
  customer_name: string; // CPO-visible display name only; no customer ID/email/phone
  charger_id: string;
  charger_name: string;
  hub_name?: string;
  connector_id: UUID; // Canonical CMS connector UUID, distinct from physical number
  connector_number: number;
  latest_meter_wh?: number;
  consumed_wh?: number;
  meter_observed_at?: RFC3339;
  meter_freshness: "FRESH" | "STALE" | "UNKNOWN";
  soc_percent?: string;
  soc_observed_at?: RFC3339;
  soc_freshness: "FRESH" | "STALE" | "UNKNOWN";
}

export interface LiveChargingSessionListResponse {
  sessions: LiveChargingSessionView[];
  next_after_started_at?: RFC3339;
  next_after_id?: UUID;
  has_more: boolean;
  as_of: RFC3339;
}
```

## API Endpoints Explained

### `GET /api/v1/cpo/operations/fleet`

This endpoint provides a high-level, aggregated snapshot of the entire charger fleet for the authenticated CPO. It's designed to power dashboard widgets and overview cards.

The response is a `CpoFleetView` object containing counts of chargers by connection status, connectors by availability status, and the number of currently active charging sessions.

### `GET /api/v1/cpo/operations/chargers/{charger_id}`

This returns a detailed, combined view of a single charger, identified by its public 6-character `charger_id`. The response object, `CpoChargerWithLiveState`, merges the administrative data (like name, model, hub assignment) with the live operational data (`live_state`) for both the charger and each of its connectors.

Use this endpoint to populate a detailed view for a specific charger. The `live_state` object may be absent if the CMS has no runtime information for that charger. The `freshness` field within `live_state` is crucial for the UI to indicate if the data is recent or potentially outdated.

### `GET /api/v1/cpo/operations/events`

This is the REST-based event replay endpoint. It allows the client to "catch up" on any events it may have missed while it was disconnected or inactive. This pattern is a standard for realtime data across the platform.

**Query Parameters:**
- `after_id=<number>`: Exclusive cursor. Returns events with an ID greater than this value. Start with `0` for the first call.
- `limit=<number>`: Number of events to return, from 1 to 100. Default is 50.

The response is a `CpoOperationalEventPage`. The client should process the `events` array in order, then use `next_cursor` as the `after_id` for the subsequent request if `has_more` is true. This process is repeated until the client is fully caught up, at which point it can connect to the SSE stream.

### `GET /api/v1/cpo/operations/realtime/stream`

This endpoint provides a long-lived Server-Sent Events (SSE) stream for low-latency notifications of state changes.

**Crucial Implementation Detail:** You **must** use `fetch()` with a `ReadableStream` to consume this endpoint. The native browser `EventSource` API does not support sending the required `Authorization` and `X-CPO-App-ID` headers.

The stream sends events that should be treated as invalidation hints. When an event for a specific charger or connector is received, the frontend should refetch its authoritative state using the `/operations/chargers/{resource_id}` endpoint.

### Live-session table: one full-snapshot SSE

`GET /api/v1/cpo/operations/live-sessions` is the one normal live-table
connection. It returns only materialized `ACTIVE`, `STOP_PENDING`, and
`RECONCILIATION_REQUIRED` sessions as full `LiveChargingSessionListResponse`
frames. It first emits `event: snapshot`, then emits `event: live_sessions`
after committed `charging.session_changed`, `charging.meter_changed`, or
`charging.telemetry_changed` changes. On either frame, replace the table state
with `JSON.parse(event.data).sessions`; do not merge meter patches, deduplicate
event rows, or call another endpoint per update. A completed session simply
disappears from the next replacement snapshot.

The payload includes `duration_seconds` measured at the response `as_of`, the
CPO-visible `customer_name`, and canonical `connector_id`, but no customer ID,
email, phone, wallet, tariff, total amount, or settlement fields. Use the
duration plus the current browser clock to keep a display timer moving between
frames. `charger_id`, `charger_name`, optional `hub_name`, and
`connector_number` are display fields. Meter and SoC observations are
independently fresh, stale, or unknown; never display a stale value as current
charger truth. `limit` defaults to `100` and has a maximum of `200`.

Reconnect the primary stream after any close: it always gives a new immediate
snapshot, so no `Last-Event-ID` or durable browser cursor is required for this
table. `GET /operations/live-sessions/snapshot` is available for manual refresh
or paginated recovery using the paired `after_started_at` + `after_id` cursor.
`/operations/live-sessions/events` is only advanced durable reconciliation;
the old `/realtime/stream` route is a deprecated full-stream alias. The server
revalidates the bearer session, active membership, `chargers.operations`, and
app ID on stream heartbeats.

## Realtime, Replay, and Recovery Workflow

A robust frontend should implement the following logic to ensure its state is always consistent with the backend. This workflow is a standard pattern used for realtime updates across the platform, including in the SuperAdmin interface.

1.  **Initial Load**: When the application loads, fetch the necessary data from the REST endpoints (`/fleet`, `/chargers/{id}`, etc.).
2.  **Catch-up via Replay**: Retrieve the last processed event ID from persistent local storage. Use the `/events` endpoint to fetch all events that have occurred since that ID, processing them in order until `has_more` is false.
3.  **Connect to SSE Stream**: Once caught up, connect to the `/realtime/stream` endpoint, providing the last processed event ID in the `Last-Event-ID` header.
4.  **Process Live Events**: As events arrive on the stream:
    a. Deduplicate events by checking the `id` against the last processed ID.
    b. Use the `resource_type` and `resource_id` to identify which piece of data is now stale.
    c. Trigger a refetch of the authoritative data from the appropriate REST endpoint (e.g., refetch `/operations/chargers/{resource_id}`).
    d. After the event is successfully processed (i.e., the refetch is complete), persist the new `event.id` as the last processed ID.
5.  **Handle Disconnection**: If the SSE stream closes (due to network issues, token expiry, or browser tab suspension), restart the process from step 2 (Catch-up via Replay). Use a bounded exponential backoff strategy for reconnection attempts.

### Authenticated SSE Client Example

This TypeScript function demonstrates how to consume the authenticated SSE stream using `fetch`.

```ts
import { CpoOperationalEvent } from "./types"; // Your defined types

async function failureFrom(response: Response): Promise<Error> {
  // Implement error handling similar to the main application
  // This should parse the standard ApiErrorEnvelope
  return new Error(`Request failed with status ${response.status}`);
}

export async function consumeCpoOperationalEvents(args: {
  origin: string;
  accessToken: string;
  cpoAppId: string;
  lastEventId?: number;
  signal: AbortSignal;
  onEvent: (event: CpoOperationalEvent) => Promise<void> | void;
}): Promise<number | undefined> {
  const headers = new Headers({
    Accept: "text/event-stream",
    Authorization: `Bearer ${args.accessToken}`,
    "X-CPO-App-ID": args.cpoAppId,
  });
  if (args.lastEventId !== undefined) {
    headers.set("Last-Event-ID", String(args.lastEventId));
  }

  const response = await fetch(
    `${args.origin}/api/v1/cpo/operations/realtime/stream?limit=100`,
    { headers, credentials: "omit", signal: args.signal },
  );

  if (!response.ok) throw await failureFrom(response);
  if (!response.body) throw new Error("SSE response has no readable body.");

  const reader = response.body.getReader();
  const decoder = new TextDecoder();
  let buffer = "";
  let committedCursor = args.lastEventId;

  for (;;) {
    const { value, done } = await reader.read();
    if (args.signal.aborted) {
      reader.cancel();
      return committedCursor;
    }

    buffer += decoder.decode(value, { stream: !done });
    buffer = buffer.replace(/\r\n/g, "\n");

    for (;;) {
      const boundary = buffer.indexOf("\n\n");
      if (boundary < 0) break;
      const frame = buffer.slice(0, boundary);
      buffer = buffer.slice(boundary + 2);

      let frameId: number | undefined;
      const data: string[] = [];
      for (const line of frame.split("\n")) {
        if (line.startsWith(":")) continue; // heartbeat comment
        if (line.startsWith("id:")) frameId = Number(line.slice(3).trim());
        if (line.startsWith("data:")) data.push(line.slice(5).trimStart());
      }
      if (data.length === 0) continue;

      const event = JSON.parse(data.join("\n")) as CpoOperationalEvent;
      if (!Number.isSafeInteger(event.id) || event.id !== frameId) {
        throw new Error("Unsafe or inconsistent operational event cursor.");
      }
      if (committedCursor !== undefined && event.id <= committedCursor) continue;
      
      await args.onEvent(event);
      committedCursor = event.id; // Persist only after successful processing
    }

    if (done) return committedCursor;
  }
}
```

## Error Handling

| Status/Code                 | FE Behavior                                                                                                                                                                                                                         |
| :-------------------------- | :---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `401 unauthorized`          | The access token is invalid or expired. Attempt one coordinated refresh; if that fails, redirect to login.                                                                                                                          |
| `403 forbidden`             | The user is not an authorized CPO `ADMIN` or the `X-CPO-App-ID` does not match the session (`cpo_app_id_mismatch`). This should not happen in a correctly configured app. Treat as a session error and redirect to login.               |
| `404 charger_not_found`     | The requested charger does not exist or does not belong to this CPO. Close any detailed view for this charger and refresh the main list.                                                                                             |
| `409 realtime_cursor_expired` | The `after_id` or `Last-Event-ID` is too old and has been purged from the event log. Discard the saved cursor, perform a full REST refresh of all visible data, and then reconnect to the stream without a cursor to start from the current live events. |
| Network Timeout/Abort       | The outcome of the request is unknown. For these read-only endpoints, it is safe to retry with a backoff strategy. For the SSE stream, this is the expected behavior on disconnect, and the client should initiate the recovery workflow. |

## Frontend Verification Checklist

- [ ] All requests to these endpoints include a valid `Authorization` bearer token and the correct `X-CPO-App-ID`.
- [ ] The fleet overview correctly displays aggregate data from `/operations/fleet`.
- [ ] The charger detail view correctly displays both administrative and live data from `/operations/chargers/{charger_id}`.
- [ ] The UI correctly indicates when live data is `STALE`.
- [ ] The application uses a single, shared SSE connection for all CPO operational events.
- [ ] The SSE client is implemented using `fetch` and a `ReadableStream`, not `EventSource`.
- [ ] The last processed event ID is persisted to client storage only after the event has been successfully handled.
- [ ] Events from the SSE stream trigger a REST refetch for the corresponding resource, rather than merging event data directly into the UI state.
- [ ] The application correctly handles SSE stream disconnections by performing a REST-based catch-up and reconnecting.
- [ ] The application correctly handles a `409 realtime_cursor_expired` error by performing a full data refresh and resetting the event cursor.
- [ ] The application correctly handles `401` and `403` errors by attempting token refresh or redirecting to login as appropriate.
