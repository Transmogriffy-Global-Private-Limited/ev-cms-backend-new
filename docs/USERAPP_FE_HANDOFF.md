# User App Frontend Handoff

This is the implementation handoff for the customer-facing app authentication
surface. It describes the currently routed backend contract. The authoritative
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
- Every `/api/v1/app/auth/...` request must carry the current
  `X-CPO-App-ID`, including signup, login, refresh, recovery, and protected
  calls.
- The app ID selects a CPO before authentication. It is routing metadata, not a
  secret and not authority. Protected calls additionally require a validated
  customer bearer token and the supplied app ID must match that customer's
  current CPO app ID.
- The backend can replace a CPO's dummy app ID with its live app ID. Treat the
  app ID as deploy-time app configuration, not customer state.

## 2. Base URL, Headers, and API Explorer

Production-style base URL example:

```text
https://<cms-api-host>/api/v1/app/auth
```

Headers on every request:

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
body. The backend does not use authentication cookies, so frontend requests do
not need `credentials: "include"`.

If browser access is cross-origin, the deployment must set
`CORS_ALLOW_ALL=true`; the current permissive mode accepts requested headers,
including `Authorization` and `X-CPO-App-ID`. CORS is disabled when that setting
is false.

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
    status: "ACTIVE" | "BLOCKED" | "DELETED";
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
```

`me.user` is a compatibility presentation object. `me.user.id` always equals
`me.customer.id`; both are the CPO-local `customer_id`.

## 5. Endpoint Matrix

| Method and path | Bearer | Success | Purpose |
| --- | --- | --- | --- |
| `POST /signup` | No | `202 ChallengeResponse` | Start verified signup. |
| `POST /signup/verify` | No | `201 SignupResponse` | Create customer and wallet. |
| `POST /signup/resend` | No | `202 ChallengeResponse` | Replace signup OTP challenge. |
| `POST /login` | No | `202 ChallengeResponse` | Verify password and start mail OTP. |
| `POST /login/verify` | No | `200 CustomerTokenResponse` | Create authenticated session. |
| `POST /login/resend` | No | `202 ChallengeResponse` | Replace login OTP challenge. |
| `POST /refresh` | No | `200 CustomerTokenResponse` | Rotate refresh and renew access. |
| `POST /password/forgot` | No | `202 MessageResponse` | Enumeration-safe recovery start. |
| `POST /password/reset/resend` | No | `202 ChallengeResponse` | Replace recovery ID/code pair. |
| `POST /password/reset` | No | `200 MessageResponse` | Replace forgotten password. |
| `GET /me` | Yes | `200 CustomerMe` | Bootstrap authenticated app state. |
| `PATCH /profile` | Yes | `200 CustomerUser` | Update this account's name or phone. |
| `GET /sessions` | Yes | `200 SessionList` | List this account's active sessions. |
| `DELETE /sessions/{session_id}` | Yes | `204` | Revoke one owned session. |
| `POST /logout` | Yes | `204` | Revoke current session. |
| `POST /logout-all` | Yes | `204` | Revoke all sessions for this CPO account. |
| `POST /password/change` | Yes | `200 MessageResponse` | Change password and revoke all sessions. |

Paths in the table are relative to `/api/v1/app/auth`.

## 6. Signup Flow

### 6.1 Start signup

`POST /signup`

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

`POST /signup/verify`

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

`POST /signup/resend`

```json
{"challenge_id":"<current-signup-challenge-id>"}
```

On `202`, atomically replace the stored challenge and timing values. The old
challenge and OTP are invalid immediately.

## 7. Login, OTP, and Token Bootstrap

### 7.1 Password step

`POST /login`

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

`POST /login/verify`

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

`POST /login/resend`

```json
{"challenge_id":"<current-login-challenge-id>"}
```

On `202`, replace the prior challenge and timing fields. The prior code cannot
complete login.

## 8. Refresh and Request Serialization

`POST /refresh`

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

`GET /sessions`

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

`DELETE /sessions/{session_id}` returns `204`. The customer may revoke their
current session; if `is_current` was true, clear local authentication state
immediately. `404 session_not_found` means the session is not owned by this
account or does not exist.

### 9.5 Logout

- `POST /logout` returns `204` and revokes the current session.
- `POST /logout-all` returns `204` and revokes all sessions for only this
  CPO-local account.

Clear local tokens even if logout fails due to an already invalid session.
Logout-all does not affect a same-email account under another CPO and does not
affect Superadmin/CPO-staff sessions.

## 10. Password Recovery and Change

### 10.1 Forgot password

`POST /password/forgot`

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

`POST /password/reset/resend`

```json
{"challenge_id":"<current-recovery-id>"}
```

On `202`, the mail contains a replacement recovery ID and OTP. Replace the
stored pair; the old pair is unusable.

### 10.3 Complete recovery

`POST /password/reset`

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

`POST /password/change`

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
const API_ROOT = "https://<cms-api-host>/api/v1/app/auth";
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

  const response = await fetch(`${API_ROOT}${path}`, { ...init, headers });
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

The authentication boundary is ready. These customer-product surfaces are not
part of this slice and the frontend must not invent calls for them:

- edit email;
- charger/hub discovery and favorites;
- RFID/access-token management;
- start/stop charging and live transaction telemetry;
- wallet funding, charging bills, or payment history;
- customer notifications and realtime feeds.

Authenticated name and phone editing is now available through `PATCH /profile`.
Keep email editing and the other listed customer-product surfaces disabled
until their routes appear in the same OpenAPI document.

## 14. Frontend Acceptance Checklist

- Every app-auth request carries the configured `X-CPO-App-ID`.
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
