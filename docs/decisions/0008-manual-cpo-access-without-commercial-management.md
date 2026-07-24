# ADR 0008: Manual CPO Access Without Commercial Management

Status: Accepted

Date: 2026-07-24

## Context

The subscription, entitlement, platform-invoice, and platform-payment prototype
was implemented and migrations seven and eight reached the development VPS.
The product direction is now simpler: TransEV will decide which CPOs may use
the platform manually. The CMS is not responsible for selling, invoicing, or
collecting tenant access.

Rewriting or deleting already-applied migrations would make deployed databases
unreliable. Dropping the prototype tables would also destroy any records that
may have been created during development.

## Decision

- CPO access is controlled only by the existing platform-superadmin activation
  and suspension operations.
- No tenant subscription, entitlement package, platform invoice, or platform
  payment API is exposed.
- No commercial worker or payment event changes CPO lifecycle automatically.
- Runtime modules, routes, models, workers, mail producers, OpenAPI operations,
  and current contracts for the prototype are removed.
- Migrations seven and eight remain immutable historical migrations.
- Forward migration nine moves their tables into the non-runtime
  `retired_commercial` schema without deleting rows.
- Retired worker records become non-required and `DISABLED`.
- Migration nine refuses to proceed while related mail is pending or
  processing, preventing silent message loss.
- Its down migration restores the tables, immutability triggers, and worker
  requirements if the application is intentionally rolled back.

## Consequences

- A superadmin can create, activate, suspend, and rotate app identity for a CPO
  without any commercial record.
- The runtime and API become smaller and downstream clients no longer integrate
  subscription or platform-billing contracts.
- The active OpenAPI contract contains 44 operations instead of 74.
- Historical prototype data remains recoverable but is not authoritative
  runtime state.
- Tenant Razorpay credential storage remains because it is CPO-owned
  integration configuration for future charging-customer payments, not
  TransEV platform billing.
- Any future commercial-management feature requires a new explicit product and
  architecture decision rather than reactivating the retired prototype
  silently.
