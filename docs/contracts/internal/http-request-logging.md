# HTTP Request Logging Contract

## Purpose

Every request that reaches the Gin router produces one structured completion
record for operational diagnosis. The record identifies the operation, result,
latency, response size, safe authenticated scope, and stable handled API error
code without copying request or response content into logs.

The application writes these records as newline-delimited JSON to standard
output. The deployment supervisor owns capture, retention, access control, and
rotation; the application does not add a second log database or background
shipping process.

`LOG_LEVEL=INFO` is the default concise mode. `LOG_LEVEL=DEBUG` keeps every
normal completion/panic record and additionally emits safe request-start and
handled-error diagnostics for developers. Changing the value requires a
process restart.

## Completion Record

Example using non-production identifiers:

```json
{"timestamp":"2026-08-03T06:30:00.0015Z","level":"WARN","event":"http_request_completed","request_id":"3c99d6f3-b8ad-4dc2-a07c-374696365bb6","method":"POST","route":"/api/v1/platform/cpos","status":409,"duration_ms":1.5,"response_bytes":105,"peer_ip":"127.0.0.1","client_ip":"198.51.100.8","error_code":"cpo_gstin_conflict","auth_scope":"PLATFORM","user_id":"658f4da5-71b9-4795-b6cc-735a46ed1c04"}
```

| Field | Presence and meaning |
| --- | --- |
| `timestamp` | Always; UTC RFC3339Nano completion time |
| `level` | Always; `INFO` below 400, `WARN` for 400–499, `ERROR` for 500+ |
| `event` | Always `http_request_completed` |
| `request_id` | Always; server-generated UUID also returned as `X-Request-ID` |
| `method` | Always; HTTP method |
| `route` | Always; matched Gin route template, or `<unmatched>` for no route |
| `status` | Always; final HTTP response status |
| `duration_ms` | Always; elapsed handling time in fractional milliseconds |
| `response_bytes` | Always; bytes written by the Gin response writer |
| `peer_ip` | When available; direct TCP peer without a port |
| `client_ip` | When available; effective client address according to the proxy rule below |
| `error_count` | Only when Gin has recorded one or more internal context errors |
| `error_code` | Only for handled API errors; the stable response code, never its message |
| `auth_scope` | Only after successful authentication: `PLATFORM`, `CPO`, or `CUSTOMER` |
| `user_id` | Only after successful authentication; opaque global user UUID |
| `cpo_id` | Only for authenticated CPO/customer scope; trusted session tenant UUID |
| `customer_id` | Only for authenticated customer scope; trusted customer UUID |
| `role` | Only for authenticated administrative CPO scope |

The logger is installed before panic recovery. A panic that reaches Gin recovery
therefore still produces a `500` completion record, but the panic value and
stack are not copied into the JSON request record. Long-lived SSE requests are
logged when the stream disconnects, when final duration and byte count are
known.

### Panic diagnostic record

Recovery also emits one `http_panic_recovered` JSON diagnostic before the
completion record. It contains `timestamp`, `level=ERROR`, `event`,
`request_id`, method, matched route template, the Go panic value's type, and a
runtime stack. The stack provides code locations for developers, while the
panic value, request dump, raw URL, query, bodies, and headers are never
recorded. Gin's stock recovery output is disabled so debug mode cannot dump
request headers. Broken-connection panics handled internally by Gin do not emit
this diagnostic because their connection is already unusable.

### DEBUG diagnostic records

With `LOG_LEVEL=DEBUG`, the request logger adds:

- `http_request_started` at request entry, with `timestamp`, `level=DEBUG`,
  `event`, `request_id`, method, matched route template, direct `peer_ip`, and
  trusted effective `client_ip`. The route is `<unmatched>` for a 404 and is
  never the raw path or query.
- `http_error_handled` when a standard API error writer handles a failure, with
  `timestamp`, `level=DEBUG`, `event`, `request_id`, method, matched route
  template, a stable owning component, HTTP status, stable `error_code`, the
  outer Go `error_type`, a safe `error_class`, and PostgreSQL `sql_state` when
  available. Current error
  classes are `application`, `postgresql`, `network`, `timeout`, and `canceled`.
  The error string and wrapped values are not recorded.

Current stable component values are `auth`, `customer_auth`, `cpo`,
`integrations`, and `platform_operations`. DEBUG records are diagnostic process
logs, not durable audit state. INFO mode omits both DEBUG event types;
completion records and panic diagnostics remain active.

## Request Correlation

The backend generates a new UUID for every request and returns it in:

```http
X-Request-ID: 3c99d6f3-b8ad-4dc2-a07c-374696365bb6
```

Client-supplied request IDs are not adopted. Browser clients may read this
header because permissive CORS explicitly exposes `X-Request-ID`. Frontends may
show or retain it with a failed operation as a support reference, but must not
attach request bodies, credentials, or token material to that reference.

