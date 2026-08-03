# ADR 0011: Safe HTTP Request Observability

Status: Accepted

Date: 2026-08-03

## Context

The service had database, worker, and startup logs but no application HTTP
access log. Operators could see a `409` in a browser without being able to find
the matched operation, stable API error code, authenticated scope, or request
duration in the service journal. Logging payloads to compensate would expose
passwords, OTPs, tokens, personal data, and integration credentials.

## Decision

- Emit one newline-delimited JSON completion record for every request that
  reaches Gin.
- Install logging outside recovery so recovered panics are recorded as `500`.
- Replace Gin's stock recovery output with a correlated JSON panic diagnostic
  containing code stack locations but no request dump or recovered value.
- Generate a server-owned UUID and return it as `X-Request-ID` on every Gin
  response; permissive CORS exposes the header to browser clients.
- Log the matched route template, not the raw URL path or query string.
- Enrich authenticated records only with trusted opaque scope, user, CPO,
  customer, and role identifiers already established by authentication.
- Record stable handled API error codes but never response messages or bodies.
- Trust `X-Forwarded-For` only from a loopback peer, matching the documented
  Caddy-to-loopback deployment; otherwise use the direct peer address.
- Write to standard output and leave storage, access, rotation, and retention
  to the process supervisor.
- Keep request logging always enabled rather than adding configuration for a
  security-safe baseline capability.
- Default `LOG_LEVEL` to `INFO`; allow `DEBUG` to add safe request-start and
  handled-error component/type diagnostics without relaxing exclusions.

The exact schema and exclusions are owned by
`docs/contracts/internal/http-request-logging.md`.

## Consequences

Operators can correlate a frontend failure with one safe structured record and
can distinguish exact handled errors such as `cpo_gstin_conflict`. The frontend
may preserve the response request ID as a support reference. Logs remain
operationally sensitive because IP addresses and opaque identifiers are
present.

Long-lived SSE requests produce their completion record only at disconnect.
Requests rejected by Go's HTTP server before Gin are outside this logger. Log
delivery is best effort and does not alter API success or failure behavior.
Recovered panics produce a separate safe developer diagnostic before their
normal `500` completion line.
DEBUG mode also makes hanging requests visible through a start event and adds
the owning component plus Go error type for handled failures. It still does not
record error strings or request/response content. Safe error classes and a
PostgreSQL SQL state, when present, make internal failures distinguishable
without exposing database values.

## Rejected Alternatives

- Gin's default text logger: rejected because it lacks the required stable
  fields and does not enforce the content-exclusion policy.
- Logging bodies, query values, headers, user agents, or API messages: rejected
  because those surfaces contain credentials, personal data, and arbitrary
  caller content.
- Trusting forwarding headers from every caller: rejected because direct
  clients could spoof the logged address.
- Adding a log table or shipping worker: rejected because the current service
  supervisor already captures stdout and no durable in-application log store is
  required.
