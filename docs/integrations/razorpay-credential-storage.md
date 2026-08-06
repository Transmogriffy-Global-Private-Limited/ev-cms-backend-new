# Razorpay Credential Storage Integration

## Current Capability

The CMS can store, rotate, list metadata for, internally resolve, and remove one
Razorpay credential set per CPO. The User App now uses those credentials
internally to create Razorpay wallet-recharge orders and verify checkout
payments. It does not expose secret plaintext through HTTP or add a CPO-side
payment API.

## Authorization

Only an authenticated CPO session with the currently supported `ADMIN` role may
use the HTTP surface. Dormant role enum values grant no access. Every request
also requires the current `X-CPO-App-ID`. Platform superadmins have no
tenant-secret read route.

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

`ResolveRazorpay(ctx, cpoID)` is an internal service method used by the trusted
User App payment orchestration callback. Its caller must independently
establish a trusted CPO context. There is deliberately no generic secret-read
endpoint.

The method refuses records encrypted under an unavailable key ID. Before an
encryption key is removed, every affected row must be deliberately
re-encrypted; changing the environment key without migration makes resolution
fail closed.

## User App recharge flow

The customer calls `POST /api/v1/app/wallet/recharge/orders` with a
positive two-decimal INR amount and an `Idempotency-Key`. The CMS derives the
customer, CPO, and wallet from the validated customer principal, creates a
durable internal recharge order, resolves the encrypted CPO credentials, and
uses the official Razorpay Go SDK to create the provider order. The response
contains the provider order ID, amount in rupees and minor units, currency,
status, and public Razorpay key ID for checkout.

The customer then calls `POST /api/v1/app/wallet/recharge/verify` with the
Razorpay order ID, payment ID, and checkout signature. The CMS verifies the
signature, fetches the payment through the SDK, requires matching order ID,
amount, currency, and captured status, and commits one transaction containing:

```text
verified provider payment
-> completed wallet CREDIT ledger row
-> wallet balance increment
-> PAID internal recharge order
```

The wallet credit is protected by the recharge order link, a wallet lock, and
the existing wallet idempotency constraint. Repeated verification cannot credit
the wallet twice. Authorized-but-not-captured payments are stored but do not
credit the wallet.

The implementation stores the non-secret provider order/payment snapshots,
provider IDs, amounts, currency, status, method, fee/tax fields, provider
timestamps, error fields, checkout signature evidence, and a future refund
record shape. Sensitive provider credentials, card numbers, CVV, and
authorization headers are excluded from stored snapshots. Refund execution and
webhook ingestion remain separate follow-up work; the durable refund table is
already linked to the recharge order/payment so that later refund orchestration
does not need a schema rewrite.

## Explicit Non-Capabilities

Stored credentials do not mean the CMS currently:

- captures payments on the provider or performs refunds yet;
- ingests Razorpay webhooks or reconciles provider settlements yet;
- retries provider calls after an ambiguous order-creation timeout;
- grants platform staff secret access.
