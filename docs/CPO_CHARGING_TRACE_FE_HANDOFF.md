# CPO charging transaction trace

The browser uses CMS only; it must never query the OCPP HAL. Trace is
diagnostic evidence, never charging, billing, connector, wallet, or command
authority. Keep the normal CMS charging-session projection as the source for
current status, money, and meter display.

## Access and static snapshot

`GET /api/v1/cpo/charging-sessions/{session_id}/trace` and
`GET /api/v1/cpo/charging-traces/{trace_id}` require a CPO bearer token, the
matching `X-CPO-App-ID`, active membership, and `charging_traces.read`.
`trace_id`, `session_id`, `hal_transaction_id`, and
`ocpp_transaction_id` are separate labelled identities.

The static response is newest-first by `(occurred_at,id)`. Fetch later pages
with both `before_occurred_at` and `before_event_id`; never send either cursor
alone. It also returns `sources_present` and `replay_cursor`.

`sources_present` means only that CMS has persisted evidence attributed to one
or more of `APP`, `CMS`, `HAL`, and `CHARGER`. It is not HAL availability or
health. A `404 charging_trace_not_found` is a neutral “diagnostic evidence
unavailable/retained out” state, not a charging-session error.

## Race-free SSE

After reading the static trace, connect to:

```text
GET /api/v1/cpo/charging-traces/{trace_id}/stream?after={replay_cursor}
```

The stream emits `event: trace_event`. Its SSE `id` and reconnect cursor are
the durable CMS ingestion sequence, not `occurred_at`. Send it back as
`Last-Event-ID` (or `after`) after reconnect. The client dedupes by immutable
event `id`, then inserts into the display by `(occurred_at,id)`. This separates
safe delivery replay from chronological waterfall display.

The connection is reauthorized during heartbeats. If membership, app context,
or `charging_traces.read` is revoked, it closes; refresh the normal CPO session
instead of attempting to bypass the scope.

## Waterfall rendering

Use fixed lanes:

```text
APP | CMS | HAL | CHARGER
```

Render only the backend-declared `source -> target` relation. Do not infer
arrows from timestamps, correlation IDs, adjacent events, charger identity, or
customer identity. `correlation_id` is details metadata only.

Each immutable event includes source, target, category, protocol, phase,
summary, occurrence and CMS-recording time, optional state transition, and
sanitized `data`. Meter events may be visually collapsed, but that display must
not produce billing calculations. No event data contains credentials, idTags,
authorization headers, customer contacts, or raw OCPP frames.
