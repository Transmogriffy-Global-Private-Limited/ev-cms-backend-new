# App-User Authentication Workflow

## Frontend Sequence

The CPO app carries its current `X-CPO-App-ID` on every app-auth request. The
value may be bundled with the frontend because it identifies the CPO but is not
a credential.

New customer:

1. Call `POST /api/v1/app/auth/signup`.
2. Collect the emailed OTP.
3. Call `POST /api/v1/app/auth/signup/verify`.
4. Continue to customer login; signup does not silently create a session.

Returning customer:

1. Call `POST /api/v1/app/auth/login` with email and password.
2. Store only the returned challenge ID and timing fields.
3. Collect the emailed OTP.
4. Call `POST /api/v1/app/auth/login/verify`.
5. Keep the access token in short-lived application state and the refresh token
   in the platform's most protected practical client storage.
6. Call `GET /api/v1/app/auth/me` to bootstrap the user, customer, CPO, and
   wallet view.
7. Atomically replace the refresh token after every successful refresh.

On `401 unauthorized`, attempt refresh only with the current refresh token. On
`401 invalid_refresh_token`, clear all local authentication state. Never retry
with an older refresh token: reuse is treated as possible theft and revokes the
session.

## Backend Handler Sequence

Register an app route behind both customer middleware layers:

```go
protected := router.Group("/api/v1/app")
protected.Use(customerAuthService.Authenticate(), customerauth.RequireAppID())
```

Inside a handler:

```go
principal, ok := customerauth.CurrentPrincipal(ctx)
if !ok {
    // Return the normal unauthorized envelope.
}

userID, _ := customerauth.CurrentUserID(ctx)
customerID, _ := customerauth.CurrentCustomerID(ctx)
cpoID, _ := customerauth.CurrentCPOID(ctx)
appID, _ := customerauth.CurrentCPOAppID(ctx)
```

Use `customerID` and `cpoID` from these helpers for repositories, ownership
filters, jobs, cache keys, events, and audits. A request body may contain a
resource ID, but it must never establish the caller's customer or tenant
authority.

`principal` contains the same validated user, customer, CPO, wallet, and
session context returned by `GET /api/v1/app/auth/me`. Use `service.Me` for the
canonical external response rather than rebuilding its fields ad hoc.

## Session and Password Scope

- Session list/revoke/logout-all affects only this exact customer and CPO.
- Logging out as a customer does not affect CPO staff or platform sessions.
- Passwords belong to the global identity. Password reset/change therefore
  revokes every session for that identity across every CPO and administrative
  plane.
- A blocked customer or suspended CPO invalidates customer access on the next
  request because PostgreSQL authority is revalidated.
