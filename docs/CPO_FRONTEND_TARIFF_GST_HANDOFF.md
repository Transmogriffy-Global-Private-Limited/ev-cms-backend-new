# CPO Frontend Handoff: Tariffs and Hub GST

This document is the CPO-admin frontend contract for commercial tariff and GST
management. It complements the canonical schema at
`docs/contracts/openapi/openapi.yaml` and the complete HTTP reference at
`docs/contracts/api/administrative-http-api.md`.

## 1. Access and scope

All routes below are under `/api/v1/cpo` and require:

- a valid CPO-admin bearer token; and
- the current tenant selection header `X-CPO-App-ID`.

The server derives the CPO from trusted authentication and selected app scope.
The frontend must never send a `cpo_id`, tariff target ID, or GST tenant ID in
a body in an attempt to choose scope. Only the callable `ADMIN` role may use
these routes. Cross-CPO and unknown records deliberately look like `404`.

Use JSON decimal strings, not JavaScript floating-point arithmetic, for all
money and tax values. Render values from the API as exact decimal strings.

## 2. Commercial model the UI must represent

Tariff and GST are independent:

- A tariff sets the commercial price and targets exactly one Hub, Charger, or
  UserGroup.
- A GST profile belongs to a Hub assignment, because the Hub provides the
  location/state context.
- Customer charging chooses tariffs in this fixed order:
  `UserGroup > Charger > Hub`.
- Regardless of which tariff wins, GST is resolved independently from the
  selected charger's Hub.

Do not show a GST selector in tariff create/edit UI and do not expect a tariff
response to include `gst_id`.

### Valid tariff combinations

| Tariff type | Price type | `units` | Meaning of `price_per_unit` |
| --- | --- | --- | --- |
| `fixed` | `energy` | `kwh` | exact INR (or selected currency) per kWh |
| `fixed` | `time` | `minutes` | exact amount per actual completed minute |
| `fixed` | `sessions` | omitted | one fixed amount for one completed session |

Energy is not priced per meter Wh. The backend derives kWh as `meter_wh / 1000`
with exact decimals. For example, `16.91`, `energy`, `kwh` means INR 16.91 per
kWh; a completed 7200 Wh session has a base amount of INR 121.752 before GST
and final monetary rounding.

Time tariffs use durable HAL start and completion timestamps. This is distinct
from the existing customer-selected time-bounded-session cutoff; the tariff UI
does not configure that cutoff.

`watt/hour` is retired and must not be offered or submitted. It can appear only
in historical session pricing data, where it is displayed as historical legacy
data rather than a current tariff choice.

### Idle fee

The CMS has no authoritative idle-start/idle-end lifecycle. Therefore:

- new or active tariffs must send `idle_fee_per_min: "0"` (or omit it, using
  the zero default);
- a non-zero fee on an active tariff returns `400 idle_fee_unsupported`;
- the value is retained in tariff/snapshot records for compatibility and audit,
  but is not a billable UI control;
- do not derive or display idle billing from total session duration.

## 3. Tariff APIs

Choose exactly one target route family. The URL is the immutable target; the
request body must not contain `hub_id`, `charger_id`, `user_group_id`, or
`assigned_to`.

| Target | Create/list | Read/update/delete |
| --- | --- | --- |
| Hub | `POST`, `GET /hubs/{hub_id}/tariffs` | `GET`, `PATCH`, `DELETE /hubs/{hub_id}/tariffs/{tariff_id}` |
| Charger | `POST`, `GET /chargers/{charger_id}/tariffs` | `GET`, `PATCH`, `DELETE /chargers/{charger_id}/tariffs/{tariff_id}` |
| UserGroup | `POST`, `GET /user-groups/{user_group_id}/tariffs` | `GET`, `PATCH`, `DELETE /user-groups/{user_group_id}/tariffs/{tariff_id}` |

### Create body

```json
{
  "price_per_unit": "16.9100",
  "idle_fee_per_min": "0.0000",
  "currency": "INR",
  "is_active": true,
  "tariff_type": "fixed",
  "price_type": "energy",
  "units": "kwh",
  "start_date": "2026-09-01T00:00:00Z",
  "end_date": "2026-10-01T00:00:00Z"
}
```

`price_per_unit`, `tariff_type`, and `price_type` are required. `currency`
defaults to `INR`; `is_active` defaults to true. A schedule is a root (omit
both timestamps), open-ended fallback (supply `start_date`, omit `end_date`),
or bounded override (`start_date < end_date`, effective on `[start_date,
end_date)`). An end-only schedule is invalid. Timestamps describe commercial
tariff applicability, not the separate customer-selected session cutoff. The
server normalizes currency to uppercase.

For `sessions`, omit `units`. For `energy`, use only `kwh`; for `time`, use
only `minutes`. Do not send `null` as a substitute for an omitted property
unless the frontend's serializer is known to omit it.

### Update body, explicit clearing, and resulting-state validation

`PATCH` accepts any non-empty subset of the create fields and cannot move the
tariff to another target. Omitted properties retain their stored values.
Unlike create, PATCH gives `units`, `start_date`, and `end_date` a deliberate
`null` meaning:

