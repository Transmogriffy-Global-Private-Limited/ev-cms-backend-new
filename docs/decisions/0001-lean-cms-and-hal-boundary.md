# ADR 0001: Lean multi-tenant CMS with a separate HAL

Status: Accepted

Date: 2026-07-23

## Context

The old CMS mixes weak ownership boundaries with patched charging behavior. The
new repository was an empty scaffold accompanied by an initial schema that
treated CPO as a global user role and attached one CPO profile directly to one
user. The product is software sold to CPO organizations, which then manage
staff, customers, and their charging ecosystem.

The existing Go HAL already owns OCPP runtime behavior and should remain a
separate service.

## Decision

- Model each CPO as a tenant organization.
- Keep login identity separate from platform-superadmin authority, CPO staff
  membership, and CPO customer membership.
- Begin with fixed CPO-wide roles: owner, admin, operator, and viewer.
- Represent CPO commercial access initially through simple lifecycle status
  controlled by platform administration.
- Defer custom permissions, resource scopes, subscription feature matrices, and
  database RLS until concrete requirements justify them.
- Preserve the complete supplied CMS business schema as a tenant-aware data
  baseline even where the corresponding application workflow belongs to a
  later phase.
- Keep the HAL in a separate service and database. Use authenticated,
  idempotent service contracts when integration is implemented.
- Build the CMS as a Go modular monolith backed by PostgreSQL.

## Consequences

- A person can work for or be a customer of multiple CPOs without mixing
  tenant-owned data.
- Staff authorization remains understandable and cheap to implement initially.
- Fine-grained access and commercial plan enforcement are not available in the
  first release.
- The complete business domain can be implemented incrementally without
  redesigning or omitting its base data relationships.
- The CMS cannot treat a successful HAL command request as proof of an OCPP
  state transition; later session work must consume durable HAL callbacks.
