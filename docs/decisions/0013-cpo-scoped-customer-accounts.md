# 0013: CPO-Scoped Customer Accounts

Status: Implemented; database-backed lifecycle verification requires `TEST_DATABASE_URL`

Date: 2026-08-04

## Decision

- `users` remains the global identity store only for platform Superadmin and
  CPO staff.
- Each `customers` row is the app-user account owned by one CPO. It owns email,
  password, profile, verification, lockout, recovery, sessions, and refresh
  lineage.
- Email uniqueness is normalized per CPO. The same email may have a separate
  account under another CPO. Its password may be the same or different; there
  is no cross-CPO password-uniqueness rule.
- `X-CPO-App-ID` remains app routing metadata on every customer request; it is
  never authority and must match the customer session's CPO when authenticated.
- There are no existing customer records, so the additive migration can remove
  the legacy global-user customer link without a data backfill. It must fail
  safely if that assumption is ever false at migration time.

## Consequences

Customer signup, login, OTPs, password recovery/change, sessions, access-token
subject, trusted principal helpers, and audit actor references use the
CPO-local account. A password update in one CPO affects only that CPO account.
Administrative identity and its sessions remain untouched. `CurrentUserID` and
the `me.user` key remain compatibility shapes whose UUID is the customer ID,
not a global user ID.

This supersedes the global-identity reuse portion of ADR 0005 and the
global-user/password portions of ADR 0006. Their CPO app-ID routing and tenant
isolation decisions remain valid.
