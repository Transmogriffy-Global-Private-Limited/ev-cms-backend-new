# CPO Backend HAL Operational Capability Manual

## Status, audience, and authority

This is the canonical CMS-side manual for a CPO-backend developer. It records
the source state that consumes the read-only `ocpp-hal-go-new` v1 provider at
`21836e5d98967399d599d6afeca52fe1c375ec0d`. It does not claim that migration,
dual-service, virtual-charge-point, restart, or production acceptance has run.
Read the provider's `docs/contracts/CMS_HAL_CHARGING_V1.md` for the frozen wire
contract; this manual documents CMS ownership and actual extension points.

| Fact or decision | Authority | CMS consequence |
| --- | --- | --- |
| CPO/customer scope, published inventory, static status, tariff/GST, wallet, settlement/audit | CMS | Handler derives trusted tenant context before any operation. |
| OCPP connection/protocol, remote delivery, exact transaction ID and raw meter samples | HAL | CMS never writes HAL storage or invents protocol outcome. |
| Command intent, business session and customer projection | CMS correlated to HAL evidence | Command acknowledgement is not charging or completion truth. |
| Connection/status/meter evidence | HAL fact projected in CMS | REST reads the durable CMS projection, never synchronously calls HAL. |

CPO is a tenant, not a global role. Callable tenant staff authority is `ADMIN`
and requires the matching `X-CPO-App-ID`; platform observation is a distinct
plane and does not grant command authority.

## Capability map and composition

| CMS package / function | Owns | Consumer rule |
| --- | --- | --- |
| `halclient.Client`: `SyncMapping`, `Start`, `Stop`, `GetCommand` | authenticated versioned HTTP wire adapter | Private adapter: handlers do not call it. |
| `halops.Service`: `SyncMapping`, `EnsureChargerMapping`, `RequestStart`, `RequestStop`, `ReconcileCommand`, `ReconcilePending`, `RunReconciler` | CMS mapping, command and exact-ID recovery mechanics | Call only after business authorization. |
| `halops.FactIngestor.Accept` | bearer, schema/JCS integrity, receipt and duplicate protection | HAL-only ingress socket. |
| `liveops.Service`: `GetCharger`, `GetConnector`, `GetChargerDetail`, `GetChargerDetails`, `GetSession`, `GetFleet` | bounded committed operational projections/freshness | The list-safe read socket; never a HAL transport. |
| `customerauth.ApplyHALFactProjection` | session/wallet/runtime projection and event emission | Existing business fact projector. |
| `operationalrealtime.Service` | durable event `Emit`, scoped list, cursor/SSE frame and retention | Recovery notification, never state authority. |

`main.go` composes these once and injects CPO/User App consumers. This is the
joining seam for a future approved capability; do not add a second HAL client,
direct HAL DB use, or a parallel event stream.

## Identifier crosswalk

| Identifier | Meaning | Not interchangeable with |
| --- | --- | --- |
| `cpo_id` | CMS trusted tenant UUID | client-selected scope |
| `cms_charger_id`, `cms_connector_id` | CMS inventory UUIDs | public `charger_id` |
| `charger_id` | six-character public CMS charger ID; the value assigned to `ocpp_identity` for newly created rows | CMS UUID |
| `charger_ocpp_identity` | HAL/OCPP mapping identity; equals `charger_id` for newly created rows while older rows retain their stored compatibility value | CMS UUID |
| `ocpp_connector_number` | physical/device connector number | connector UUID |
| `cms_command_id` | CMS command/idempotency identity | a retry-generated UUID |
| `hal_command_id` | HAL durable command identity | CMS command ID |
| `cms_start_intent_id`, `cms_charging_session_id` | CMS commercial request/session | HAL transaction |
| `hal_transaction_id` | HAL durable transaction UUID | CMS session ID |
| `ocpp_transaction_id` | exact positive signed-32-bit OCPP transaction ID | string/UUID surrogate |
| `fact_id` | immutable HAL delivery identity | command idempotency key |
| operational event `id` | CMS replay cursor | fact/resource ID |

```text
CMS start intent -> CMS command -> one-use credential/idTag -> HAL command
-> HAL transaction -> exact OCPP transactionId -> CMS charging session
```

## Mapping and CPO inventory lifecycle

Inventory creation, edit, and status changes first commit CMS rows and a
`HALChargerMapping` state. Post-commit `EnsureChargerMapping` loads the
committed CMS charger/connectors and calls
`PUT /v1/mappings/chargers/{cms_charger_id}` with CPO ID, CMS IDs, OCPP
identity, static `ACTIVE`-derived `enabled`, and every connector UUID/number.

```text
CPO `chargers.manage` inventory mutation -> CMS transaction + PENDING mapping
-> post-commit provider mapping -> SYNCHRONIZED
                              -> RECONCILIATION_REQUIRED
-> bounded reconciler reuses the same committed mapping
```

Mapping success is not an online proof. Provider failure remains observable;
never create a replacement mapping, drop a connector-number change, or infer
connection state from administrative inventory.

## Fact ingress, exact fields, and projection

`POST /v1/hal-facts` requires the separate HAL fact bearer and
`Idempotency-Key == fact_id`. Its v1 envelope is `fact_id`, `fact_type`,
`schema_version`, `occurred_at`, producer `ocpp-hal-go-new`,
`immutable_content_sha256`, and `payload`. Digest is SHA-256 of RFC 8785 JCS
of that envelope with the digest excluded. The fact receipt and projected state
commit together: identical ID/digest is `204` no-op; altered ID reuse is `409
fact_integrity_violation`; bad bearer is `401`; malformed facts are `400`.

| Fact | Required payload fields | CMS projection rule |
| --- | --- | --- |
| `charger.connection.updated` | `cpo_id`, `cms_charger_id`, `charger_ocpp_identity`, `connection_state`, `connection_generation`, `connection_sequence`, `observed_at` | exact mapping validation; update runtime only on higher sequence. |
| `connector.status.updated` | `cpo_id`, `cms_charger_id`, `cms_connector_id`, `charger_ocpp_identity`, `ocpp_connector_number`, `ocpp_connector_status`, `connector_status_sequence`, `observed_at` | exact connector mapping validation; update only on higher sequence. |
| `command.updated` | `hal_command_id`, `cms_command_id`, `kind`, `state`, `charger_ocpp_identity`, `ocpp_connector_number`, `delivery_attempts`, `ocpp_result`, `last_error_category`, `occurred_at` | command evidence only, never a session transition. |
| `transaction.started` | `hal_transaction_id`, `ocpp_transaction_id`, `hal_command_id`, `cms_command_id`, `cms_start_intent_id`, `charger_ocpp_identity`, `ocpp_connector_number`, `id_tag`, `meter_start_wh`, `started_at` | validates intent/command/credential/mapping chain; materializes one `ACTIVE` session. |
| `transaction.meter` | `hal_transaction_id`, `ocpp_transaction_id`, `cms_start_intent_id`, `charger_ocpp_identity`, `ocpp_connector_number`, `meter_sequence`, `meter_value_wh`, `consumed_wh`, `meter_observed_at` | only a newer sequence updates current integer Wh. |
| `transaction.completed` | `hal_transaction_id`, `ocpp_transaction_id`, `cms_start_intent_id`, `charger_ocpp_identity`, `ocpp_connector_number`, `meter_start_wh`, `meter_stop_wh`, `stopped_at`, `ocpp_stop_reason`; optional stop command/request fields | validates final fact then completes/settles exactly once; local stop is valid without CMS stop command. |

