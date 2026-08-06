# User App Frontend Handoff

This is the implementation handoff for the customer-facing User App HTTP
surface: customer authentication, published-network discovery, favorites,
informational pricing, wallet history, and Razorpay wallet recharge. It
describes the currently routed backend contract. The authoritative
machine-readable contract is `docs/contracts/openapi/openapi.yaml`; when
`API_DOCS_ENABLED=true`, the same contract is available through Swagger UI at
`/docs/` and as YAML at `/openapi.yaml`.

## 1. Identity and CPO Rules

- An app account is one `customers` row owned by exactly one CPO.
- Account uniqueness is `(cpo_id, normalized email)`, not email alone.
- The same person may register the same email and password under CPO A and CPO
  B. Those are separate accounts with separate customer IDs, profiles,
  passwords, wallets, challenges, and sessions.
- A customer cannot move an account between CPOs. They sign up separately in
  the other CPO app.
- `users` is reserved for Superadmin and CPO staff. The app never receives or
  authenticates a global administrative user ID.
- Every `/api/v1/app/...` request must carry the current
  `X-CPO-App-ID`, including signup, login, refresh, recovery, and protected
  calls.
- The app ID selects a CPO before authentication. It is routing metadata, not a
  secret and not authority. Protected calls additionally require a validated
  customer bearer token and the supplied app ID must match that customer's
  current CPO app ID.
- The backend can replace a CPO's dummy app ID with its live app ID. Treat the
  app ID as deploy-time app configuration, not customer state.

## 2. Base URL, Environment Selection, Headers, and API Explorer

The frontend uses one configured CMS API base for the selected environment.
Do not invent a second origin per endpoint, and do not put the CPO app ID into
the URL. The shared base stops at `/api/v1`; each route group appends its own
path.

| Environment | API origin (`API_ORIGIN`) | Shared API base (`API_BASE_URL`) | User App root (`USER_APP_ROOT`) | Credential root (`USER_APP_AUTH_ROOT`) |
| --- | --- | --- | --- | --- |
| Local backend on the same development machine | `http://127.0.0.1:8080` | `http://127.0.0.1:8080/api/v1` | `http://127.0.0.1:8080/api/v1/app` | `http://127.0.0.1:8080/api/v1/app/auth` |
| Current shared development deployment | `https://dev-evcmsnew.transev.site` | `https://dev-evcmsnew.transev.site/api/v1` | `https://dev-evcmsnew.transev.site/api/v1/app` | `https://dev-evcmsnew.transev.site/api/v1/app/auth` |
| Another approved deployment | `<configured-https-origin>` | `<configured-https-origin>/api/v1` | `<configured-https-origin>/api/v1/app` | `<configured-https-origin>/api/v1/app/auth` |

The repository does not define a separate production hostname. A frontend or
AI agent must receive that deployment origin through its environment
configuration; it must not guess one.

Frontend environment examples:

```dotenv
# local frontend configuration
VITE_EV_CMS_API_ORIGIN=http://127.0.0.1:8080

# shared development frontend configuration
VITE_EV_CMS_API_ORIGIN=https://dev-evcmsnew.transev.site
```

Store the origin without a trailing slash. `API_ORIGIN` is only the scheme,
host, and optional port; it must not contain a path, `/api/v1`, or a route-group
path. `API_BASE_URL` must contain `/api/v1` exactly once and must not contain
`/app/auth`, `/cpo`, or `/platform`.
The backend server listen address `0.0.0.0:8080` is not a browser base URL;
use `127.0.0.1`, `localhost`, or the explicitly approved host/IP that resolves
to the backend.

For an AI coding agent, the deterministic selection rule is:

1. Read the frontend environment's `VITE_EV_CMS_API_ORIGIN` (or the
   framework-equivalent public runtime variable).
2. If developing against the local backend and no override is supplied, use
   `http://127.0.0.1:8080`.
3. If using the shared development deployment, use
   `https://dev-evcmsnew.transev.site`.
4. If neither applies, ask for the approved API origin; never infer a URL from
   a database name, CPO app ID, frontend host, mockup, or old documentation.

The CPO app ID is a separate trusted frontend configuration value. The API
origin selects the CMS deployment; `X-CPO-App-ID` selects the CPO application
within that deployment. One white-label CPO build may therefore use the same
API base with its own app-ID value.

Route-group derivation for an AI agent or shared frontend client is:

```ts
const API_BASE_URL = `${API_ORIGIN.replace(/\/+$/, "")}/api/v1`;
const USER_APP_ROOT = `${API_BASE_URL}/app`;
const USER_APP_AUTH_ROOT = `${USER_APP_ROOT}/auth`;
const CPO_ROOT = `${API_BASE_URL}/cpo`;
const PLATFORM_ROOT = `${API_BASE_URL}/platform`;
```

