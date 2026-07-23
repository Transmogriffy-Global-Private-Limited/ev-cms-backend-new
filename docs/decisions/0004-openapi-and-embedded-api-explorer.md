# 0004: OpenAPI Contract and Embedded API Explorer

Status: Accepted

Date: 2026-07-23

## Context

Frontend, QA, mobile, and backend developers need a complete executable
contract. Markdown alone cannot prove route parity, and a CDN-hosted API
explorer would fail in restricted or offline development environments.

## Decision

- Maintain OpenAPI 3.1 at
  `docs/contracts/openapi/openapi.yaml`.
- Embed that exact file into the Go binary; do not maintain a second runtime
  copy.
- Serve the contract publicly at `/openapi.yaml`.
- Serve embedded Swagger UI assets at `/docs/`, configured against the
  same-origin specification.
- Keep `docs/contracts/api/administrative-http-api.md` as the complete
  human-readable companion.
- Parse and semantically validate OpenAPI in tests.
- Compare every implemented business/health runtime method-path against the
  OpenAPI operation set in both directions.
- Keep payload descriptions explicit in source-controlled OpenAPI instead of
  generating an incomplete contract from handler comments.

## Consequences

- Developers can exercise APIs without a separate frontend, Node toolchain,
  container, or CDN.
- Route additions fail verification until OpenAPI is updated.
- Schema accuracy still requires disciplined review because route parity alone
  cannot prove every field; handler/request/response tests remain authoritative
  executable behavior.
- The docs endpoints expose API shape but no secrets. They have no operation
  that bypasses authentication.
- The binary grows because Swagger UI assets are embedded.

## Deferred

- Generated TypeScript or other SDKs
- Schema-driven server request validation
- Automated comparison of every Go struct field to component schemas
- Multiple published API versions