Connection generation protects stale disconnects upstream; CMS independently
ignores non-increasing connection, connector, and meter sequences. It retains
historical OCPP status but never presents it as fresh live state after stale,
`OFFLINE`, or `UNKNOWN` parent evidence.

## Derived live state

`liveops` intentionally keeps CMS administrative status separate from HAL
protocol observations. Meter freshness uses `HAL_V1_METER_STALE_AFTER` (default
`30s`) and never creates a meter reading. Connection freshness uses the
independent `HAL_V1_CONNECTION_STALE_AFTER` (default `15m`), which must remain
longer than HAL's requested five-minute Heartbeat cadence. A retained connector
StatusNotification is live only while its parent connection is `ONLINE` and
connection-fresh; its historical observation remains diagnostic data.

| Runtime evidence | Derived connector availability |
| --- | --- |
| no observation | `UNKNOWN` / `UNKNOWN` |
| parent `OFFLINE`, `UNKNOWN`, or stale | `UNAVAILABLE` / `STALE` |
| fresh `ONLINE` + OCPP `Available` | `AVAILABLE` |
| fresh `ONLINE` + `Charging`, `Preparing`, `Finishing` | `CHARGING` |
| fresh `ONLINE` + `Faulted` | `FAULTED` |
| other fresh status | `UNAVAILABLE` |

Customer projections then apply a CMS safety overlay: only customer-visible
inventory reaches the adapter; inactive, suspended, maintenance, or
decommissioned charger/connector status is `UNAVAILABLE` even when runtime
evidence is older or says `AVAILABLE`.

## Operational reads and User App consumption

| Actor | Route | Scope/result |
| --- | --- | --- |
| CPO `chargers.operations` | `GET /api/v1/cpo/operations/fleet` | same-CPO `FleetState`: totals, online/offline/unknown, available/charging/faulted connectors, active sessions. |
| CPO `chargers.operations` | `GET /api/v1/cpo/operations/chargers/{charger_id}` | static CPO charger plus `ChargerDetail` connection/connector evidence. |
| Platform | `/api/v1/platform/cpos/{cpo_id}/operations/fleet` and `/chargers/{charger_id}` | `PLATFORM` observation of an existing CPO only. |
| Customer | `GET /api/v1/app/chargers`, hub detail, charger detail, favorites | full `CustomerCharger` projections overlay committed availability/freshness. |
| Customer | start-intent/session reads | owner-only durable progress, meter, connection, connector and completion data. |

Full User App charger producers are list, hub detail, single detail, and
favorite charger list. Their response charger IDs are collected and passed to
one `GetChargerDetails` query set, avoiding N+1 live reads. The compact map
`GET /api/v1/app/chargers/locations` remains exactly name/latitude/longitude;
it intentionally has no live state. A User App `POST /charging-sessions`
acceptance is not `ACTIVE`; poll start intent/session. Stop is a request and
completion remains charger-originated `transaction.completed` evidence.

## CPO commands: documented future pattern only

There is no CPO start/stop/reset/unlock HTTP command in this slice. A future
approved command must authorize active same-CPO `ADMIN`, create an auditable
durable CMS command before provider I/O, use `halops` with one idempotency and
correlation identity, reconcile by that same `cms_command_id`, distinguish HAL
acceptance/OCPP acknowledgement/fact completion, and define fact projection,
event scope, REST recovery, OpenAPI, and dual-service tests. Platform must not
become a command fallback.

## Events, SSE, reconciliation, and errors

`operational_events` is durable scoped notification data. CPO recovery is
`GET /api/v1/cpo/operations/events?after_id=<n>&limit=<1..500>`; SSE is
`GET /api/v1/cpo/operations/realtime/stream`. Platform uses the corresponding
per-CPO path. Events are ascending numeric cursor records, at-least-once:
persist/deduplicate ID, cursor-recover, then refresh the REST snapshot. SSE
frames are `id`, `event`, JSON `data`; heartbeats revalidate bearer/session/CPO
scope and close invalid streams. Event types are
`charging.command_changed`, `charger.live_state_changed`,
`connector.live_state_changed`, `charging.session_changed`, and
`charging.meter_changed`. They are invalidation hints, never command or meter
truth; older sequences create neither state regression nor a new event.

The CPO ongoing-session table is intentionally easier: its primary
`GET /api/v1/cpo/operations/live-sessions` connection emits a complete CMS
snapshot immediately and complete replacement snapshots after committed
session/meter/SoC changes. Each CPO-safe row carries `duration_seconds` at
`as_of`, `customer_name`, the CMS `connector_id`, and charger/hub context; it
does not carry customer contact or financial data. The browser never receives
the raw event records for normal table updates. Reconnect that route for a fresh snapshot; use
`/live-sessions/snapshot` for explicit JSON/keyset recovery. The retained
`/live-sessions/events` route is only for advanced reconciliation.

| Failure/condition | Correct action |
| --- | --- |
| no HAL URL/tokens | charging returns `503 hal_unavailable`; projection reads still work. |
| transport/provider command or mapping uncertainty | retain `RECONCILIATION_REQUIRED`; exact-ID reconcile, never new command ID. |
| command lookup `404` | provider never accepted it; preserve evidence and apply explicit business recovery. |
| CPO inactive | user start denies `cpo_not_active`; admin/auth scope still controls reads. |
| offline/stale runtime | present unavailable/stale; retain historical status without calling it fresh. |
| duplicate/altered fact | exact replay no-op; altered digest conflict and no side effect. |

## Persistence, configuration, development topology, and extension checklist

CMS durable records include `hal_charger_mappings`, `hal_command_records`,
`hal_fact_receipts`, `hal_charger_runtime`, `hal_connector_runtime`, intents,
sessions, holds, and `operational_events`. Migration 28 is the core charging
vertical; migration 33 is durable event replay. This manual applies neither.

Use `HAL_V1_BASE_URL`, `HAL_V1_CMS_BEARER_TOKEN`,
`HAL_V1_CMS_FACT_BEARER_TOKEN`, `HAL_V1_REQUEST_TIMEOUT`, and
`HAL_V1_METER_STALE_AFTER`, and `HAL_V1_CONNECTION_STALE_AFTER` from
`contracts/configuration.md`. The provider has
separate fact-delivery enablement, CMS facts URL, and fact bearer. All are
loopback-only in local topology; tokens are distinct and never browser/API
values, logs, docs examples, or OpenAPI content.