`USER_APP_ROOT` owns current customer-facing app resources. Use it for `me`,
profile, hubs, chargers, pricing, favorites, wallet, and recharge endpoints;
for example, call `GET ${USER_APP_ROOT}/chargers`.

`USER_APP_AUTH_ROOT` owns credential and session operations only: signup,
login, refresh, password recovery/change, session list/revocation, and logout.
`CPO_ROOT` and `PLATFORM_ROOT` are administrative route groups and must not be
called by the customer app.

This is a route migration, not an alias arrangement. The former resource URLs
under `/api/v1/app/auth` (for example, `/api/v1/app/auth/chargers`) are no
longer registered. Update the frontend and backend together to the canonical
`/api/v1/app/...` resource URLs; do not retry a `404` by calling an undocumented
legacy path.

The shared-development hostname identifies a deployment, not a source checkout.
Use these new resource paths against it only after this source revision has
been deployed there. Until then, obtain that deployment's served
`/openapi.yaml` (when enabled) or deploy the matching backend and frontend
together; do not assume an unpublished local route migration is live.

Headers on every currently callable User App API request under
`USER_APP_ROOT` (including its `/auth` child):

```http
X-CPO-App-ID: <configured-current-cpo-app-id>
Accept: application/json
```

Add `Content-Type: application/json` when sending JSON. Add this header on
protected requests:

```http
Authorization: Bearer <access_token>
```

Do not send an access token to signup, login, OTP verification, refresh, or
forgot/reset-password endpoints. Refresh uses the refresh token in its JSON
body. Health and documentation URLs are not User App API calls and do not
require `X-CPO-App-ID`. The backend does not use authentication cookies, so
frontend requests do not need `credentials: "include"`.

If browser access is cross-origin, the deployment must set
`CORS_ALLOW_ALL=true`; the current permissive mode accepts requested headers,
including `Authorization` and `X-CPO-App-ID`. CORS is disabled when that setting
is false.

Health and API documentation use the same origin, without the User App API
prefix:

```text
GET {API_ORIGIN}/health/live
GET {API_ORIGIN}/health/ready
GET {API_ORIGIN}/docs/
GET {API_ORIGIN}/openapi.yaml
```

`/docs/` and `/openapi.yaml` require `API_DOCS_ENABLED=true`; they are not User
App business endpoints.

## 3. Canonical Error Envelope

Every handled error uses:

```json
{
  "error": {
    "code": "invalid_credentials",
    "message": "The supplied credentials are invalid."
  }
}
```

Branch on `error.code`, not the English message.

| HTTP | Code | Frontend action |
| --- | --- | --- |
| `400` | `invalid_request` | Fix malformed/unknown fields or oversized body. |
| `400` | `missing_cpo_app_id` | Treat as app configuration failure. |
| `401` | `invalid_credentials` | Show one generic email/password failure. |
| `401` | `invalid_challenge` | Discard that challenge; restart or resend only when allowed. |
| `401` | `unauthorized` | Attempt one refresh, then clear auth if refresh fails. |
| `401` | `invalid_refresh_token` | Clear tokens, session, and cached `me`; return to login. |
| `403` | `signup_unavailable` | App ID is unknown or its CPO is not active. |
| `403` | `cpo_app_id_mismatch` | Clear auth and report app/account configuration mismatch. |
| `409` | `customer_already_registered` | Offer login or recovery for this CPO. |
| `429` | `rate_limited` | Stop automatic retries and ask the user to wait. |
| `503` | `mail_unavailable` | Email authentication is unavailable; do not show false success. |
| `500` | `internal_error` | Show a retryable generic failure and retain safe user input. |

HTTP success only proves that the challenge and encrypted mail job committed;
it does not prove that SMTP delivery has already finished.

## 4. Shared Types

