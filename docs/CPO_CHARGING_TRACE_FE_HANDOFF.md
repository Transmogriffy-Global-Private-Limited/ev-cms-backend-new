# CPO charging transaction trace

Use the CMS only; browsers must never call the HAL.

`GET /api/v1/cpo/charging-sessions/{session_id}/trace` and
`GET /api/v1/cpo/charging-traces/{trace_id}` require the normal CPO Bearer
token, `X-CPO-App-ID`, active membership, and `charging_traces.read`.

The response is a diagnostic waterfall, not charging authority. Keep the CMS
session projection as the source for status, money, and meter display.
`trace_id`, `session_id`, `hal_transaction_id`, and
`ocpp_transaction_id` are distinct identities.

Events are newest-first by the pair `(occurred_at, id)`. To fetch the next
page send both `before_occurred_at` and `before_event_id` from the response;
never send either cursor component alone. A page is bounded to 100 events.
`hal_source: UNAVAILABLE` means HAL diagnostic evidence could not be fetched:
show the available CMS events and a partial-data indicator, without treating
it as a charging error. Event data is intentionally sanitized; do not expect
idTags, credentials, authorization headers, customer contact data, or raw
OCPP frames.

## Waterfall rendering contract

Render one chronological waterfall after reversing the newest-first page (or
prepend later pages). Use four fixed lanes and directional arrows:

```text
APP  -> CMS  -> HAL  -> CHARGER
```

Each event provides `source`, `target`, `category`, `protocol`, `phase`,
`summary`, occurrence/recording timestamps, optional before/after state, and
sanitized data. Show protocol acknowledgements and rejected outcomes as their
own events; do not infer success from a later state row.

- Band events by `PRE_START`, `STARTING`, `CHARGING`, `STOPPING`, and
  `POST_STOP`.
- Render state transitions (`state_before` -> `state_after`) as markers in
  the connector's lane.
- Collapse repeated meter samples into a count plus first/last time and meter
  value; let an operator expand them. Never turn the collapsed summary into a
  billing calculation.
- Visually mark persistence/reconciliation failure evidence, but keep it
  distinct from a protocol acceptance/rejection and from commercial state.
- Treat `session_id`, `hal_transaction_id`, and `ocpp_transaction_id` as
  distinct labelled correlation chips. Do not substitute any one for another.

`cms_source` and `hal_source` independently say `AVAILABLE`, `UNAVAILABLE`,
or `NOT_REQUESTED`. A partial source is expected diagnostic degradation: the
normal charging-session API remains the source for status, money, energy and
stop eligibility. A `404 charging_trace_not_found` is not a session error and
should show a neutral “diagnostic evidence unavailable/retained out” state.

The initial user-app start response exposes `trace_id`. A CPO can also begin
from a session ID after the authoritative HAL start fact has materialized the
CMS session.