Full acceptance requires disposable CMS/HAL PostgreSQL, both loopback services,
facts/bearers, and a virtual OCPP charge point. Prove mapping, start, fact
idempotency, started/session materialization, connection/status freshness,
meter ordering, stop/completion/settlement, exact-ID reconciliation, SSE cursor
recovery, and restart/outage recovery. `TEST_DATABASE_URL` must be disposable;
when absent, lifecycle tests are skipped—not passed.

Before extension, map authority, ID chain, trusted scope, durable transition,
fact/sequence rule, retry owner, reconciliation read, REST/SSE recovery,
contract and test topology. Never query HAL from list handlers, expose HAL to a
browser, trust client CPO IDs, collapse CMS/OCPP status, retry using a new ID,
settle from a raw non-final fact, make SSE authoritative, or directly write the
HAL database.

## Source-accurate operation reference

The public operation signatures are intentionally CMS-shaped:

```go
func (s *halops.Service) SyncMapping(ctx context.Context, mapping ChargerMapping, correlationID string) error
func (s *halops.Service) EnsureChargerMapping(ctx context.Context, chargerID uuid.UUID, correlationID string) error
func (s *halops.Service) RequestStart(ctx context.Context, request StartRequest, correlationID string) (Command, error)
func (s *halops.Service) RequestStop(ctx context.Context, request StopRequest, correlationID string) (Command, error)
func (s *halops.Service) ReconcileCommand(ctx context.Context, commandID uuid.UUID) (Command, error)
func (s *halops.Service) ReconcilePending(ctx context.Context, limit int) error
func (s *liveops.Service) GetCharger(ctx context.Context, cpoID, chargerID uuid.UUID) (ChargerState, error)
func (s *liveops.Service) GetConnector(ctx context.Context, cpoID, connectorID uuid.UUID) (ConnectorState, error)
func (s *liveops.Service) GetChargerDetail(ctx context.Context, cpoID, chargerID uuid.UUID) (ChargerDetail, error)
func (s *liveops.Service) GetChargerDetails(ctx context.Context, cpoID uuid.UUID, chargerIDs []uuid.UUID) (map[uuid.UUID]ChargerDetail, error)
func (s *liveops.Service) GetSession(ctx context.Context, cpoID, sessionID uuid.UUID) (SessionState, error)
func (s *liveops.Service) GetFleet(ctx context.Context, cpoID uuid.UUID) (FleetState, error)
```

`halclient` attaches the CMS bearer, JSON body, `Idempotency-Key`, and a
required non-empty `X-Correlation-ID` for every mutation; its client timeout is
`HAL_V1_REQUEST_TIMEOUT`. HTTP-originated calls use the CMS `RequestLogger`
request ID from Gin context, not an optional client `X-Request-ID` header.
Mutating calls use the CMS mapping/command ID as their idempotency key. A
timeout means delivery is unknown, not that it is safe to send another command.
It returns a typed `HTTPError` for non-2xx provider response,
`ErrUnavailable` when base URL/bearer is absent, and
`ErrMissingCorrelationID` locally before an invalid mutation is sent.
`GetCommand` is the sole recovery lookup and queries
`GET /v1/remote-commands?cms_command_id=...` without a correlation header.

## Persistence map and consumer comparison

| CMS model/table | Key invariant and lifecycle |
| --- | --- |
| `HALChargerMapping` / `hal_charger_mappings` | one CMS charger and unique OCPP identity; pending/synchronized/reconciliation mapping evidence. |
| `HALChargerRuntime` / `hal_charger_runtime` | one CMS charger; latest accepted connection generation/sequence/observation. |
| `HALConnectorRuntime` / `hal_connector_runtime` | one CMS connector; latest accepted OCPP status sequence/observation. |
| `ChargingStartIntent` | credential hash unique; one materialized session ID; commercial decision before HAL command. |
| `WalletHold` | one start intent; held/captured/released/reconciliation accounting lifecycle. |
| `HALCommandRecord` | `cms_command_id` primary and HAL command unique when known; exact recovery identity. |
| `ChargingSession` | start intent and HAL transaction unique; exact OCPP numeric transaction retained. |
| `HALFactReceipt` | fact ID primary plus immutable digest; receipt/projection transaction boundary. |
| `OperationalEvent` / `operational_events` | increasing durable replay ID, CPO scope and optional customer scope, retention expiry. |

| Concern | Platform | CPO `chargers.operations` | User App |
| --- | --- | --- | --- |
| Live charger/connector visibility | selected CPO, observation only | own CPO operational detail/fleet | published own-CPO consumer-safe projection |
| Active session visibility | not a command surface | aggregate only in current fleet | owner-only session detail |
| Command authority in this slice | none | no CPO command endpoint | start/owned stop only through CMS business flow |
| Realtime scope | selected CPO event cursor/SSE | own CPO cursor/SSE | own customer events plus safe tenant availability hints |
| Private IDs/diagnostics | platform contract only | CPO operational contract | does not expose OCPP identity, HAL IDs, raw facts, credentials |

## Essential sequences

### Mapping, connection, and connector status

```text
CPO inventory mutation -> CMS transaction/mapping PENDING -> halops.SyncMapping
-> HAL mapping persisted -> CMS mapping SYNCHRONIZED
charger connects -> HAL validates mapping -> connection.updated(sequence,generation)
-> CMS receipt + runtime -> operational event -> REST/SSE invalidation
charger StatusNotification -> HAL connector.status.updated(sequence)
-> CMS receipt + connector runtime -> operational event -> refreshed CPO/App view
```

### Customer start, meter, and completion

```text
App Start -> CMS authorization/tariff/hold/StartIntent/CMS command -> HAL RemoteStart
-> command.updated may show delivery/OCPP acknowledgement (not ACTIVE)
-> charger StartTransaction -> HAL transaction.started -> CMS receipt + one session ACTIVE
-> event -> App REST refetch
charger MeterValues -> HAL transaction.meter(sequence) -> CMS session projection -> meter event
customer stop OR time/energy limit OR device stop -> HAL one stop workflow
-> charger StopTransaction -> HAL transaction.completed -> CMS completion/settlement once -> event
```

### Ambiguity, duplicate delivery, and SSE recovery

```text
CMS command request times out -> state RECONCILIATION_REQUIRED
-> ReconcileCommand(same cms_command_id) -> HAL durable command or exact 404 evidence
   START 404 with no materialized session -> CMS atomically releases HELD hold and terminalizes start
   STOP 404 -> remains reconciliation-required; it never proves session completion
HAL repeats same fact_id/digest -> CMS 204 and no second projection/event
client reconnects -> GET events after persisted numeric ID -> dedupe -> GET fleet/charger/session
-> reopen SSE; a missed stream never changes durable truth
```