## Client Address and Proxy Trust

`peer_ip` always represents the direct connection peer. `client_ip` equals that
peer for direct connections. When—and only when—the peer is loopback, the
logger accepts the first valid IP in `X-Forwarded-For`. This supports the
documented loopback Caddy deployment without trusting forwarding headers from
direct network clients.

Trust `X-Forwarded-For` only from a loopback peer.

No other forwarded header affects request-log identity or tenant context.

## Mandatory Data Exclusions

The request logger never records:

- request or response bodies;
- raw URL paths or path-parameter values;
- query strings or query values;
- request or response header values other than the derived client IP rule;
- authorization headers, access tokens, refresh tokens, cookies, OTPs, or
  passwords;
- email addresses, names, phone numbers, user agents, CPO app IDs, provider
  credentials, mail payloads, or database errors;
- API error messages or panic values.

The matched route template is deliberately logged instead of the raw path so
resource identifiers are not copied from the URL. New logging fields require a
security review and an update to this contract before use.

## Failure and Operational Behavior

- Log encoding or output failure never changes the HTTP response.
- Completion and panic logging are always enabled for the application listener.
  `LOG_LEVEL` controls only the additional safe DEBUG diagnostics.
- The application does not promise durable log retention; systemd/journald or
  the selected process supervisor owns it.
- Requests rejected by Go's HTTP server before they reach Gin cannot produce a
  Gin request record.
- No distributed trace propagation or downstream-service correlation is
  implemented yet.

On the development VPS:

```bash
journalctl -u evcmsnew-dev.service -n 120 --no-pager
journalctl -u evcmsnew-dev.service --since '10 minutes ago' --no-pager
```

Treat logs as operationally sensitive because they contain IP addresses and
opaque identity/tenant identifiers even though secret and content fields are
excluded.

## Developer Logging Rules

The global middleware owns the request completion record. Endpoint code must
not emit a second access/completion line. Standard authentication and API error
writers already attach trusted actor fields, stable `error_code` values, and
DEBUG handled-error classification.

When an endpoint genuinely needs an additional operational event, include:

- a stable, searchable `event` name;
- the current server-generated request ID, obtained in a Gin handler with
  `middleware.RequestID(ctx)`;
- the outcome or committed state-transition name;
- only the minimum opaque entity/tenant identifiers needed for diagnosis;
- bounded counts, durations, retry attempt, or downstream status when useful;
- whether a failure is retryable, expressed as a stable class rather than raw
  provider/database content.

Do not make domain/service packages depend on Gin merely for logging. If a
request ID is materially useful below the handler boundary, pass the string
explicitly as diagnostic context; it must never become business state,
idempotency identity, authorization input, or an audit-log substitute.

Developers should log these kinds of facts:

- a committed state transition that needs operational correlation beyond the
  durable business audit/event;
- a downstream integration call's provider, operation, duration, safe status,
  retry attempt, and final outcome;
- a worker claim/retry/completion with bounded counts and opaque job ID;
- reconciliation totals, lag, or a safe reason class;
- startup/shutdown and dependency-health transitions.
- unhandled panics are already covered by `http_panic_recovered`; do not add a
  second panic logger or log the recovered value.

Developers should not log:

- routine successful reads already represented by the completion record;
- a duplicate line for every handler entry and exit;
- SQL text/parameters, serialized models, mail/provider payloads, or external
  response bodies;
- raw Go errors when they may embed URLs, database values, provider content,
  addresses, credentials, or personal data;
- durable business/audit facts only in process logs. Required audit evidence
  remains transactional PostgreSQL state.

Use `INFO` for normal completed facts, `WARN` for handled/retryable degradation,
and `ERROR` for failed outcomes requiring intervention. New application logs
should remain one JSON object per line on stdout so local tools and journald can
filter them consistently. A new field is not safe merely because it is easy to
obtain; update this contract and its tests before expanding the schema.

For local development, run the API in one PowerShell terminal and issue a
request from another. The first terminal shows the JSON completion line:

```powershell
$env:LOG_LEVEL = 'DEBUG'
go run .
```

Start with `request_id`, then read `http_request_started`, any
`http_error_handled`/`http_panic_recovered` diagnostic, and the final
`http_request_completed` record. `route`, `status`, `error_code`, `component`,
`error_type`, `error_class`, and `sql_state` narrow the failing boundary without
exposing the failing data.

## Verification

```powershell
go test ./src/config ./src/middleware ./src/routes -run 'TestConfig|TestLoadHostinger|TestRequestLog|TestRequestLogger|TestDebugLogging|TestPermissiveCORS' -count=1
.\scripts\verify-docs.ps1
go test ./...
go vet ./...
git diff --check
```