- `units: null` clears units; use it when changing to `price_type: "sessions"`.
- `start_date: null` and `end_date: null` together clear a previously scheduled
  tariff, making it the unbounded root.
- A supplied `start_date` with omitted/null `end_date` is an open-ended
  fallback. A supplied end always requires a start and must be later.

The server applies the patch to the full stored row and validates the result,
so a basis change must submit its complete final trio in one request. Examples:

```json
{"tariff_type":"fixed","price_type":"sessions","units":null}
```

```json
{"tariff_type":"fixed","price_type":"time","units":"minutes"}
```

```json
{"start_date":null,"end_date":null}
```

The UI should omit `units` for a normal price-only PATCH. It must not present
`watt/hour` as an editable option: that value is accepted only when displaying
historical frozen session data, never in a current tariff request.

### Temporal hierarchy and publish floor

The server validates the complete enabled hierarchy after every create, patch,
activation/deactivation, date change, and delete. For one exact target it
allows one root, many open fallbacks with distinct starts, and nested bounded
overrides only. Crossing or identical bounded overrides, duplicate active root,
or equal open starts return `409 tariff_temporal_conflict`. Inactive rows still
need a structurally valid date shape, but may temporarily conflict.

Do not calculate customer price from this list. The backend resolves scope first
(`UserGroup > Charger > Hub`), then time within that winning exact target:
deepest matching bounded interval, latest started open fallback, then root.
`is_active` is an admin switch and never changes automatically when a boundary
passes. A UserGroup root outranks a Charger override.

Before setting a Hub customer-visible, create exactly one active unbounded Hub
root. The server rejects publishing, deleting, re-purposing, or deactivating
that required root with `409 hub_tariff_root_required`. Charger and UserGroup
roots are optional. `DELETE` returns `204`; `409 tariff_in_use` means a frozen
charging/history reference prevents removal, so retain or deactivate the row.

### Tariff response

Successful create/read/update returns a `Tariff` object (`201` for create,
`200` otherwise). Important fields are:

```ts
type Tariff = {
  id: string;
  cpo_id: string;
  assigned_to: "hub" | "charger" | "usergroup";
  hub_id?: string;
  charger_id?: string;
  user_group_id?: string;
  price_per_unit: string;
  idle_fee_per_min: string;
  currency: string;
  is_active: boolean;
  start_date?: string;
  end_date?: string;
  tariff_type?: "fixed";
  price_type?: "energy" | "time" | "sessions";
  units?: "kwh" | "minutes";
  created_at: string;
  updated_at: string;
};
```

Exactly one target ID is present and agrees with `assigned_to`. Preserve that
server response rather than constructing target data client-side.

List calls accept cursor pagination query parameters `limit`, `before`, and
`before_id`. Send `before` and `before_id` together from the prior response's
`next_before` and `next_before_id`. The response is:

```ts
type TariffListResponse = {
  tariffs: Tariff[];
  next_before?: string;
  next_before_id?: string;
  has_more: boolean;
};
```

### Tariff error handling

Show field-level validation errors without retrying unchanged input. Important
codes include `invalid_price_per_unit`, `invalid_idle_fee_per_min`,
`invalid_currency`, `invalid_tariff_type`, `invalid_price_type`, `invalid_units`,
`unsupported_tariff_pricing`, `idle_fee_unsupported`, `invalid_schedule`, and
`invalid_date_range`. A hierarchy conflict returns
`409 tariff_temporal_conflict`; a customer-visible Hub missing its root returns
`409 hub_tariff_root_required`; and history-protected deletion returns
`409 tariff_in_use`. Incorrect/missing target or tariff IDs return
the relevant `404` such as `hub_not_found`, `charger_not_found`,
`user_group_not_found`, or `tariff_not_found`.

## 4. GST profile APIs

| Intent | Method and URL |
| --- | --- |
| Create profile | `POST /gsts` |
| List profiles | `GET /gsts?limit=&before=&before_id=` |
| Read profile | `GET /gsts/{gst_id}` |
| Patch profile | `PATCH /gsts/{gst_id}` |

Create requires `name`, `state`, `sgst_rate`, `cgst_rate`, and `igst_rate`;
`is_active` defaults true. Each component is an exact decimal from 0 through
100. Do not mix non-zero SGST/CGST with non-zero IGST in one profile.

Same-state configuration example:

```json
{"name":"West Bengal standard","state":"West Bengal","sgst_rate":"9","cgst_rate":"9","igst_rate":"0","is_active":true}
```

Interstate configuration example:

```json
{"name":"Maharashtra interstate","state":"Maharashtra","sgst_rate":"0","cgst_rate":"0","igst_rate":"18","is_active":true}
```

`PATCH /gsts/{gst_id}` accepts a non-empty subset. It validates the complete
post-patch profile. If it is currently assigned to a Hub, it also validates the
resulting Hub/GST relationship before persisting. Consequently, changing state,
components, or `is_active:false` can return `400 invalid_gst_for_hub`; guide
the operator to unassign or replace the Hub GST first when that is intentional.