```ts
export type ApiError = {
  error: { code: string; message: string };
};

export type ChallengeResponse = {
  challenge_id: string;
  expires_at: string;            // UTC RFC 3339
  resend_available_at: string;   // UTC RFC 3339
};

export type CustomerTokenResponse = {
  access_token: string;
  access_token_expires_at: string;
  refresh_token: string;
  session_expires_at: string;
  token_type: "Bearer";
  customer_id: string;
  cpo_id: string;
  cpo_app_id: string;
};

export type CustomerMe = {
  user: {
    id: string;
    email: string;
    full_name: string;
    phone?: string;
    is_verified: boolean;
    last_login_at?: string;
  };
  customer: {
    id: string;
    status: "ACTIVE" | "BLOCKED";
    user_group_id?: string;
  };
  cpo: {
    id: string;
    business_name: string;
    app_id: string;
    app_id_mode: "DUMMY" | "LIVE";
  };
  wallet: {
    id: string;
    balance: string; // exact decimal; use a decimal/money library
    currency: string;
  };
};

export type UpdateCustomerProfileRequest = {
  full_name: string;
  phone?: string | null; // omit to preserve; null to clear
};

export type CustomerUser = CustomerMe["user"];

export type CustomerSession = {
  id: string;
  ip_address?: string;
  user_agent: string;
  created_at: string;
  last_seen_at: string;
  expires_at: string;
  is_current: boolean;
};

export type CustomerHubSummary = {
  id: string;
  name: string;
  address: string;
  latitude: number;
  longitude: number;
  twenty_four_seven_open_status: boolean;
  customer_visible: true;
  charger_count: number;
  is_favorite: boolean;
};

export type CustomerNetworkStatus =
  | "ACTIVE"
  | "INACTIVE"
  | "SUSPENDED"
  | "UNDERMAINTENANCE"
  | "DECOMMISSIONED";

export type CustomerConnector = {
  id: string;
  connector_number: number;
  connector_type: string;
  connector_total_capacity: number;
  status: CustomerNetworkStatus;
  availability: "UNKNOWN";
};

export type CustomerCharger = {
  id: string;
  hub_id: string;
  charger_id: string; // six-character public ID
  vendor?: string;
  model?: string;
  max_power_kw: number;
  ocpp_version: string;
  status: CustomerNetworkStatus;
  charger_image_url?: string; // authenticated relative API path
  hub_name?: string;
  hub_address?: string;
  hub_latitude?: number;
  hub_longitude?: number;
  twenty_four_seven_open_status?: boolean;
  distance_km?: number;
  availability: "UNKNOWN";
  is_favorite: boolean;
  connectors: CustomerConnector[];
};

export type CustomerChargerList = {
  chargers: CustomerCharger[];
  next_before?: string;
  next_before_id?: string;
  has_more: boolean;
};

export type CustomerWalletDetails = {
  id: string;
  balance: string;
  currency: string;
  updated_at: string;
};

export type CustomerWalletTransaction = {
  id: string;
  amount: string;
  transaction_type: "CREDIT" | "DEBIT";
  description: string;
  session_id?: string;
  status: "PENDING" | "COMPLETED" | "FAILED" | "REVERSED";
  created_at: string;
};

export type CustomerWalletTransactionList = {
  wallet: CustomerWalletDetails;
  transactions: CustomerWalletTransaction[];
  next_before?: string;
  next_before_id?: string;
  has_more: boolean;
};

export type CustomerRechargeOrder = {
  recharge_order_id: string;
  provider: "RAZORPAY";
  provider_order_id: string;
  amount: string;
  amount_minor: number;
  currency: "INR";
  provider_key_id?: string; // present when creating the checkout order
  status: "PAYMENT_PENDING" | "PAID";
  created_at: string;
};

export type CustomerRechargeVerifyRequest = {
  razorpay_order_id: string;
  razorpay_payment_id: string;
  razorpay_signature: string;
};

export type CustomerHub = CustomerHubSummary & {
  chargers: CustomerCharger[];
};

export type CustomerFavorites = {
  hubs: CustomerHubSummary[];
  chargers: CustomerCharger[];
  next_hub_before?: string;
  next_hub_before_id?: string;
  has_more_hubs: boolean;
  next_charger_before?: string;
  next_charger_before_id?: string;
  has_more_chargers: boolean;
};

export type CustomerPriceResponse = {
  status: "AVAILABLE" | "UNAVAILABLE";
  effective_at: string;
  currency?: string;
  price_per_kwh?: string;
  idle_fee_per_minute?: string;
  gst?: {
    sgst_rate: string;
    cgst_rate: string;
    igst_rate: string;
  };
  unavailable_reason?: "no_eligible_tariff";
};
```

`me.user` is a compatibility presentation object. `me.user.id` always equals
`me.customer.id`; both are the CPO-local `customer_id`.

## 5. Endpoint Matrix

