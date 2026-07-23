# Razorpay Credential Storage Integration

## Current Capability

The CMS can store, rotate, list metadata for, internally resolve, and remove one
Razorpay credential set per CPO. It does not yet call Razorpay, execute
payments, verify webhooks, reconcile settlements, or expose secret plaintext
through HTTP.

## Authorization

Only an authenticated CPO session with `OWNER` or `ADMIN` role may use the HTTP
surface. Every request also requires the current `X-CPO-App-ID`. Platform
superadmins have no tenant-secret read route.

Supported routes:

- `GET /api/v1/cpo/integrations`
- `GET /api/v1/cpo/integrations/RAZORPAY`
- `PUT /api/v1/cpo/integrations/RAZORPAY`
- `DELETE /api/v1/cpo/integrations/RAZORPAY`

Request evaluation order is bearer authentication, temporary-password gate,
current app-ID comparison, role check, provider/JSON validation, then the
tenant-scoped operation. The CPO ID used by storage comes from the principal.

## Credential Contract

The write payload contains:

- `key_id`: 8 to 100 characters;
- `key_secret`: 16 to 255 characters;
- optional `webhook_secret`: at most 255 characters.

Unknown JSON fields, multiple JSON values, malformed bodies, and bodies over
32 KiB return `invalid_request`. Unsupported provider names return
`unsupported_integration_provider`. Invalid lengths return
`invalid_integration_credentials`.

The three fields are serialized together and encrypted using AES-256-GCM. The
authenticated associated data is:

```text
ev-cms-cpo-integration:<cpo_uuid>:RAZORPAY
```

Binding ciphertext to CPO and provider prevents it from being moved to another
tenant or provider without decryption failure. Rows record an encryption key
ID; automatic key rotation is not implemented.

Responses contain only provider, masked key-ID hint, active state, and
timestamps. Audit records identify the mutation and provider but do not include
credential fields.

Example metadata response:

```json
{
  "provider": "RAZORPAY",
  "display_hint": "****5678",
  "is_active": true,
  "configured_at": "2026-07-23T12:00:00Z",
  "updated_at": "2026-07-23T12:00:00Z"
}
```

`PUT` is an atomic upsert. Rotation replaces ciphertext/key ID/hint and retains
one CPO/provider row. `DELETE` removes the row and returns 204; deleting an
absent row returns `integration_not_found`. Listing returns only the current
CPO's rows.

## Internal Use Boundary

`ResolveRazorpay(ctx, cpoID)` is an internal service method for future payment
orchestration. Its caller must independently establish a trusted CPO context.
There is deliberately no generic secret-read endpoint.

The method refuses records encrypted under an unavailable key ID. Before an
encryption key is removed, every affected row must be deliberately
re-encrypted; changing the environment key without migration makes resolution
fail closed.

Before payment execution is implemented, define provider idempotency, webhook
authentication, retry policy, transaction ownership, reconciliation, and audit
behavior as one coherent feature.

## Explicit Non-Capabilities

Stored credentials do not mean the CMS currently:

- creates Razorpay orders or captures payments;
- verifies signatures or webhook secrets;
- retries provider calls;
- stores provider event IDs;
- reconciles payments or settlements;
- grants platform staff secret access.
