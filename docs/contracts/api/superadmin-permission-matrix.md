# SuperAdmin API Permission Matrix

## Authority model

This is a manual classification of every routed SuperAdmin-facing operation in
the current OpenAPI contract. It is an integration and review aid, not a new
authorization engine.

Every `/api/v1/platform/*` operation below is enforced by the same server-side
gate: authenticated `PLATFORM` SuperAdmin authority. The backend does **not**
currently enforce a finer permission per row. The `Classification` column is
the frontend's risk/confirmation grouping; it must not be emitted as a claim
that a less-privileged platform role can call a subset of endpoints.

The 12 shared `/api/v1/auth/*` operations are included because they are needed
by the SuperAdmin client. Public recovery starts remain generic and do not
grant platform authority by themselves.

The complete support-desk lifecycle, full-thread response behavior, retry and
privacy rules, and intentionally unsupported support features are in
[`../../guides/workflows/superadmin-support-tickets.md`](../../guides/workflows/superadmin-support-tickets.md).
The four support rows below remain this document's manual authority/risk
classification.

| API | Enforced authority | Classification | FE rule |
| --- | --- | --- | --- |
| `POST /api/v1/auth/login` | Public administrative login start; request must use `scope: PLATFORM` | Authentication | Start OTP only; do not disclose identity existence. |
| `POST /api/v1/auth/2fa/verify` | Valid pending challenge | Authentication | Establish platform session only after returned scope is `PLATFORM`. |
| `POST /api/v1/auth/2fa/resend` | Valid pending challenge | Authentication | Replace challenge ID with response value. |
| `POST /api/v1/auth/refresh` | Current refresh token | Session security | Serialize; consumed-token reuse revokes session. |
| `POST /api/v1/auth/password/forgot` | Public generic recovery start | Recovery | Always show generic acknowledgement. |
| `POST /api/v1/auth/password/reset` | Valid recovery evidence | Recovery | Never log code/challenge values. |
| `GET /api/v1/auth/me` | Current `PLATFORM` bearer | Session bootstrap | Reject a non-PLATFORM response in platform UI. |
| `GET /api/v1/auth/sessions` | Current administrative bearer | Self-service security | Shows only own global identity sessions. |
| `DELETE /api/v1/auth/sessions/{session_id}` | Current administrative bearer, owned session | Self-service security | Confirm revocation unless it is explicit logout. |
| `POST /api/v1/auth/logout` | Current administrative bearer | Self-service security | Clear client state after success. |
| `POST /api/v1/auth/logout-all` | Current administrative bearer | Self-service security | Confirm because it ends other own sessions. |
| `POST /api/v1/auth/password/change` | Current administrative bearer/current password | Self-service security | Forces reauthentication after session revocation. |
| `GET /api/v1/platform/events` | `PLATFORM` | Platform observation | Replay-only; dedupe event IDs. |
| `GET /api/v1/platform/realtime/stream` | `PLATFORM` | Platform observation | Use header-capable fetch streaming; events invalidate REST. |
| `GET /api/v1/platform/audit-logs` | `PLATFORM` | Audit observation | Read-only, cursor-based, no client-side reconstruction. |
| `GET /api/v1/platform/workers` | `PLATFORM` | Operations observation | Observational only; no worker-control UI. |
| `POST /api/v1/platform/hal-facts/{fact_id}/requeue` | `PLATFORM` | Critical fact recovery | Require exact fact selection and a deliberate confirmation; never fabricate/replay a different fact. |
| `GET /api/v1/platform/cpos` | `PLATFORM` | CPO observation | Collection/filter/cursor screen. |
| `POST /api/v1/platform/cpos` | `PLATFORM` | CPO provisioning | Validate legal identity; creation remains final authority over advisory availability. |
| `GET /api/v1/platform/cpos/slug-availability` | `PLATFORM` | CPO provisioning preflight | Advisory only; never reserve a slug. |
| `GET /api/v1/platform/cpos/{cpo_id}` | `PLATFORM` | CPO observation | Platform CPO record, not tenant impersonation. |
| `GET /api/v1/platform/cpos/{cpo_id}/operations/fleet` | `PLATFORM` | Platform operational observation | Display projection freshness; no CPO token/header. |
| `GET /api/v1/platform/cpos/{cpo_id}/operations/chargers/{charger_id}` | `PLATFORM` | Platform operational observation | Use only platform operational detail contract. |
| `GET /api/v1/platform/cpos/{cpo_id}/operations/events` | `PLATFORM` | Platform operational observation | Replay and dedupe before stream. |
| `GET /api/v1/platform/cpos/{cpo_id}/operations/realtime/stream` | `PLATFORM` | Platform operational observation | Invalidation-only SSE with platform bearer. |
| `PUT /api/v1/platform/cpos/{cpo_id}/profile` | `PLATFORM` | CPO legal identity | Full replacement snapshot; checksum/state/PIN errors are field-specific. |
| `POST /api/v1/platform/cpos/{cpo_id}/activate` | `PLATFORM` | CPO lifecycle control | Mandatory reason and confirmation. |
| `POST /api/v1/platform/cpos/{cpo_id}/suspend` | `PLATFORM` | CPO lifecycle control | Mandatory reason; explain tenant-session consequences. |
| `PUT /api/v1/platform/cpos/{cpo_id}/app-id` | `PLATFORM` | CPO routing-identity control | Confirm; affected CPO clients must use returned current app ID. |
| `GET /api/v1/platform/cpos/{cpo_id}/primary-admin` | `PLATFORM` | CPO administrator observation | Safe identity/onboarding metadata only. |
| `PUT /api/v1/platform/cpos/{cpo_id}/primary-admin` | `PLATFORM` | CPO administrator recovery | Confirm role/session impact; never display temporary password. |
| `POST /api/v1/platform/cpos/{cpo_id}/primary-admin/resend-onboarding` | `PLATFORM` | CPO administrator recovery | Reasoned, credential-free resend. |
| `POST /api/v1/platform/cpos/{cpo_id}/administrative-sessions/revoke` | `PLATFORM` | CPO administrator recovery | Reasoned administrative-session invalidation; not customer stop. |
| `GET /api/v1/platform/cpos/{cpo_id}/customer-intelligence` | `PLATFORM` | Bounded customer intelligence | Use only documented aggregates/top-customer projections; no tenant impersonation. |
| `GET /api/v1/platform/administrators` | `PLATFORM` | Platform governance observation | Platform-admin list, not CPO staff. |
| `POST /api/v1/platform/administrators` | `PLATFORM` | Platform governance mutation | High impact; invite/grant flow and audit evidence. |
| `POST /api/v1/platform/administrators/{user_id}/activate` | `PLATFORM` | Platform governance mutation | Reason and confirmation. |
| `POST /api/v1/platform/administrators/{user_id}/deactivate` | `PLATFORM` | Platform governance mutation | Reason and confirmation; last active authority is protected. |
| `GET /api/v1/platform/security/locked-identities` | `PLATFORM` | Security observation | Do not expose more than returned safe metadata. |
| `GET /api/v1/platform/security/events` | `PLATFORM` | Security observation | Cursor-based audit/security feed. |
| `POST /api/v1/platform/security/users/{user_id}/unlock` | `PLATFORM` | Security recovery | Mandatory reason and target confirmation. |
| `POST /api/v1/platform/security/users/{user_id}/sessions/revoke` | `PLATFORM` | Security recovery | Confirm scope (`PLATFORM`, `CPO`, or `ALL`) and required CPO target. |
| `GET /api/v1/platform/mail/jobs` | `PLATFORM` | Mail observation | Safe job metadata only; never mail ciphertext/body. |
| `GET /api/v1/platform/mail/jobs/{job_id}` | `PLATFORM` | Mail observation | Safe metadata only. |
| `POST /api/v1/platform/mail/jobs/{job_id}/retry` | `PLATFORM` | Mail recovery | Deliberately retry one selected job. |
| `POST /api/v1/platform/mail/jobs/{job_id}/cancel` | `PLATFORM` | Mail recovery | Mandatory reason and confirmation. |
| `GET /api/v1/platform/mail/metrics` | `PLATFORM` | Mail observation | Bounded aggregate view. |
| `POST /api/v1/platform/mail/reconcile` | `PLATFORM` | Mail maintenance | Mandatory reason; show resulting count/metadata. |
| `POST /api/v1/platform/mail/retention` | `PLATFORM` | Destructive mail maintenance | Confirm cutoff and reason; server enforces retention floor. |
| `GET /api/v1/platform/announcements` | `PLATFORM` | Communications observation | Cursor/reload after publish. |
| `POST /api/v1/platform/announcements` | `PLATFORM` | Communications mutation | Confirm target/audience; recipients are snapshotted durably. |
| `GET /api/v1/platform/notifications` | `PLATFORM` | Notification observation | Recipient derives from platform bearer. |
| `POST /api/v1/platform/notifications/{notification_id}/read` | `PLATFORM` | Notification mutation | Idempotent read-state update. |
| `GET /api/v1/platform/overview` | `PLATFORM` | Platform observation | Bounded platform aggregate, not tenant export. |
| `GET /api/v1/platform/status` | `PLATFORM` | Platform observation | Service/database/worker status only. |
| `GET /api/v1/platform/cpo-assets` | `PLATFORM` | Platform observation | Asset metadata/projection only. |
| `GET /api/v1/platform/support/tickets` | `PLATFORM` | Support observation | Cross-CPO support queue, not general tenant-data access. |
| `GET /api/v1/platform/support/tickets/{ticket_id}` | `PLATFORM` | Support observation | Ticket-specific safe support conversation. |
| `POST /api/v1/platform/support/tickets/{ticket_id}/replies` | `PLATFORM` | Support mutation | Reply is durable; refresh ticket after success. |
| `PATCH /api/v1/platform/support/tickets/{ticket_id}/status` | `PLATFORM` | Support lifecycle mutation | Confirm lifecycle status change. |
| `GET /api/v1/platform/plans` | `PLATFORM` | Subscription observation | Manual platform plan catalog. |
| `POST /api/v1/platform/plans` | `PLATFORM` | Subscription plan mutation | Creates draft plan; no provider billing. |
| `GET /api/v1/platform/plans/{plan_id}` | `PLATFORM` | Subscription observation | Read plan/version state. |
| `PUT /api/v1/platform/plans/{plan_id}/draft` | `PLATFORM` | Subscription plan mutation | Edit draft only; preserve version semantics. |
| `POST /api/v1/platform/plans/{plan_id}/publish` | `PLATFORM` | Subscription plan mutation | High-impact immutable publication; explicit confirmation. |
| `POST /api/v1/platform/plans/{plan_id}/archive` | `PLATFORM` | Subscription plan mutation | Confirm archive; do not delete historical records. |
| `POST /api/v1/platform/cpos/{cpo_id}/subscription` | `PLATFORM` | Subscription lifecycle mutation | Issue manual CPO subscription after confirmed business action. |
| `GET /api/v1/platform/cpos/{cpo_id}/subscription` | `PLATFORM` | Subscription observation | Current commercial record; does not govern CPO admin access. |
| `POST /api/v1/platform/cpos/{cpo_id}/subscription/renew` | `PLATFORM` | Subscription lifecycle mutation | Renewal can reactivate expired subscription after repayment confirmation. |
| `POST /api/v1/platform/cpos/{cpo_id}/subscription/change-plan` | `PLATFORM` | Subscription lifecycle mutation | Confirm plan/version selection. |
| `POST /api/v1/platform/cpos/{cpo_id}/subscription/activate` | `PLATFORM` | Subscription lifecycle mutation | Confirm explicit state transition. |
| `POST /api/v1/platform/cpos/{cpo_id}/subscription/pause` | `PLATFORM` | Subscription lifecycle mutation | Confirm explicit state transition. |
| `POST /api/v1/platform/cpos/{cpo_id}/subscription/resume` | `PLATFORM` | Subscription lifecycle mutation | Confirm explicit state transition. |
| `POST /api/v1/platform/cpos/{cpo_id}/subscription/mark-past-due` | `PLATFORM` | Subscription lifecycle mutation | Confirm explicit state transition. |
| `POST /api/v1/platform/cpos/{cpo_id}/subscription/expire` | `PLATFORM` | Subscription lifecycle mutation | Confirm; expiry blocks only documented new customer commands. |
| `POST /api/v1/platform/cpos/{cpo_id}/subscription/cancel` | `PLATFORM` | Subscription lifecycle mutation | Confirm; retain history. |
| `GET /api/v1/platform/cpos/{cpo_id}/subscription/history` | `PLATFORM` | Subscription observation | Read immutable lifecycle history. |

## Explicitly absent authority

The matrix does not imply access to unrouted/unsupported platform billing,
payment, checkout, webhook, entitlement-override, CPO impersonation, or tenant
integration-secret APIs. A future granular platform RBAC model must add server
enforcement, migration/seed policy, OpenAPI declarations, tests, and a revised
matrix before the frontend can hide/show actions by a new permission key.