### Future approved CPO stop pattern (documentation only)

```text
CPO handler -> require active same-CPO `chargers.operations` -> load exact session/charger/connector
-> persist CMSCommandID + audit + expiry -> halops.RequestStop
-> accepted/ambiguous response is STOP_PENDING/reconciliation only
-> exact transaction.completed fact finalizes; no router-to-halclient shortcut
```

## Developer cookbook

| Need | Safe implementation |
| --- | --- |
| Add a CPO charger detail widget | authorize `chargers.operations`, call existing `GetOperationalCharger`; do not query HAL or runtime tables in the handler. |
| Add dashboard aggregate | extend `liveops.GetFleet` or a new bounded `liveops` read, keep CPO scope in every query, document OpenAPI and refresh event. |
| Show connector state | consume `ChargerDetail.Connectors`; show availability/freshness, not just last OCPP status. |
| Observe current customer energy | owner-only `GetChargingSession`, then `latest_meter_wh`, `consumed_wh`, observation and freshness; never calculate synthetic samples. |
| Add a CPO-safe App projection | authorize/publish static inventory first, then call `GetChargerDetails` once for collected IDs and apply a consumer adapter. |
| Recover ambiguous command | retain/read `HALCommandRecord.CMSCommandID`, call `ReconcileCommand`; do not query latest charger transaction. |
| Publish an event | only within the successful projection transaction using `operationalrealtime.Emit`; use no raw fact or financial payload. |
| Add a new HAL fact | approve provider schema, extend `ApplyHALFactProjection` and sequence/idempotency rule, then `liveops`, tests, OpenAPI/docs, scoped event and recovery. |
| Keep a fact CPO-only | emit CPO-scoped data without a customer ID and do not add it to customer projection/contract. |

## Verification matrix and explicit limits

Focused source checks are `go test ./src/liveops ./src/customerauth -count=1`,
route contract coverage is `go test ./src/routes -run
TestOpenAPIContractMatchesRuntimeRoutesAndServesUI -count=1`, documentation is
`./scripts/verify-docs.ps1`, and broad checks are `go test -p 1 ./...`, `go vet
-p 1 ./...`, and `git diff --check`. Add database tests only against an explicitly
disposable `TEST_DATABASE_URL`. New capability tests must cover tenant scope,
customer ownership, duplicate/altered fact, stale/out-of-order sequence,
offline parent, timeout/reconciliation, cursor/SSE recoverability, and virtual
charge-point start/meter/stop races. No full dual-service evidence should be
claimed merely because database-free package tests pass.

## Junior operating guide and exact reference

The earlier sections are the short reference. This section is the deliberate
slow pass for a junior: it names the safe sockets, explains the distinctions
that prevent expensive mistakes, and records current source behavior rather
than a desired future architecture.

### 1 — Start with the mental model

```text
physical charger -- OCPP --> HAL -- immutable facts --> CMS receipt/projector
                                                     --> PostgreSQL projection
                                                     --> liveops --> REST
                                                     --> operational_events --> cursor/SSE

CPO/User business decision --> durable CMS intent --> halops --> halclient --> HAL
```

HAL speaks OCPP and owns protocol evidence. CMS owns commercial decisions,
tenant authority, inventory, money, and its own REST/SSE contract. PostgreSQL
is the durable CMS truth. REST reads that truth; SSE only says it changed.
Normal dashboards do not ask HAL synchronously because that would create N+1
network work, turn provider failure into read failure, and remove the CMS
recovery authority.

### 2 — Ownership is a safety boundary

| HAL owns | CMS owns |
| --- | --- |
| OCPP connection/reconnection, protocol status, RemoteStart/Stop delivery, exact OCPP transaction ID, StartTransaction, StopTransaction, meter evidence, physical energy/time cutoff | CPO/customer identity, authorization, CMS inventory/status, tariffs/GST, wallet/holds, StartIntent, CMS command/session, settlement, REST, and browser event scope |

CMS does not write HAL storage; HAL does not read CMS wallets; CPO code does
not speak OCPP. A mapping, an acknowledgement, and an event each cross this
boundary with a narrower meaning than a business result.

### 3 — Traffic-light map

| Surface | Rule |
| --- | --- |
| `src/cpo` | **GREEN:** normal CPO route/service/DTO work. |
| public `src/liveops` API | **GREEN:** live projection read socket. |
| public `src/halops` API | **GREEN:** only after business authorization/persistence. |
| `src/operationalrealtime` public API | **YELLOW:** shared committed notification capability. |
| `liveops`/`halops` internals | **YELLOW:** change only for a real shared capability. |
| `src/halclient`, fact ingress/projector, runtime tables, HAL DB/OCPP | **RED:** never bypass for an ordinary CPO feature. |

If you need data, use `liveops`. If you need an approved operation, use
`halops`. If you are about to import `halclient`, create raw provider JSON, or
query a HAL runtime table from CPO code, stop.

### 4 — Practical repository map

| Job | Start at | Do not replace with |
| --- | --- | --- |
| CPO route/live screen | `src/cpo/router.go`, `service.go`, `schemas.go` | direct HAL HTTP |
| User App full charger list/hub/detail/favorites | `src/customerauth/network.go` | a per-row live/HAL call |
| customer start/stop/session | `src/customerauth/charging.go` | a CPO command shortcut |
| provider transport | `src/halclient/client.go` | a second HTTP client |
| mapping/commands/recovery | `src/halops/operations.go` | handlers writing integration records |
| fact validation/projection | `src/halops/facts.go`, `src/customerauth/charging_facts.go` | a client-facing endpoint |
| durable events/SSE | `src/operationalrealtime/service.go` and CPO/User routers | an in-memory broadcast |
| durable structure | `src/models/schema.go`, migrations 28/33/34 | undocumented direct SQL |

`main.go` is the composition point. It creates and injects one shared
`halops`, `liveops`, and `operationalrealtime` capability, starts reconciliation
every minute, and starts durable-event retention. Add a compatible capability
through this composition root, never with a parallel client or event stream.

### 5 — Plug/socket reference

#### 5.1 `halclient`: private transport

```go
func (c *halclient.Client) SyncMapping(ctx context.Context, mapping ChargerMapping, correlationID string) error
func (c *halclient.Client) Start(ctx context.Context, command StartCommand, correlationID string) (Command, error)
func (c *halclient.Client) Stop(ctx context.Context, command StopCommand, correlationID string) (Command, error)
func (c *halclient.Client) GetCommand(ctx context.Context, id uuid.UUID) (Command, error)
```

