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
CPO ADMIN inventory mutation -> CMS transaction + PENDING mapping
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
protocol observations. Freshness is `FRESH`, `STALE`, or `UNKNOWN`, with the
CMS display threshold `HAL_V1_METER_STALE_AFTER` (default 30s), not a meter SLA.

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
| CPO ADMIN | `GET /api/v1/cpo/operations/fleet` | same-CPO `FleetState`: totals, online/offline/unknown, available/charging/faulted connectors, active sessions. |
| CPO ADMIN | `GET /api/v1/cpo/operations/chargers/{charger_id}` | static CPO charger plus `ChargerDetail` connection/connector evidence. |
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
`hal_fact_receipts`, `hal_charger_runtimes`, `hal_connector_runtimes`, intents,
sessions, holds, and `operational_events`. Migration 28 is the core charging
vertical; migration 33 is durable event replay. This manual applies neither.

Use `HAL_V1_BASE_URL`, `HAL_V1_CMS_BEARER_TOKEN`,
`HAL_V1_CMS_FACT_BEARER_TOKEN`, `HAL_V1_REQUEST_TIMEOUT`, and
`HAL_V1_METER_STALE_AFTER` from `contracts/configuration.md`. The provider has
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

`halclient` attaches the CMS bearer, JSON body, `Idempotency-Key`, and optional
`X-Correlation-ID`; its client timeout is `HAL_V1_REQUEST_TIMEOUT`. Mutating
calls use the CMS mapping/command ID as their idempotency key. A timeout means
delivery is unknown, not that it is safe to send another command. It returns a
typed `HTTPError` for non-2xx provider response and `ErrUnavailable` when base
URL/bearer is absent. `GetCommand` is the sole recovery lookup and queries
`GET /v1/remote-commands?cms_command_id=...`.

## Persistence map and consumer comparison

| CMS model/table | Key invariant and lifecycle |
| --- | --- |
| `HALChargerMapping` / `hal_charger_mappings` | one CMS charger and unique OCPP identity; pending/synchronized/reconciliation mapping evidence. |
| `HALChargerRuntime` / `hal_charger_runtimes` | one CMS charger; latest accepted connection generation/sequence/observation. |
| `HALConnectorRuntime` / `hal_connector_runtimes` | one CMS connector; latest accepted OCPP status sequence/observation. |
| `ChargingStartIntent` | credential hash unique; one materialized session ID; commercial decision before HAL command. |
| `WalletHold` | one start intent; held/captured/released/reconciliation accounting lifecycle. |
| `HALCommandRecord` | `cms_command_id` primary and HAL command unique when known; exact recovery identity. |
| `ChargingSession` | start intent and HAL transaction unique; exact OCPP numeric transaction retained. |
| `HALFactReceipt` | fact ID primary plus immutable digest; receipt/projection transaction boundary. |
| `OperationalEvent` / `operational_events` | increasing durable replay ID, CPO scope and optional customer scope, retention expiry. |

| Concern | Platform | CPO ADMIN | User App |
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
-> ReconcileCommand(same cms_command_id) -> HAL durable command or 404 evidence
HAL repeats same fact_id/digest -> CMS 204 and no second projection/event
client reconnects -> GET events after persisted numeric ID -> dedupe -> GET fleet/charger/session
-> reopen SSE; a missed stream never changes durable truth
```

### Future approved CPO stop pattern (documentation only)

```text
CPO handler -> require active same-CPO ADMIN -> load exact session/charger/connector
-> persist CMSCommandID + audit + expiry -> halops.RequestStop
-> accepted/ambiguous response is STOP_PENDING/reconciliation only
-> exact transaction.completed fact finalizes; no router-to-halclient shortcut
```

## Developer cookbook

| Need | Safe implementation |
| --- | --- |
| Add a CPO charger detail widget | authorize CPO ADMIN, call existing `GetOperationalCharger`; do not query HAL or runtime tables in the handler. |
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
`./scripts/verify-docs.ps1`, and broad checks are `go test ./...`, `go vet
./...`, and `git diff --check`. Add database tests only against an explicitly
disposable `TEST_DATABASE_URL`. New capability tests must cover tenant scope,
customer ownership, duplicate/altered fact, stale/out-of-order sequence,
offline parent, timeout/reconciliation, cursor/SSE recoverability, and virtual
charge-point start/meter/stop races. No full dual-service evidence should be
claimed merely because database-free package tests pass.