| Method and path | Bearer | Success | Purpose |
| --- | --- | --- | --- |
| `POST /auth/signup` | No | `202 ChallengeResponse` | Start verified signup. |
| `POST /auth/signup/verify` | No | `201 SignupResponse` | Create customer and wallet. |
| `POST /auth/signup/resend` | No | `202 ChallengeResponse` | Replace signup OTP challenge. |
| `POST /auth/login` | No | `202 ChallengeResponse` | Verify password and start mail OTP. |
| `POST /auth/login/verify` | No | `200 CustomerTokenResponse` | Create authenticated session. |
| `POST /auth/login/resend` | No | `202 ChallengeResponse` | Replace login OTP challenge. |
| `POST /auth/refresh` | No | `200 CustomerTokenResponse` | Rotate refresh and renew access. |
| `POST /auth/password/forgot` | No | `202 MessageResponse` | Enumeration-safe recovery start. |
| `POST /auth/password/reset/resend` | No | `202 ChallengeResponse` | Replace recovery ID/code pair. |
| `POST /auth/password/reset` | No | `200 MessageResponse` | Replace forgotten password. |
| `GET /me` | Yes | `200 CustomerMe` | Bootstrap authenticated app state. |
| `PATCH /profile` | Yes | `200 CustomerUser` | Update this account's name or phone. |
| `GET /hubs` | Yes | `200 CustomerHubList` | List published hubs in this CPO. |
| `GET /hubs/{hub_id}` | Yes | `200 CustomerHub` | Read one published hub and attached chargers. |
| `GET /chargers` | Yes | `200 CustomerChargerList` | Search/filter published chargers, including optional near-me results. |
| `GET /chargers/{charger_id}` | Yes | `200 CustomerCharger` | Read one attached charger by public ID. |
| `GET /chargers/{charger_id}/image` | Yes | `200 image/*` | Download one uploaded charger image by public ID. |
| `GET /favorites` | Yes | `200 CustomerFavorites` | List current published favorites. |
| `PUT /favorite-hubs/{hub_id}` | Yes | `204` | Idempotently save a published hub. |
| `DELETE /favorite-hubs/{hub_id}` | Yes | `204` | Idempotently remove a hub favorite. |
| `PUT /favorite-chargers/{charger_id}` | Yes | `204` | Idempotently save an attached published charger by its public `charger_id`, never its UUID `id`. |
| `DELETE /favorite-chargers/{charger_id}` | Yes | `204` | Idempotently remove a charger favorite by its public `charger_id`, never its UUID `id`. |
| `GET /hubs/{hub_id}/price` | Yes | `200 CustomerPriceResponse` | Resolve the current informational hub price. |
| `GET /chargers/{charger_id}/price` | Yes | `200 CustomerPriceResponse` | Resolve the current informational charger price. |
| `GET /wallet` | Yes | `200 CustomerWalletResponse` | Read the current exact-decimal wallet balance. |
| `GET /wallet/transactions` | Yes | `200 CustomerWalletTransactionList` | Read this customer’s bounded wallet ledger history. |
| `POST /wallet/recharge/orders` | Yes | `201 CustomerRechargeOrder` | Create an idempotent Razorpay checkout order. |
| `POST /wallet/recharge/verify` | Yes | `200 CustomerRechargeOrder` | Verify a captured Razorpay payment and credit the wallet once. |
| `GET /auth/sessions` | Yes | `200 SessionList` | List this account's active sessions. |
| `DELETE /auth/sessions/{session_id}` | Yes | `204` | Revoke one owned session. |
| `POST /auth/logout` | Yes | `204` | Revoke current session. |
| `POST /auth/logout-all` | Yes | `204` | Revoke all sessions for this CPO account. |
| `POST /auth/password/change` | Yes | `200 MessageResponse` | Change password and revoke all sessions. |

Every listed path is relative to `USER_APP_ROOT` (`/api/v1/app`). Only rows
whose path starts with `/auth` use `USER_APP_AUTH_ROOT`.

### 5.1 Published Network Discovery

The discovery endpoints are read-only and use the same customer auth plus
matching app-ID header as `/me`. The backend derives CPO and customer scope
from the validated session. The frontend must never send a customer ID or CPO
ID to select ownership.

`GET /hubs` uses bounded keyset pagination. Preserve both `next_before` and
`next_before_id` together; discard them when `q` changes. A `customer_visible`
hub is explicitly published by a CPO ADMIN. Unpublished hubs, independent
chargers, and cross-CPO resources are not returned.

The DB-backed `status` on chargers and connectors is the CPO's static CMS
administrative lifecycle (`ACTIVE`, `INACTIVE`, `SUSPENDED`,
`UNDERMAINTENANCE`, or `DECOMMISSIONED`). It is not a live OCPP/HAL state.
`connector_total_capacity` is the connector capacity value; the obsolete
`max_current` and `max_voltage` fields are not returned.