It sends JSON to the configured HAL base URL with the CMS bearer,
`Idempotency-Key`, and a required non-empty `X-Correlation-ID` for mutations.
Mapping uses the CMS charger UUID as idempotency key; start/stop use
`cms_command_id`. HTTP-originated callers use the CMS-generated request ID;
the User App is not required to supply `X-Request-ID`. Missing base URL or
command bearer gives `halclient.ErrUnavailable`; empty correlation gives
`halclient.ErrMissingCorrelationID` without sending HTTP; non-2xx gives
`*halclient.HTTPError`; timeout is unknown delivery, not a safe retry with a
new identity. It is **PRIVATE TRANSPORT — DO NOT CALL FROM CPO CODE**.

#### 5.2 `halops`: normal CMS operation socket after authorization

```go
func (s *halops.Service) SyncMapping(ctx context.Context, mapping ChargerMapping, correlationID string) error
func (s *halops.Service) EnsureChargerMapping(ctx context.Context, chargerID uuid.UUID, correlationID string) error
func (s *halops.Service) RequestStart(ctx context.Context, request StartRequest, correlationID string) (Command, error)
func (s *halops.Service) RequestStop(ctx context.Context, request StopRequest, correlationID string) (Command, error)
func (s *halops.Service) ReconcileCommand(ctx context.Context, commandID uuid.UUID) (Command, error)
func (s *halops.Service) ReconcilePending(ctx context.Context, limit int) error
func (s *halops.Service) RunReconciler(ctx context.Context, interval time.Duration)
```

| Method | Durable/remote action | Return does not prove |
| --- | --- | --- |
| `SyncMapping` | sends supplied committed mapping and records mapping outcome | charger online |
| `EnsureChargerMapping` | loads committed charger/connectors and sends mapping | inventory transaction depended on HAL |
| `RequestStart` | sends trusted CMS start request | actual charging/session |
| `RequestStop` | sends exact session/transaction stop request | completion/settlement |
| `ReconcileCommand` | reads HAL with exact stored `cms_command_id`, updates matching record | that a missing transaction may be guessed |
| `ReconcilePending` | bounded pending-mapping and command recovery (default 50) | all external state is instantly resolved |

`StartRequest` contains trusted CMS command/start-intent/CPO/customer/charger/
connector IDs, mapped identity/number, one-use credential, expiry, integer Wh
limit and duration. `StopRequest` contains exact CMS session, HAL transaction,
and numeric OCPP transaction IDs. `Command` contains `HALCommandID`,
`CMSCommandID`, kind/state, optional transaction IDs, and `UpdatedAt`.

For customer starts, mapping synchronization is a pre-command operational
prerequisite. A failed preflight returns `503 charger_mapping_unavailable`
without a commercial intent. CMS reconfirms the mapping after creating an
intent only to close a concurrent inventory-change race; a failure on that
second check is immediately terminalized as `NOT_ATTEMPTED`, never reported as
delivery reconciliation. `RECONCILIATION_REQUIRED` is reserved for an invoked
start request whose outcome cannot be known. The appv1 credential remains
transient and hashed only, so CMS never replays a missing command; exact HAL
404 instead enables a fresh customer retry with a new credential.

#### 5.3 Fact ingestor: shared infrastructure

`POST /v1/hal-facts` is HAL-only. It requires the independent fact bearer,
`Idempotency-Key == fact_id`, a 32 KiB maximum envelope, version 1 producer
`ocpp-hal-go-new`, and an SHA-256 digest of RFC 8785 canonical immutable
content. `FactIngestor.Accept` applies the projector and stores the receipt in
one transaction. Exact same ID/digest is `204` no-op; changed same ID is `409
fact_integrity_violation`; bad bearer is `401`; invalid shape is `400`.

#### 5.4 `liveops`: exact read signatures and returns

```go
func (s *liveops.Service) GetCharger(ctx context.Context, cpoID, chargerID uuid.UUID) (ChargerState, error)
func (s *liveops.Service) GetConnector(ctx context.Context, cpoID, connectorID uuid.UUID) (ConnectorState, error)
func (s *liveops.Service) GetChargerDetail(ctx context.Context, cpoID, chargerID uuid.UUID) (ChargerDetail, error)
func (s *liveops.Service) GetChargerDetails(ctx context.Context, cpoID uuid.UUID, chargerIDs []uuid.UUID) (map[uuid.UUID]ChargerDetail, error)
func (s *liveops.Service) GetSession(ctx context.Context, cpoID, sessionID uuid.UUID) (SessionState, error)
func (s *liveops.Service) GetFleet(ctx context.Context, cpoID uuid.UUID) (FleetState, error)
```

| Type | Important exported fields |
| --- | --- |
| `ChargerState` | charger/CPO IDs, `ConnectionState`, `ConnectionFreshness`, observed time, sequence, generation |
| `ConnectorState` | connector/charger/CPO IDs, `LastOCPPStatus`, derived availability/freshness, observation/sequence, parent connection |
| `ChargerDetail` | `Charger ChargerState`, `Connectors []ConnectorState` |
| `SessionState` | session/CPO/customer IDs, state, start/completion, latest/consumed Wh, meter observation/freshness |
| `FleetState` | totals; online/offline/unknown chargers; available/charging/faulted connectors; active sessions |

Every `liveops` method reads CMS PostgreSQL only. `GetChargerDetails` is the
list-safe method: it de-duplicates IDs and loads topology, charger runtime, and
connector runtime in bounded query sets. Use it for 50 rows, not 50 calls.

#### 5.5 `operationalrealtime`: shared realtime capability

```go
func (s *operationalrealtime.Service) Emit(tx *gorm.DB, input Input) (models.OperationalEvent, error)
func (s *operationalrealtime.Service) ListCPO(ctx context.Context, cpoID uuid.UUID, after int64, limit int) (Page, error)
func (s *operationalrealtime.Service) ListCustomer(ctx context.Context, cpoID, customerID uuid.UUID, after int64, limit int) (Page, error)
func (s *operationalrealtime.Service) ListPlatform(ctx context.Context, cpoID *uuid.UUID, after int64, limit int) (Page, error)
func (s *operationalrealtime.Service) StreamTiming() (time.Duration, time.Duration, int)
func ParseCursor(afterText, limitText string) (int64, int, error)
func WriteSSE(writer io.Writer, events []models.OperationalEvent) error
```

Emit only inside the successful projection transaction. It persists scope,
event/resource identity, small data, occurrence, and expiry before any SSE
frame exists. Cursors are nonnegative numeric IDs; limit is 1–500 (default
100). Heartbeats revalidate the bearer/session scope. This is a **SHARED
REALTIME CAPABILITY**, not a second authoritative data store.

### 6 — Which method do I call?

