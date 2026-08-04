# 0005: CPO-Scoped Customer Signup

Status: Superseded in part by ADR 0013

Date: 2026-07-23

## Context

Each CPO distributes its own application. At the time of this decision,
customer login identity was global and its customer relationship was
tenant-scoped. A client-embedded app
ID can identify the intended CPO but cannot be kept secret. The old CMS held
pending passwords and OTPs in process memory and attached users through a
global administrator identifier.

## Decision

- Resolve public signup through the current `X-CPO-App-ID` of an active CPO.
- Treat that ID as routing metadata, not a credential or anti-abuse mechanism.
- Verify email before creating durable identity or customer records.
- Store pending signup state durably with only an Argon2id password hash and an
  HMAC-protected OTP.
- Historical decision, superseded: reuse an existing active global identity.
- Create the CPO customer and its INR wallet atomically after verification.
- Apply durable rate limits and use the encrypted mail outbox.
- Keep customer login/session issuance outside this implementation slice.

## Consequences

- Hardcoding the app ID in a frontend is operationally acceptable but provides
  no secrecy.
- ADR 0013 replaces global reuse with independent CPO-local accounts.
- A newly registered customer cannot authenticate until the separate customer
  login/session contract is implemented.
- CPO suspension or app-ID rotation immediately prevents new signup steps that
  use stale routing state.