The backend deliberately does not contact HAL in this slice. `availability` is
`UNKNOWN` for chargers and connectors. Do not render either `availability` or
the CMS `status` as online, available, offline, or live state; a later
HAL-backed contract must define those states, reconnect behavior, and REST
recovery before the app depends on them.

The safe projection omits OCPP identity, serial number, last-seen timestamps,
sanctioned load, CPO notes, and audit data. Favorite flags are present in the
same safe projection used by the favorite list.

When present, `charger_image_url` is a relative path such as
`/api/v1/app/chargers/a1b2c3/image`; it is not the storage path. The image
route requires the normal Bearer and `X-CPO-App-ID` headers, so a browser
`<img>` tag cannot call it directly. Fetch it as a blob with the app's normal
authenticated client, then use a temporary object URL:

```ts
const response = await fetch(`${origin}${charger.charger_image_url}`, {
  headers: {
    Authorization: `Bearer ${accessToken}`,
    "X-CPO-App-ID": cpoAppID,
  },
});
if (!response.ok) throw new Error("charger image unavailable");
const imageURL = URL.createObjectURL(await response.blob());
```

`404 charger_image_not_found` means the published charger has no safe uploaded
image (or its stored file is unavailable). The API only serves JPEG, PNG, GIF,
and WebP content from the existing upload directory.

Favorite mutations are now callable. `PUT` is idempotent and accepts no body;
`DELETE` is idempotent and returns `204` when the favorite is absent. For
`/favorite-chargers/{charger_id}`, pass `CustomerCharger.charger_id`: it is the
six-character lowercase public ID. Never pass `CustomerCharger.id`, which is
the internal UUID. A hub or charger must be published and in the current CPO
when it is added. If a CPO later unpublishes the resource, `GET /favorites`
omits it while the durable favorite may remain until the customer removes it.
This preserves the saved intent without leaking unpublished inventory.

`GET /favorites` uses independent bounded cursors for hubs and chargers:
preserve each `next_*` pair together and send it back as the corresponding
`hub_before`/`hub_before_id` or `charger_before`/`charger_before_id` pair.

`GET /hubs` accepts `q`, `limit`, `before`, and `before_id`. `q` is a
case-insensitive name/address search with a maximum of 255 characters. The
default page size is 25 and the maximum is 100; cursor timestamp and ID must
always be sent as a pair.

`GET /chargers` accepts the following query parameters. All ordinary list
queries default to `limit=25` and reject a limit above 100.

| Parameter | Meaning and validation |
| --- | --- |
| `q` | Case-insensitive public charger ID, vendor, model, hub name, or hub address search; maximum 255 characters. |
| `connector_type` | Case-insensitive connector-type match; maximum 50 characters. |
| `min_power_kw`, `max_power_kw` | Non-negative numbers; minimum cannot exceed maximum. |
| `open_24_hours` | Boolean hub opening-hours filter. |
| `before`, `before_id` | Ordinary descending `(created_at, id)` keyset cursor; both are required together. |
| `lat`, `lng` | Customer location for a near-me query; both are required together (`-90..90`, `-180..180`). |
| `radius_km` | Only valid with `lat` and `lng`; greater than 0 and at most 100; defaults to 10. |

Near-me results are ordered by calculated distance and are intentionally
bounded without a continuation cursor (`has_more` is false and no `next_*`
cursor is returned). A location query cannot include `before`/`before_id`.
All results remain limited to attached chargers in published hubs belonging to
the authenticated CPO. Stored CMS status is not live availability: charger and
connector `availability` remains `UNKNOWN` until the separate HAL contract
exists.

### 5.2 Wallet Reads

`GET /wallet` returns the authenticated customer’s CPO-local wallet with an
exact two-decimal balance string, currency, and durable wallet update time.
`GET /wallet/transactions` returns only that wallet’s transactions in
descending `(created_at, id)` keyset order. It accepts `limit` (default 25,
maximum 100) plus the paired `before` and `before_id` cursor. Preserve
`next_before` and `next_before_id` together when fetching the next page. The
response does not expose internal idempotency keys, recharge-order IDs, or
provider credentials.

Wallet recharge is implemented through the two Razorpay routes below. A
successful verified recharge appears as a completed `CREDIT` ledger entry.
Refund execution, charging-session billing, charging bills, payment-provider
history, webhook ingestion, settlement reconciliation, and general transaction
history beyond this wallet ledger are not implemented.