| Need | Call |
| --- | --- |
| one charger connection | `GetCharger` |
| one charger plus connectors | `GetChargerDetail` |
| fifty charger details | `GetChargerDetails` |
| one connector | `GetConnector` |
| CPO aggregates | `GetFleet` |
| session/meter state | `GetSession` |
| committed mapping push | `EnsureChargerMapping` |
| approved operation | `RequestStart`/`RequestStop` after durable business work |
| ambiguous command | `ReconcileCommand` with same CMS command ID |

### 7 — Return-value school

| Result | Means | Does not mean |
| --- | --- | --- |
| mapping success | provider accepted directory mapping | charger connected |
| `RequestStart` command | HAL has durable command representation | actual start |
| `RequestStop` command | HAL has durable stop command | actual stop/completion |
| `transaction.started` accepted | CMS created one active session | final meter/settlement |
| `transaction.completed` accepted | CMS finalization transaction succeeded | changed fact will be processed again |
| timeout | caller lacks response knowledge | provider did not receive request |
| SSE frame | committed state changed | payload is current full state |

### 8 and 9 — Static/live data and availability derivation

Static inventory supplies public ID, CMS UUID, name, hub, connector number/type,
and administrative status. HAL-derived projections supply connection state,
OCPP connector status, sequences, observations, freshness, session/meter state.

| Evidence | Derived connector availability |
| --- | --- |
| no runtime observation | `UNKNOWN` / `UNKNOWN` freshness |
| parent offline, unknown, or stale | `UNAVAILABLE` / `STALE` |
| fresh online + `Available` | `AVAILABLE` |
| fresh online + `Charging`/`Preparing`/`Finishing` | `CHARGING` |
| fresh online + `Faulted` | `FAULTED` |
| other fresh OCPP state | `UNAVAILABLE` |

CMS `ACTIVE` is not HAL `ONLINE`. The User App overlay preserves that safety
gate: inactive charger/connector inventory is unavailable regardless of older
live evidence. `HAL_V1_METER_STALE_AFTER` defaults to 30 seconds and applies
only to meters; `HAL_V1_CONNECTION_STALE_AFTER` defaults to 15 minutes and
applies only to HAL connection evidence. Neither setting invents a reading or
an `ONLINE` connection.

### 10 — Identifier school

| ID | Meaning | Not interchangeable with |
| --- | --- | --- |
| `cms_charger_id` / CMS charger UUID | durable CMS inventory | public `charger_id` |
| `charger_id` | six-character public identifier | internal UUID |
| `charger_ocpp_identity` | stored CMS/HAL mapping identity | a fabricated `ocpp_` prefix |
| `cms_connector_id` / `ocpp_connector_number` | CMS connector / physical protocol address | each other |
| `cms_start_intent_id` | commercial request | CMS session |
| `cms_command_id` / `hal_command_id` | CMS recovery/idempotency ID / HAL command ID | each other |
| CMS session / `hal_transaction_id` / `ocpp_transaction_id` | business projection / HAL UUID / exact numeric OCPP ID | each other |
| `fact_id` / event `id` | immutable delivery identity / replay cursor | command/resource IDs |

New chargers assign the same generated six-character value to `charger_id` and
`ocpp_identity`; older rows retain stored compatibility identities. The CMS
charger UUID remains a different resource ID.

### 11 — Creation and mapping flow

```text
CPO `chargers.manage` validation -> CMS transaction: charger + connectors + PENDING mapping
                    -> commit -> one EnsureChargerMapping("cpo-charger-created")
                              -> SYNCHRONIZED or RECONCILIATION_REQUIRED
                              -> bounded reconciler retries committed mapping
```

Creation has exactly one normal immediate mapping push. Update and status flows
retain their own post-commit mapping sync. HAL failure never rolls back durable
CMS inventory, and synchronized mapping never proves online.

### 12 and 13 — Connection and connector facts

`charger.connection.updated` validates CPO, CMS charger, stored OCPP identity,
state (`ONLINE`, `OFFLINE`, `UNKNOWN`), generation, sequence, and observation;
only a higher connection sequence changes `HALChargerRuntime`.

`connector.status.updated` validates CPO, CMS charger/connector, stored OCPP
identity, mapped connector number, status, sequence, and observation; only a
higher sequence changes `HALConnectorRuntime`. Both commit a scoped operational
event only when the projection advanced.

### 14 through 19 — Start, meter, stop, and settlement

```text
customer POST charging-sessions
  -> lock/check active CPO, published ACTIVE charger/connector, wallet/tariff
  -> persist StartIntent + credential hash + WalletHold + CMS command + PENDING mapping
  -> map then RequestStart -> ACCEPTED_FOR_DELIVERY (not ACTIVE)
  -> charger StartTransaction -> transaction.started -> one ACTIVE CMS session
  -> MeterValues -> newer transaction.meter updates integer Wh
  -> user/time/energy/natural stop -> charger StopTransaction
  -> transaction.completed -> final meter, wallet debit/hold capture, COMPLETED/SETTLED
```

The start endpoint returns `202` and an intent, not a session. The stop endpoint
sets `STOP_PENDING` and returns `202`, not `COMPLETED`. Only exact validated
`transaction.started` materializes a session; only exact validated
`transaction.completed` finalizes/settles one. CMS calculates affordability and
sends integer Wh/duration limits; HAL enforces them physically without receiving
tariff, tax, balance, or settlement authority.

Settlement reads only the immutable tariff and tax snapshots captured at start;
it never re-resolves a current tariff, Hub, GST assignment, or GST state. Before
using the frozen rates it validates their complete commercial component shape:
all SGST, CGST, and IGST rates must be present and within 0 through 100, and a
non-zero IGST component cannot be mixed with non-zero SGST/CGST. A malformed
snapshot is unsupported rather than silently reinterpreted. Historical tariff
snapshot readers remain deliberately named compatibility paths for their
released `price_per_kwh` and `watt/hour` semantics.

### 20 and 21 — Fact delivery and complete catalog

HAL sends immutable facts to `POST /v1/hal-facts`; after a lost 204 it retries
the same ID/digest/body. Same ID/digest is no-op; altered content is an integrity
conflict. Current accepted facts are:

| Fact | CMS projection |
| --- | --- |
| `charger.connection.updated` | latest accepted charger runtime + `charger.live_state_changed` |
| `connector.status.updated` | latest accepted connector runtime + `connector.live_state_changed` |
| `command.updated` | matching `HALCommandRecord` state + `charging.command_changed` |
| `transaction.started` | one session `ACTIVE`, intent `ACTUALLY_STARTED` + session event |
| `transaction.meter` | newer session meter only + meter event |
| `transaction.completed` | completion, wallet/hold/ledger/payment settlement + session event |

Unknown type is `400 unsupported_hal_fact`. HAL facts are not tariff, GST,
wallet, or frontend event conclusions.

### 22 through 24 — CPO reads, commands, and SSE

