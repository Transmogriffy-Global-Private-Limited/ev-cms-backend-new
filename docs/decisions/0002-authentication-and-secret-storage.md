# 0002: Authentication and Tenant Secret Storage

Status: Accepted

Date: 2026-07-23

## Context

The CMS needs one credential boundary for platform superadmins and CPO staff
without collapsing their authorization planes. Administrative access requires
mail OTP, password recovery, durable session control, and helpers for every
later API. CPOs also need to configure payment-provider credentials that neither
the API nor a platform administrator should reveal.

The expected workload does not justify an external identity service, Redis, or
message broker.

## Decision

- Keep authentication inside the Go modular monolith with PostgreSQL as durable
  truth.
- Use global identities and scope each session to either platform or one CPO.
- Require email OTP for platform and CPO administrative login.
- Store passwords with Argon2id.
- Issue short-lived signed-then-encrypted access JWTs and one-time rotating
  opaque refresh tokens.
- Validate durable session and authorization state on protected requests.
- Use a PostgreSQL mail outbox with an in-process SMTP worker.
- Require one explicit encrypted SMTP transport: implicit TLS or mandatory
  STARTTLS. Reject plaintext and conflicting transport configuration.
- Encrypt tenant provider credentials using authenticated encryption bound to
  the CPO and provider; expose metadata only.
- Audit privileged authentication and provider-credential mutations without
  recording credential values.

## Consequences

- SMTP is required for new administrative logins and password recovery.
- The current provider contract uses Hostinger implicit TLS on port 465.
- Database availability is intentionally required for protected requests so
  revocation and tenant suspension are authoritative.
- Access tokens are confidential in addition to being integrity protected, but
  application authorization still comes from validated claims and database
  state.
- Refresh-token theft is contained through rotation and reuse detection.
- Provider credentials can be used by internal services but cannot be retrieved
  through a user-facing endpoint.
- The initial design avoids infrastructure that is unnecessary at current
  scale.

## Deferred

- Public registration and consumer login policy
- Automatic encryption-key rotation
- Authenticator apps, passkeys, SMS, and third-party identity providers
- A general secrets-management product