`POST /wallet/recharge/orders` requires an `Idempotency-Key` header (1–120
characters; CR, LF, and NUL are rejected) and a body such as `{"amount":"500.00"}`.
`amount` is a positive INR decimal with at most two decimal places. Reuse the
same key only for the same amount. The response supplies the public Razorpay
`provider_key_id`, `provider_order_id`, `amount_minor`, and `currency` for a
checkout SDK while the order is `PAYMENT_PENDING`; an idempotent replay of an
already `PAID` order can omit `provider_key_id`. The frontend must never
receive or store the CPO key secret.

After checkout succeeds, send the provider-returned order ID, payment ID, and
signature to `POST /wallet/recharge/verify`. The backend verifies the signature,
fetches the payment through Razorpay, requires captured status and exact order
amount/currency matches, then atomically credits the wallet. Do not treat an
authorized-but-not-captured response as funded. Repeating a successful verify
with the same payment ID is safe; a different payment ID for an already paid
order returns `409 recharge_already_completed`.

Treat `400 invalid_idempotency_key`, `400 invalid_amount`,
`400 invalid_payment_signature`, `404 recharge_order_not_found`, and the
`409` payment/idempotency errors as user- or checkout-flow errors rather than
generic retry candidates. If order creation returns
`502 payment_provider_unavailable`, retry creation only after recovery and
with a **new** idempotency key: the failed key is durably terminal. If
verification returns that `502`, retry `/wallet/recharge/verify` with the same
provider-returned IDs and signature. `503 payment_provider_not_configured`
means this CPO cannot currently offer wallet recharge.

### 5.3 Informational Price Display

The price routes are authenticated, CPO-scoped reads. The server chooses the
tariff at `effective_at`; the frontend must not reconstruct precedence from CPO
tariff rows. For a charger read, the precedence is:

1. matching UserGroup tariff;
2. generic charger tariff;
3. generic hub tariff.

In the current backend schema, “User Tariff” is the tariff whose
`user_group_id` matches the authenticated customer’s existing group assignment.
No new group-management or per-customer tariff API is introduced here. A
matching UserGroup tariff always wins over generic charger and hub tariffs. If
both a matching group/charger and group/hub row exist, the charger row is the
more-specific tie-breaker within the UserGroup tier. A customer without a group
uses only generic tariffs. A hub-price read cannot use a charger-scoped tariff:
it resolves a matching UserGroup hub tariff first, then the generic hub tariff.
`AVAILABLE` includes exact
decimal strings for currency, energy price, idle fee, and GST when referenced;
`UNAVAILABLE` is a valid `200` response with `unavailable_reason` and never a
zero-price fallback. The response is informational and is not a charging or
payment commitment. HAL is not called.

## 6. Signup Flow

### 6.1 Start signup

`POST /auth/signup`

```json
{
  "email": "driver@example.com",
  "password": "<10-to-128-character-password>",
  "full_name": "Example Driver",
  "phone": "+919876543210"
}
```

`phone` is optional. Email is normalized to lowercase. Full name must contain
1–255 characters. Phone must contain 7–15 digits with an optional leading `+`.
Unknown JSON fields, multiple JSON objects, and bodies over 32 KiB fail with
`400 invalid_request`.

On `202`, open the OTP screen and retain the complete `ChallengeResponse`.
Drive resend from `resend_available_at` and expiry from `expires_at`; use server
UTC timestamps rather than a hardcoded duration.

### 6.2 Verify signup

`POST /auth/signup/verify`

```json
{
  "challenge_id": "5cef4c95-a1da-448e-bd7c-19d570cd4497",
  "code": "123456"
}
```

`201 Created`:

```json
{
  "customer_id": "e8a751ff-d7d4-4ce8-ab30-cdd8c8111363",
  "cpo_id": "c821a013-5041-42f7-80c8-aa153cf9d455",
  "wallet_id": "5bd431a7-63f0-4df7-a2f5-1b55112df560"
}
```

Signup creates the account and zero-balance INR wallet. It does not log the
customer in. Navigate to login after success.

### 6.3 Resend signup OTP

`POST /auth/signup/resend`

```json
{"challenge_id":"<current-signup-challenge-id>"}
```

On `202`, atomically replace the stored challenge and timing values. The old
challenge and OTP are invalid immediately.

## 7. Login, OTP, and Token Bootstrap

### 7.1 Password step

`POST /auth/login`

```json
{
  "email": "driver@example.com",
  "password": "<password>"
}
```

Missing account, wrong password, blocked account, and lockout intentionally
share `401 invalid_credentials`. On `202`, show the login OTP screen and retain
the returned challenge/timestamps.

### 7.2 Verify login OTP

`POST /auth/login/verify`

```json
{
  "challenge_id": "<current-login-challenge-id>",
  "code": "123456"
}
```

