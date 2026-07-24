# Platform Billing API Contract

## Boundary

This surface records what a CPO owes TransEV for use of the CMS platform. It is
not the CPO's charger/customer payment system. It never reads or uses the CPO's
Razorpay keys and contains no automatic collection or provider webhook.

Every route requires an active `PLATFORM` bearer session. State, audit, durable
events, allocations, and mail work commit together. Exact amounts are signed
64-bit integers in currency minor units; floats are never accepted.

The machine-readable source is `../openapi/openapi.yaml`.

## Billing Account

### `GET /api/v1/platform/cpos/{cpo_id}/billing-account`

Returns the CPO's platform billing identity or `404
billing_account_not_found`. This record can exist with or without a
subscription.

### `PUT /api/v1/platform/cpos/{cpo_id}/billing-account`

Creates or replaces billing metadata:

```json
{
  "legal_name": "Example Charging Private Limited",
  "billing_email": "accounts@example.com",
  "tax_id": "19ABCDE1234F1Z5",
  "currency": "INR",
  "billing_address": {
    "line1": "1 Example Road",
    "city": "Kolkata",
    "postal_code": "700001",
    "country": "IN"
  }
}
```

Legal name and valid email are required. Currency is exactly three uppercase
letters. The address is non-secret JSON. The response never contains payment
credentials.

Side effects: `CPO_BILLING_ACCOUNT_SET` audit and
`platform.invoice.billing_account_updated`.

## Invoices

### `POST /api/v1/platform/cpos/{cpo_id}/invoices`

Creates a `DRAFT` invoice. A billing account must already exist.

```json
{
  "invoice_number": "TE-2026-000042",
  "subscription_id": "8e9a038d-545e-4644-a1ea-b5aa798290b8",
  "period_starts_at": "2026-07-24T12:00:00Z",
  "period_ends_at": "2026-08-24T12:00:00Z",
  "external_reference": "erp-export-2042",
  "idempotency_key": "invoice-contract-0042-period-1",
  "lines": [
    {
      "description": "Growth Monthly platform subscription",
      "quantity": 1,
      "unit_amount_minor": 250000,
      "tax_minor": 45000,
      "metadata": {"plan_version": 2}
    }
  ]
}
```

The server calculates:

```text
line subtotal = quantity * unit_amount_minor
line total = line subtotal + tax_minor
invoice subtotal = sum(line subtotals)
invoice tax = sum(line taxes)
invoice total = subtotal + tax
invoice due = total
```

All operations are overflow-bounded. Invoice number, external reference, and
actor/idempotency key are unique. If supplied, `subscription_id` must belong to
the path CPO. Exact replay returns the original invoice and lines; cross-CPO key
reuse returns `409 idempotency_conflict`.

The billing router permits up to 64 KiB because a request may contain up to 500
lines. Unknown JSON fields remain rejected.

### `GET /api/v1/platform/cpos/{cpo_id}/invoices`

Returns at most 500 invoice headers, newest first.

### `GET /api/v1/platform/invoices/{invoice_id}`

Returns `{"invoice": {...}, "lines": [...]}` with lines ordered by
`line_number`.

### `POST /api/v1/platform/invoices/{invoice_id}/issue`

```json
{
  "due_at": "2026-08-10T18:30:00Z",
  "reason": "Approved monthly platform invoice"
}
```

Only `DRAFT` may become `ISSUED`; due time must be future. Publication freezes
invoice commercial fields and every line through database triggers. When mail
is enabled, encrypted `CPO_PLATFORM_INVOICE_ISSUED` work is queued to the
billing-account email in the same transaction.

Side effects: `PLATFORM_INVOICE_ISSUED` and `platform.invoice.issued`.

### `POST /api/v1/platform/invoices/{invoice_id}/void`

```json
{"reason":"Duplicate invoice issued in error"}
```

Only an issued invoice with `paid_minor=0` can be voided. `VOID` is terminal;
reason and time are retained. Drafts remain editable internal records but have
no public delete endpoint.

## Payments and Allocations

### `POST /api/v1/platform/cpos/{cpo_id}/payments`

Records a provider-neutral payment:

```json
{
  "payment_reference": "TEPAY-2026-000018",
  "currency": "INR",
  "amount_minor": 295000,
  "method": "BANK_TRANSFER",
  "external_reference": "UTR000000000018",
  "occurred_at": "2026-08-03T09:15:00Z",
  "notes": "Received in platform subscription account.",
  "idempotency_key": "bank-utr-000000000018",
  "allocations": [
    {
      "invoice_id": "c8524487-3b43-4b34-86cc-ab2cba79f83b",
      "amount_minor": 295000
    }
  ]
}
```

`occurred_at` cannot be future. Allocations are optional and their sum cannot
exceed payment amount. Each invoice must be open, belong to the same CPO, use
the same currency, and have sufficient due balance. Rows are locked while
allocating. Invoice becomes `PAID` at zero due, otherwise `PARTIALLY_PAID` or
`OVERDUE` according to due time. Unallocated payment balance is retained as
`amount_minor - allocated_minor`.

### `GET /api/v1/platform/cpos/{cpo_id}/payments`

Returns at most 500 payment headers, newest first.

### `GET /api/v1/platform/payments/{payment_id}`

Returns payment plus immutable allocation rows.

### `POST /api/v1/platform/payments/{payment_id}/void`

```json
{"reason":"Bank transfer was reversed"}
```

Atomically changes the payment to `VOID`, retains reason/time and allocation
history, reverses each allocated amount from its invoice, and recalculates
invoice `ISSUED`, `PARTIALLY_PAID`, or `OVERDUE` state. It rejects reversal
when an allocated invoice is already void or its recorded paid amount cannot
safely support the reversal.

## Timeline

### `GET /api/v1/platform/cpos/{cpo_id}/billing-timeline`

Returns optional billing account plus the newest 500 invoice headers and 500
payment headers:

```json
{
  "account": {},
  "invoices": [],
  "payments": []
}
```

This is an operational query, not a financial general ledger or tax filing
system.

## Overdue Worker and Recovery

`billing-maintenance` sends durable heartbeats and repeatedly locks due open
invoices using `FOR UPDATE SKIP LOCKED`. It marks `ISSUED` or
`PARTIALLY_PAID` invoices `OVERDUE` when `due_at <= now` and due remains.
State, audit, and `platform.invoice.overdue` commit together.

Crash before commit leaves the invoice due for the next pass. Crash after
commit leaves durable state and event recovery. Stale registered worker state
degrades readiness through the shared worker contract.

## Errors

In addition to shared auth errors:

- `400 invalid_billing_email`, `invalid_invoice`, `invalid_lines`,
  `invalid_period`, `invalid_issue`, `invalid_payment`, or
  `invalid_allocations`;
- `404 cpo_not_found`, `billing_account_not_found`, `invoice_not_found`,
  `payment_not_found`, or `subscription_not_found`;
- `409 invoice_conflict`, `payment_conflict`, `idempotency_conflict`,
  `allocation_scope_mismatch`, `allocation_exceeds_invoice_due`,
  `invalid_invoice_transition`, or `payment_reversal_conflict`.

No error includes provider credentials or internal SQL.
