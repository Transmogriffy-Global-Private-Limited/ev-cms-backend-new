# CMS and OCPP HAL Boundary

## Current State

The CMS and `OCPPHAL_Go` are separate applications. This repository does not
currently implement a CMS-to-HAL API, event transport, shared client, or
handshake. The existing HAL is not modified by this CMS foundation.

## Ownership

The HAL owns:

- charger connections and OCPP protocol state;
- protocol commands, responses, and correlation;
- raw meter communication;
- exact HAL/OCPP transaction identifiers;
- protocol reconnect and device-level recovery.

The CMS owns:

- platform and CPO administration;
- users, memberships, customers, and tenant authorization;
- commercial network projections;
- tariffs, wallets, billing, payments, reporting, and audit policy;
- CMS-side charging-session projections.

Neither service writes the other service's database. Shared concepts require an
explicit authenticated contract rather than direct table access.

## Current Data Overlap

The CMS schema already has commercial projections such as hubs, chargers,
connectors, and charging sessions. Their presence does not transfer protocol
ownership. In particular:

- CMS `chargers.ocpp_identity` is a mapping value, not a live connection;
- CMS connector state is a business projection, not the protocol source of
  truth;
- `charging_sessions.transaction_id` must preserve the exact HAL-issued value;
- integer meter Wh and immutable tariff/tax snapshots belong in the eventual
  CMS billing projection;
- commands cannot be considered delivered merely because a CMS row changed.

## Forbidden Shortcuts

- Do not import or copy the HAL into the CMS process.
- Do not share GORM models or write the HAL database.
- Do not let the HAL use a user bearer token as service identity.
- Do not invent OCPP transaction IDs in the CMS.
- Do not make WebSocket presence the durable charging-session truth.
- Do not implement remote start/stop without command idempotency and recovery.

## Required Future Contract

The charging-network and charging-lifecycle phases must define:

- service identity and authorization;
- CPO and charger identity mapping;
- command idempotency and correlation IDs;
- event ordering and deduplication;
- exact session and meter-unit semantics;
- retries, timeout, partial failure, and restart recovery;
- reconciliation when either service was unavailable;
- compatibility and versioning;
- audit and observability without leaking credentials.

Until that contract is approved and implemented, documentation and code must
not imply that charger operations, live status, remote start/stop, or charging
recovery are available through this CMS.

## Acceptance Evidence for a Future Handshake

A future integration is not verified by one successful request. Verification
must cover duplicate commands/callbacks, out-of-order events, service restart,
lost response after accepted command, reconnect, reconciliation, tenant
isolation, transaction-ID preservation, and an already-active session during
CPO suspension.