On `200`, replace the complete local token bundle with the returned
`CustomerTokenResponse`, then immediately call `GET /me`. Do not decode the
encrypted access token in the frontend; the response and `/me` are the client
contract.

### 7.3 Resend login OTP

`POST /auth/login/resend`

```json
{"challenge_id":"<current-login-challenge-id>"}
```

On `202`, replace the prior challenge and timing fields. The prior code cannot
complete login.

## 8. Refresh and Request Serialization

`POST /auth/refresh`

```json
{"refresh_token":"<current-opaque-refresh-token>"}
```

Refresh tokens are one-time rotating credentials. A successful response
contains both a new access token and a new refresh token. Replace both together
before releasing queued API requests. Reusing an already consumed refresh
token is treated as possible theft and revokes that session.

Use one refresh promise/mutex per app instance:

```ts
let refreshInFlight: Promise<CustomerTokenResponse> | null = null;

async function refreshOnce(): Promise<CustomerTokenResponse> {
  if (!refreshInFlight) {
    refreshInFlight = postRefresh(currentRefreshToken)
      .then(next => {
        replaceTokenBundleAtomically(next);
        return next;
      })
      .finally(() => { refreshInFlight = null; });
  }
  return refreshInFlight;
}
```

For a protected request returning `401 unauthorized`, perform at most one
serialized refresh and replay that request once. Never refresh recursively. On
`invalid_refresh_token`, clear authentication state and route to login.

## 9. Authenticated Bootstrap and Sessions

### 9.1 Current account

`GET /me` returns the canonical `CustomerMe`. Use it to initialize the
customer, CPO branding identity, and wallet summary. Optional fields are
omitted rather than returned as `null`.

The backend revalidates the durable session, active customer, active CPO,
wallet ownership, and current CPO app ID. A token alone is not durable
authority.

### 9.2 Edit profile

`PATCH /profile` updates the authenticated CPO-local account only:

```json
{
  "full_name": "Asha Das",
  "phone": "+919876543210"
}
```

`full_name` is required and must be 1–255 trimmed characters. `phone` is
optional and must contain 7–15 digits with an optional leading `+`. Omit
`phone` to preserve the current value; send JSON `null` to clear it. Email,
password, status, group, wallet, sessions, CPO, and identifiers cannot be
changed by this route.

The response is the canonical `CustomerUser` projection. Treat it as the new
local profile state and do not replace the complete `CustomerMe` bootstrap
object with it. The backend returns `Cache-Control: no-store` and records only
changed field names in the CPO-scoped audit event.

Errors are `400 invalid_request`, `400 invalid_full_name`,
`400 invalid_phone`, `401 unauthorized`, or `403 cpo_app_id_mismatch`.

### 9.3 Session list

`GET /auth/sessions`

```json
{
  "sessions": [
    {
      "id": "5cef4c95-a1da-448e-bd7c-19d570cd4497",
      "ip_address": "127.0.0.1",
      "user_agent": "Example Mobile App",
      "created_at": "2026-08-04T09:00:00Z",
      "last_seen_at": "2026-08-04T09:05:00Z",
      "expires_at": "2026-09-03T09:00:00Z",
      "is_current": true
    }
  ]
}
```

Only active, unexpired sessions for this exact `(cpo_id, customer_id)` are
returned.

### 9.4 Revoke one session

`DELETE /auth/sessions/{session_id}` returns `204`. The customer may revoke their
current session; if `is_current` was true, clear local authentication state
immediately. `404 session_not_found` means the session is not owned by this
account or does not exist.

### 9.5 Logout

- `POST /auth/logout` returns `204` and revokes the current session.
- `POST /auth/logout-all` returns `204` and revokes all sessions for only this
  CPO-local account.

Clear local tokens even if logout fails due to an already invalid session.
Logout-all does not affect a same-email account under another CPO and does not
affect Superadmin/CPO-staff sessions.

## 10. Password Recovery and Change

### 10.1 Forgot password

`POST /auth/password/forgot`

```json
{"email":"driver@example.com"}
```

For an active CPO, malformed, unknown, and eligible emails deliberately return
the same `202` response:

```json
{"message":"If the customer account is eligible, a password reset code will be sent."}
```

An eligible email contains all three values needed by recovery:
`challenge_id`, six-digit `code`, and expiry. The generic HTTP response does
not contain the challenge ID. Support an email deep link or manual entry of the
recovery ID/code.

### 10.2 Resend recovery

`POST /auth/password/reset/resend`

```json
{"challenge_id":"<current-recovery-id>"}
```

On `202`, the mail contains a replacement recovery ID and OTP. Replace the
stored pair; the old pair is unusable.