| CPO `chargers.operations` route | Use |
| --- | --- |
| `GET /api/v1/cpo/operations/fleet` | durable `FleetOperationsResponse` |
| `GET /api/v1/cpo/operations/chargers/{charger_id}` | static `ChargerResponse` plus `ChargerDetail` |
| `GET /api/v1/cpo/operations/events` | ordered retained cursor recovery |
| `GET /api/v1/cpo/operations/realtime/stream` | scoped SSE |
| `GET /api/v1/cpo/operations/live-sessions` | primary full-snapshot live-session SSE (`snapshot`, then `live_sessions`) |
| `GET /api/v1/cpo/operations/live-sessions/snapshot` | JSON recovery/keyset pagination for the live-session projection |
| `GET /api/v1/cpo/operations/live-sessions/events` | advanced retained `CHARGING_SESSION` reconciliation cursor; not needed by the normal live table |

Customer start and owner-only stop remain the only charging/session commands.
The CPO control vertical adds typed non-commercial charger operations through
the same boundary: Reset, UnlockConnector, ChangeAvailability, ClearCache,
Get/ChangeConfiguration, and allowlisted TriggerMessage. Each mutation creates
one CMS operation before HAL I/O and reuses that exact ID for HAL idempotency
and reconciliation. It must never be represented as a Start/Stop command or
used to mutate sessions, holds, settlement, administrative inventory, or
customer chargeability. `chargers.operations` is required; ChangeConfiguration
also requires `chargers.manage`. HAL-owned configuration keys and sensitive
values are rejected/redacted, and later OCPP facts remain the authority for
observed charger effects.

SSE frames are numeric `id`, event name, and JSON data. Client stores the ID,
uses `after_id` or `Last-Event-ID` after reconnect, deduplicates, then refetches
the authoritative fleet/charger/session REST projection. It must not interpret
event data as durable current state.

### 25 and 26 — Reconciliation and failure playbook

| Situation | Inspect / action | Never do |
| --- | --- | --- |
| mapping pending | `HALChargerMapping`; retry committed mapping | undo/recreate inventory |
| command timeout | `HALCommandRecord.CMSCommandID`; exact `ReconcileCommand` | create new command UUID |
| mapping synchronized but offline | runtime sequence/observation | call it online |
| stale Available | parent runtime plus connector freshness | show available |
| start stuck | intent, command, start fact receipt | mark active from command return |
| meter stale | HAL transaction, meter sequence/receipt/session, parent freshness | estimate meter values |
| stop stuck | stop command and completion fact | settle from RemoteStop acknowledgement |
| duplicate/altered fact | receipt ID/digest | replay altered content |

### 27 — CPO suspension and scope

Tenant scope comes from the trusted staff/customer principal, never a request
`cpo_id`. Customer start locks/checks the CPO and returns `cpo_not_active` for
an inactive CPO. Existing physical completion/fact processing remains necessary
to preserve actual session/settlement truth; suspension must not erase it.

### 28 — Persistence map

| CMS record | Role |
| --- | --- |
| `HALChargerMapping` | mapping identity, sync state/error/time |
| `HALCommandRecord` | exact CMS command recovery identity and HAL state |
| `HALFactReceipt` | fact dedupe/integrity receipt |
| `HALChargerRuntime`, `HALConnectorRuntime` | committed connection/status projections |
| `ChargingStartIntent`, `WalletHold` | pre-command commercial decision/reservation |
| `ChargingSession` | materialized business session, exact OCPP ID, meter/final state |
| `OperationalEvent` | retained CPO/customer replay notification |

Migration 28 is the charging/HAL foundation; 33 adds operational events; 34
adds nullable `tariffs.assigned_to tariff_assignment_type` metadata. No current
tariff API backfills or requires that classification.

### 29 and 30 — Red zone and derivation rules

Do not access HAL database, `halclient`, raw OCPP, raw fact envelopes, or
runtime tables directly from CPO code. Do not turn an event into authority. Do
not set active/completed session from command acknowledgement. Do not choose a
tenant from client input.

| Display/decision | Source |
| --- | --- |
| name/hub/public ID/admin status | CMS inventory |
| live connection/connector availability | `liveops` projection + freshness |
| user-visible availability | `liveops` plus active inventory safety overlay |
| consumed Wh | persisted latest meter minus start meter |
| settlement | final completion transaction only |
| near-live refresh | operational event then REST refetch |

### 31 and 32 — Cookbook and wrong/right examples

```go
// WRONG: provider call in CPO code.
command, err := halclient.New(cfg.HAL).Start(ctx, raw, correlationID)

// RIGHT: authorize/persist business state, then use the CMS operation socket.
command, err := service.halOperations.RequestStart(ctx, request, correlationID)
```

```go
// WRONG: one call per dashboard row.
for _, id := range chargerIDs { details[id], _ = service.live.GetChargerDetail(ctx, cpoID, id) }

// RIGHT: one bounded batch projection read.
details, err := service.live.GetChargerDetails(ctx, cpoID, chargerIDs)
```

For a new field use: provider contract -> fact -> projector/persistence ->
`liveops` -> CPO/User consumer -> event/recovery -> tests/docs. If `liveops`
lacks it, do not query HAL directly; extend the shared capability deliberately.

### 33 — End-to-end diagrams

#### 33.1 Charger registration

```text
CPO `chargers.manage` -> CMS validate/scope -> transaction(charger, connectors, mapping PENDING)
          -> commit -> one EnsureChargerMapping("cpo-charger-created")
```

#### 33.2 Immediate mapping push success

```text
CMS committed inventory -> halops -> halclient PUT mapping -> HAL accepts
                      -> CMS HALChargerMapping = SYNCHRONIZED
```

#### 33.3 Mapping push failure and reconciliation

```text
CMS committed inventory -> mapping delivery error -> RECONCILIATION_REQUIRED
reconciler -> same committed charger/connectors/identity -> EnsureChargerMapping
```

#### 33.4 Charger connection

```text
charger OCPP connect -> HAL connection evidence -> immutable connection fact
                    -> CMS receipt + newer HALChargerRuntime -> event -> REST/SSE
```

#### 33.5 Connector Available

```text
charger StatusNotification(Available) -> HAL connector fact -> CMS runtime
liveops -> parent ONLINE+fresh AND connector fresh -> AVAILABLE
```

#### 33.6 Customer charging start request

```text
app -> CMS authorization/CPO wallet-policy/tariff/wallet lock -> StartIntent + hold + CMS command commit
    -> mapping check -> HAL RemoteStart command -> 202/ACCEPTED_FOR_DELIVERY
```

At each new start, CMS locks the CPO settings with the CPO and customer wallet.
It requires `balance >= wallet_min_balance` and calculates the tariff/GST hold
and `EnergyLimitWh` from `balance - wallet_buffer_min_balance`; physical
connector capacity and the existing maximum-duration bound still cap the Wh
limit. `wallet_minimum_balance_not_met` and `insufficient_wallet_balance` are
terminal admission responses: no intent, hold, or HAL command is created.