The GST response shape is:

```ts
type GST = {
  id: string; cpo_id: string; name: string; state: string;
  sgst_rate?: string; cgst_rate?: string; igst_rate?: string;
  is_active: boolean; created_at: string; updated_at: string;
};
```

Legacy rows may have omitted rate fields. Treat them as non-assignable and do
not offer them for new Hub assignment until corrected. GST list pagination is
the same cursor shape as tariff lists.

## 5. Hub GST assignment APIs

| Intent | Method and URL | Body |
| --- | --- | --- |
| Assign an unassigned Hub | `POST /hubs/{hub_id}/gst` | `{"gst_id":"<uuid>"}` |
| Read current assignment | `GET /hubs/{hub_id}/gst` | none |
| Replace assignment | `PATCH /hubs/{hub_id}/gst` | `{"gst_id":"<uuid>"}` |
| Unassign | `DELETE /hubs/{hub_id}/gst` | none |

The candidate GST must be same-CPO, active, complete, and assigned to no other
Hub. The server locks and validates the complete relationship:

- same Hub/GST state: present SGST and CGST, zero IGST;
- different state: present IGST, zero SGST and CGST.

Rates represented as zero are valid components where that is the applicable
representation. The frontend should filter obvious incompatible selections for
clarity, but it must always surface the server result as authoritative.

`POST` and `PATCH` return the updated Hub (`200`); `GET` returns the assigned
GST (`200`); `DELETE` returns `204`. A GST already assigned to another Hub
returns `409 gst_already_assigned`. Missing assignment on `GET` returns
`404 gst_not_found`. Invalid relationships return `400 invalid_gst_for_hub`.

## 6. Hub state editing interaction

`PATCH /hubs/{hub_id}` edits the Hub `state` among other Hub fields. If the
Hub has GST assigned, changing the state validates the complete existing GST
profile under the same commercial lock. It can fail with
`400 invalid_gst_for_hub`; the UI should replace/unassign GST or choose a
compatible state, then retry deliberately. Do not claim success from a local
optimistic state update before the response returns.

## 7. UI workflow and display guidance

1. Create/choose the target tariff through its scoped page; never maintain a
   global unrestricted tariff editor.
2. Drive the tariff basis selector from the three valid combinations above.
   When `sessions` is chosen, remove `units` from the submitted body.
3. Display energy as `currency + price_per_unit + "/kWh"`, time as
   `... + "/minute"`, and sessions as `... + "/session"`.
4. Display the precedence explanation wherever multiple target levels exist;
   an active UserGroup tariff supersedes a Charger tariff, which supersedes a
   Hub tariff. This is selection behavior, not a client merge operation.
5. Manage GST from the Hub page. A tariff's target does not decide its GST.
6. Treat active/historical charging pricing as immutable. Tariff or GST edits
   affect future admission only; they do not alter a charging-start intent,
   session, hold, or completed settlement that already has its snapshots.
7. Do not add idle-fee controls, frontend duration billing, per-Wh conversion,
   or client-side GST aggregation. The backend owns authoritative admission,
   GST, snapshots, and settlement.

## 8. CPO wallet admission policy

`GET`, `POST`, and `PUT /api/v1/cpo/settings` now include these integer
whole-currency values in the ordinary settings response and accept them as
optional `multipart/form-data` fields:

```ts
type CPOSettings = {
  invoice_logo?: string;
  invoice_note?: string;
  wallet_min_balance: number;
  wallet_buffer_min_balance: number;
};
```

Both default to `0`, must be non-negative whole values, and are created blank
automatically whenever a CPO is provisioned. Existing CPOs receive the same
default row through migration 43. The UI should let the CPO update either
value without resubmitting invoice fields.

At every new session start the backend locks the wallet and current CPO
settings, then applies this policy before it calculates the tariff hold and
HAL energy limit:

1. Reject the start when `wallet_balance < wallet_min_balance` with
   `409 wallet_minimum_balance_not_met`.
2. Compute `usable_balance = wallet_balance - wallet_buffer_min_balance`.
3. Use only `usable_balance` for tariff/GST affordability and the resulting
   maximum Wh sent to HAL; physical connector limits still apply.

For example, minimum `500` and buffer `20` lets a wallet at exactly `500`
start, but its tariff hold and maximum kWh are calculated from `480`. The
buffer is not an additional minimum threshold. It affects only that new
session's hold/limit; settled sessions keep their existing immutable start
facts. A buffer leaving no positive usable balance returns
`409 insufficient_wallet_balance`.

## 9. Customer-facing consequence (for CPO preview screens)

The customer price API exposes `currency`, `price_per_unit`, `tariff_type`,
`price_type`, applicable `units`, and the Hub GST breakdown only when the
price is available. It will report unavailable rather than calculate tax if
the Hub GST is missing, inactive, incomplete, cross-tenant, or state/rate
inconsistent. A CPO preview should therefore identify GST remediation rather
than show a fabricated zero-tax price.

The complete machine-readable contract remains
`docs/contracts/openapi/openapi.yaml`; do not create a separately maintained
frontend schema from this document.