### 10.3 Complete recovery

`POST /auth/password/reset`

```json
{
  "challenge_id": "<current-recovery-id>",
  "code": "123456",
  "new_password": "<10-to-128-character-password>"
}
```

`200` returns:

```json
{"message":"Password reset. Sign in again."}
```

Success clears lockout, revokes every session for this one CPO account, and
invalidates its other outstanding login/reset challenges. Route to login and
clear any existing token bundle.

### 10.4 Authenticated password change

`POST /auth/password/change`

```json
{
  "current_password": "<current-password>",
  "new_password": "<different-10-to-128-character-password>"
}
```

`200` returns:

```json
{"message":"Password changed. All sessions were revoked; sign in again."}
```

The current session is revoked too. Clear auth state and route to login. Handle
`400 password_reused`, `400 invalid_password`, and
`401 invalid_current_password` inline.

## 11. Minimal Fetch Client

```ts
const API_ORIGIN = getRequiredPublicEnv("VITE_EV_CMS_API_ORIGIN");
const API_BASE_URL = `${API_ORIGIN.replace(/\/+$/, "")}/api/v1`;
const USER_APP_ROOT = `${API_BASE_URL}/app`;
const USER_APP_AUTH_ROOT = `${USER_APP_ROOT}/auth`;
const CPO_APP_ID = "<configured-app-id>";

async function api<T>(
  path: string,
  init: RequestInit = {},
  accessToken?: string,
): Promise<T> {
  const headers = new Headers(init.headers);
  headers.set("Accept", "application/json");
  headers.set("X-CPO-App-ID", CPO_APP_ID);
  if (init.body) headers.set("Content-Type", "application/json");
  if (accessToken) headers.set("Authorization", `Bearer ${accessToken}`);

  const response = await fetch(`${USER_APP_ROOT}${path}`, { ...init, headers });
  if (response.status === 204) return undefined as T;

  const body = await response.json().catch(() => undefined);
  if (!response.ok) {
    const failure = body as ApiError | undefined;
    throw Object.assign(new Error(failure?.error?.message ?? "Request failed"), {
      status: response.status,
      code: failure?.error?.code ?? "unknown_error",
    });
  }
  return body as T;
}
```

Keep the CPO app ID outside editable customer input. For a white-label app,
compile or deploy one trusted app-ID value per branded CPO distribution.
Keep `API_ORIGIN` outside editable customer input as well. The frontend must
not let a customer change the backend host or CPO app ID from a profile form,
query parameter, or deep link.

## 12. Frontend State and Security Rules

- Model signup OTP, login OTP, and password recovery as different state
  machines even though their challenge shapes are similar.
- Prefer OS-protected mobile storage for refresh tokens. Avoid browser
  `localStorage` when a safer same-origin or native secure store exists.
- Never log passwords, OTPs, access tokens, refresh tokens, or recovery IDs.
- Never put tokens or OTPs in analytics, URLs, crash reports, or screenshots.
- Treat timestamps as UTC instants and format them in the user's locale.
- Preserve exact decimal wallet strings until using a decimal/money library.
- Disable duplicate submit buttons while each command is in flight.
- After resend, verification, password reset/change, or logout, delete
  superseded client state immediately.
- Do not infer ownership from IDs in app storage. The server derives trusted
  scope from the token and current app ID.

## 13. Currently Unsupported Customer UI

The authentication boundary and the listed read-only discovery/wallet surfaces
are ready. These customer-product surfaces are not part of the currently
routed contract and the frontend must not invent calls for them:

- edit email;
- RFID/access-token management;
- start/stop charging and live transaction telemetry;
- refunds, charging bills, or payment-provider history beyond the recharge
  order/verification flow;
- customer notifications and realtime feeds.

Authenticated name and phone editing is now available through `PATCH /profile`.
Keep email editing and the other listed customer-product surfaces disabled
until their routes appear in the same OpenAPI document.

## 14. Frontend Acceptance Checklist

- Every User App request carries the configured `X-CPO-App-ID`.
- Signup finishes at account creation, then explicitly enters login.
- Login creates no local authenticated state until OTP verification succeeds.
- The token bundle is replaced atomically on login and every refresh.
- Only one refresh runs at a time; consumed refresh tokens are never retried.
- `/me` is fetched after login and its IDs are treated as opaque UUIDs.
- `user.id === customer.id` is understood as one CPO-local account.
- Password change/reset and logout clear the appropriate local state.
- Session revocation handles current-session revocation.
- Errors branch on stable codes and handle `429`/`503` without retry loops.
- No unsupported customer-product endpoint is assumed.