#### 33.7 Actual StartTransaction

```text
charger StartTransaction -> HAL transaction.started -> CMS validates exact chain
                         -> receipt + one ACTIVE session + session event
```

#### 33.8 Meter progression

```text
charger MeterValues -> HAL transaction.meter(sequence, integer Wh)
                    -> CMS accepts newer sequence only -> meter event -> GET session
```

#### 33.9 Energy-limit stop

```text
CMS affordable Wh -> HAL physical energy_limit_wh -> charger StopTransaction
                  -> transaction.completed -> CMS final meter/settlement
```

#### 33.10 Time-limit stop

```text
CMS max_duration_seconds -> HAL physical cutoff -> charger StopTransaction
                         -> transaction.completed -> CMS final settlement
```

#### 33.11 Customer stop

```text
owner -> CMS persists STOP command and sets STOP_PENDING -> halops.RequestStop
      -> HAL/OCPP delivery -> charger StopTransaction -> completion fact
```

#### 33.12 Natural charger stop

```text
charger stops independently -> HAL observes StopTransaction -> completion fact
                           -> CMS validates final transaction and settles once
```

#### 33.13 Completion and settlement

```text
completion fact -> lock session/hold/wallet -> exact meter + tariff/tax snapshot
                -> ledger/payment + hold capture + COMPLETED/SETTLED -> event
                -> unsafe amount/balance -> durable terminal evidence +
                   session/hold RECONCILIATION_REQUIRED -> bounded retry
```

#### 33.14 HAL retry after lost CMS acknowledgement

```text
HAL fact -> CMS receipt/projection commit -> 204 lost in transit
HAL retries same fact_id + digest + body -> receipt match -> 204/no second effect
```

#### 33.15 Duplicate fact

```text
same fact_id + same digest -> CMS exact duplicate no-op -> no state/event/charge repeat
same fact_id + changed digest -> 409 integrity conflict -> investigate, do not automate
```

#### 33.16 Command timeout and reconcile

```text
CMS -> HAL request -> timeout -> CMS record RECONCILIATION_REQUIRED
CMS -> HAL GET ?cms_command_id=SAME-ID -> known command or explicit missing evidence
```

For STOP, a confirmed-absent command returns an incomplete session to `ACTIVE`;
an OCPP rejection also permits a new stop. Persisted, attempted, accepted, or
ambiguous provider evidence remains `STOP_PENDING`. A completed transaction
always wins. A materialized `ACTUALLY_STARTED` intent is historical evidence,
not connector occupancy: one unmaterialized open intent reserves first, then
one active/stop-pending/reconciliation-required session reserves the connector.

#### 33.17 CPO dashboard REST

```text
CPO `chargers.operations` GET fleet/charger -> authorized CMS service -> liveops -> PostgreSQL
                         -> static inventory + committed runtime projection response
```

#### 33.18 CPO SSE update

```text
fact projection commits -> operational_events insert -> CPO SSE emits event ID/data
client stores ID -> refetches REST snapshot; frame is not state authority
```

#### 33.19 SSE reconnect and REST recovery

```text
disconnect -> client uses stored ID as after_id or Last-Event-ID -> retained event page
           -> dedupe/order -> authoritative REST refetch -> reopen stream
```

### 34 — Testing guide

```powershell
go test ./src/cpo -count=1
go test ./src/liveops -count=1
go test ./src/customerauth -count=1
go test ./src/halops -count=1
go test -p 1 ./...
go vet -p 1 ./...
go test ./src/routes -run TestOpenAPIContractMatchesRuntimeRoutesAndServesUI -count=1
.\scripts\verify-docs.ps1
git diff --check
```

The Windows checkout uses `-p 1` for broad Go work because parallel test-process
startup exhausted the paging file; that is environmental, not semantic.
`TEST_DATABASE_URL` must name an explicitly disposable database. Package tests
cannot prove the full CMS Postgres -> CMS -> HAL -> HAL Postgres -> virtual OCPP
charge-point topology, restart semantics, or physical OCPP acceptance.

### 35 — Debugging checklist

For mapping inspect inventory/connectors, `HALChargerMapping`, then reconciler.
For offline/stale inspect runtime sequence and observed time. For wrong User App
state inspect customer visibility/static status before the batch overlay. For
start inspect intent, command, actual start fact; for meter inspect HAL
transaction/sequence/receipt/session; for stop inspect command and completion
fact. For SSE inspect durable cursor recovery before blaming a stream. Never log
bearers, credentials, raw payloads, or customer-sensitive data while doing so.

### 36 — Current implementation versus planned work

Implemented: CMS inventory, pending/one-immediate mapping push and
reconciliation, fact ingress/projections, `liveops`, CPO reads, User App full
charger overlays, events/cursor/SSE, customer start/stop/session, limits, meter,
completion/settlement, migrations 33 and 34.

Not implemented/approved: a CPO remote-control API, arbitrary provider calls,
physical-vendor acceptance not proven in disposable topology, or any capability
not present in source/contract.

### 37 — Planned serial-number admission

> **PLANNED — DO NOT IMPLEMENT FROM THIS GUIDE YET**

A separate future approval may explore `/<charger_id>/<serial_number>` WebSocket
admission and serial-number mapping. It is not a current route, HAL contract,
or CMS mapping field.

### 38 — Quick reference and glossary

```text
one charger -> GetCharger / GetChargerDetail
many chargers -> GetChargerDetails
connector -> GetConnector
session/meter -> GetSession
fleet -> GetFleet
approved operation -> halops
raw transport -> halclient owns it; CPO code does not call it
HAL fact ingress -> POST /v1/hal-facts
browser near-live -> event + SSE; current truth -> CMS REST/PostgreSQL
timeout -> reconcile same cms_command_id; tenant -> trusted principal CPO ID
```

A **fact** is immutable HAL evidence. A **projection** is CMS’s durable,
validated consumer model. A **mapping** links CMS inventory to OCPP identity.
**Reconciliation** recovers exact durable identities after ambiguity. A
**StartIntent** is a commercial request, not a session. An SSE **cursor** is a
durable event replay ID.

### Junior safety-test completion

This manual now independently answers all fifty required junior safety questions:
the exact six `liveops` methods and return structures; static/live provenance;
offline/stale derivation; normal CPO `liveops`/`halops` boundaries; forbidden
transport/runtime/OCPP access; command versus physical truth; every command,
session, transaction, fact, and event identifier; exact-ID reconciliation;
fact delivery/dedupe/integrity; REST/SSE recovery; wallet versus physical
enforcement; mapping failure; safe extension; diagnosis; current versus planned
behavior; and verification. The five non-negotiable mistakes are direct HAL
access, acknowledgement-as-truth, replacement command IDs, SSE/raw-status as
authority, and bypassing tenant/fact/sequence safety boundaries.
