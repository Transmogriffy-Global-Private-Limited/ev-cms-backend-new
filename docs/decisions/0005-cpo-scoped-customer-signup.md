# 0005: CPO-Scoped Customer Signup

Status: Accepted

Date: 2026-07-23

## Context

Each CPO distributes its own application, while a person's login identity is
global and their customer relationship is tenant-scoped. A client-embedded app
ID can identify the intended CPO but cannot be kept secret. The old CMS held
pending passwords and OTPs in process memory and attached users through a
global administrator identifier.

## Decision

- Resolve public signup through the current `X-CPO-App-ID` of an active CPO.
- Treat that ID as routing metadata, not a credential or anti-abuse mechanism.
- Verify email before creating durable identity or customer records.
- Store pending signup state durably with only an Argon2id password hash and an
  HMAC-protected OTP.
- Reuse an existing active global identity without replacing its password or
  profile.
- Create the CPO customer and its INR wallet atomically after verification.
- Apply durable rate limits and use the encrypted mail outbox.
- Keep customer login/session issuance outside this implementation slice.

## Consequences

- Hardcoding the app ID in a frontend is operationally acceptable but provides
  no secrecy.
- A verified mailbox can attach its existing global identity to another CPO.
- A newly registered customer cannot authenticate until the separate customer
  login/session contract is implemented.
- CPO suspension or app-ID rotation immediately prevents new signup steps that
  use stale routing state.
