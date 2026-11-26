# CoW Run 2A Frontend Contract: Saved Service Addresses

**Contract baseline**: Go EV-CMS `anubhab-work` commit `f2be703b1d200eb0173c64ba655124496fb2755c` plus guarded Run 2A source candidate. **Not deployed**. This document is self-contained for implementing only the address screen and reusable service-input selectors; no live quote/job API exists here.

## Authentication and availability

Use the established customer login/session. Send `Authorization: Bearer <customer token>` and the required `X-CPO-App-ID` containing the current already-authenticated tenant's app ID for all address endpoints. Never let the browser select arbitrary CPO/customer IDs. These are **customer-app** endpoints, not CPO staff or Platform administrator endpoints. A customer can use them only while the CPO is ACTIVE and CoW has been enabled by SuperAdmin; disabled/inactive returns `403` / `cow_unavailable`. Existing customer session failures remain `401` / standard auth error. Store no address PII in analytics or application logs.

## Added operations (6)

| Method | Path | Request | Success |
|---|---|---|---|
| GET | `/api/v1/app/addresses?limit=50&before=...&before_id=...` | Cursor and limit optional, default 50, max 100; cursor pair required | 200 `CoWAddressPage` |
| POST | `/api/v1/app/addresses` | `CoWAddressCreate` | 201 `CoWAddress` |
| GET | `/api/v1/app/addresses/{address_id}` | Own active address UUID | 200 `CoWAddress` |
| PATCH | `/api/v1/app/addresses/{address_id}` | `CoWAddressPatch` with positive `expected_version` | 200 `CoWAddress` |
| DELETE | `/api/v1/app/addresses/{address_id}?expected_version=N` | Current positive version query param | 204 empty |
| POST | `/api/v1/app/addresses/{address_id}/make-default` | `{ "expected_version": N }` | 200 `CoWAddress` |

`CoWAddressCreate` input example:

```json
{
  "label": "Home",
  "line1": "42 Test Street",
  "line2": "Apartment 3",
  "locality": "",
  "city": "Kolkata",
  "state": "West Bengal",
  "postal_code": "700001",
  "country_code": "IN",
  "latitude": 22.5726,
  "longitude": 88.3639,
  "landmark": "Near main road",
  "building_instructions": "Gate B",
  "floor": "3",
  "contact_instructions": "Call on arrival",
  "geocode_provider": "",
  "geocode_provider_place_id": "",
  "is_default": true
}
```

All keys except `label`, `line1`, `city`, `state`, `postal_code`, `country_code` may be omitted in create. Text is trimmed; country code is uppercased. Editable string limits (Unicode characters): label 80; line1/line2 255; locality/city/state 120; postal 20; country 2; landmark 255; instructions 500; floor 40; geocode_provider 80; provider_place_id 255. Coordinates must either be both present (lat -90..90, lon -180..180) or both absent. They are *unverified customer inputs*, not navigation truth. Geocode place ID requires a provider name. The first saved address is automatically default regardless of `is_default`.

Success response example (`CoWAddress`, no CPO or customer ID exposed):

```json
{
  "id": "11111111-1111-4111-8111-111111111111",
  "label": "Home", "line1": "42 Test Street", "line2": "Apartment 3",
  "locality": "", "city": "Kolkata", "state": "West Bengal",
  "postal_code": "700001", "country_code": "IN",
  "latitude": 22.5726, "longitude": 88.3639,
  "landmark": "Near main road", "building_instructions": "Gate B", "floor": "3",
  "contact_instructions": "Call on arrival", "geocode_provider": "", "geocode_provider_place_id": "",
  "is_default": true, "version": 1,
  "created_at": "2026-11-26T07:00:00Z", "updated_at": "2026-11-26T07:00:00Z"
}
```

`CoWAddressPage` is `{ "addresses": [CoWAddress...], "has_more": boolean, "next_before"?: RFC3339, "next_before_id"?: UUID }`. For PATCH pass exactly the fields to change and `expected_version`, e.g. `{ "expected_version": 1, "landmark": "Changed" }`. Optional string null clears it; `is_default`, `is_active`, IDs, timestamps, and version are never editable via PATCH. Explicit coordinate null must be paired for both values when clearing. No-op does not increment version. Default selection increments versions of previously default and newly selected rows, so refresh the address list after success. Removing the default promotes the oldest remaining active saved address.

Error envelope is `{ "error": { "code": "...", "message": "..." } }`. Handle codes: `invalid_address`, `invalid_coordinates`, `invalid_address_id`, `invalid_cursor`, `invalid_limit`, `invalid_request` (400); `cow_unavailable`/`forbidden` (403); `address_not_found` (404; includes foreign/removed IDs); `address_version_conflict` (409). After 409, refetch and request explicit user confirmation of the new change, rather than blind retry. Standard 401/500 error policies still apply. On network uncertainty after create, list recent addresses before offering another create: create has no persisted idempotency key.

## Old interfaces: retained, not duplicated

The existing authenticated `/api/v1/app/profile` and `/api/v1/app/vehicles` endpoints are unchanged by Run 2A. The normal CMS vehicle shape (`vehicle_number`, `vehicle_type`, `vehicle_make`, `vehicle_model`) remains the canonical saved vehicle API; do not assume it carries battery capacity or connector compatibility. No existing CPO staff or SuperAdmin HTTP operation changes in this run. The top-level published OpenAPI count rises **263 -> 269**.

## Service-context semantics for future UI

A future CoW service request can select exactly one saved vehicle ID **or** inline temporary vehicle object, and exactly one saved address ID **or** inline temporary address object. Run 2A provides this as an *internal backend snapshot builder only*, **not** an externally callable API. Temporary inputs are not silently stored in saved lists. A future ServiceJob must persist a full copied snapshot when created; later edits/deletion to saved address/vehicle cannot change past job facts. No job creation, quote, promotion, payment, driver action, or HAL endpoint is available through Run 2A.

## FE completion checklist

- Authenticated CPO-context address list/create/edit/remove/default UI; handles empty state and first default.
- Exact six API calls with guarded version, stable field null/omission semantics and cursor resets.
- Prevent duplicate user submissions while in flight; recover from ambiguous create by querying list before allowing retry.
- 409 conflict -> reload without silently discarding user-entered text; deliberate reapply only.
- 403 CoW disabled -> show module-unavailable state; do not mistake old staff role defaults for entitlement.
- No claim of geocoding accuracy from raw customer coordinates and no PII in telemetry.
- Exclude operational job/charging workflows until their future backends are implemented and tested.
