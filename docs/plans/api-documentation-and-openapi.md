# Complete API Documentation and OpenAPI Explorer

Status: Verified

## Objective

Give every consuming developer one complete human handoff, one validated
machine-readable contract, and one same-origin interactive page for exercising
all implemented APIs.

## Affected Surfaces

- Gin route registration
- OpenAPI schemas, operations, examples, security, and errors
- Embedded Swagger UI
- Runtime/OpenAPI drift tests
- Authentication and CPO reference docs
- Educational workflow and troubleshooting guides
- SMTP, Razorpay, HAL, mail-outbox, and configuration contracts
- Repository instructions, project state, development plan, and changelog

## Implementation Slices

1. Extract every live request, response, validation, role, error, state change,
   and failure mode from handlers/services.
2. Write and validate OpenAPI 3.1 for all implemented business/health routes.
3. Embed and serve OpenAPI plus Swagger UI without a CDN.
4. Add bidirectional runtime-route drift verification and endpoint smoke tests.
5. Expand the human endpoint contract and all educational/integration/internal
   documents to operational handoff quality.
6. Reconcile repository memory and run focused/full verification.
7. Register the UI and raw schema only when `API_DOCS_ENABLED=true`, with
   ordinary application routes unchanged when disabled.

## Acceptance Criteria

- Every implemented business/health route appears exactly once in OpenAPI with
  its actual method.
- Every body, path/header parameter, response schema, security requirement,
  validation rule, and meaningful error code is documented.
- `/docs/` loads embedded Swagger UI and can issue same-origin requests.
- `/openapi.yaml` serves the exact reviewed source file.
- `API_DOCS_ENABLED=false` removes `/docs`, `/docs/`, and `/openapi.yaml`
  without removing ordinary application routes.
- A route/spec mismatch fails tests.
- Docs distinguish implemented behavior from future domain tables and
  integrations.
- No secret value appears in examples or embedded assets.

## Verification

- OpenAPI parse and semantic validation
- Runtime/OpenAPI bidirectional operation comparison
- Swagger UI and raw-contract HTTP smoke tests
- API-docs enabled and disabled route-registration tests
- `.\scripts\verify-docs.ps1`
- `go test ./...`
- `go vet ./...`
- `git diff --check`

## Known Limitation

No client SDK is generated yet. Schema and human-contract review still move
with handler changes even though route parity is automated.

## Verification Result

- OpenAPI parsed and passed semantic validation.
- All 24 implemented business/health method-path pairs matched Gin and OpenAPI
  bidirectionally.
- `/openapi.yaml`, `/docs`, and `/docs/` HTTP smoke tests passed.
- Documentation verification, `go test ./...`, `go vet ./...`, and
  `git diff --check` passed.
