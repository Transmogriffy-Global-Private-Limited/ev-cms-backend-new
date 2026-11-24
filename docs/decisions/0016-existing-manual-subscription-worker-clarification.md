# ADR 0016: Existing Manual Subscription Lifecycle Worker Clarification

- Date: 2026-11-24
- Status: Current documentation clarification (not a new billing or subscription policy)
- Supersedes: **only** the historical statements in ADR 0012 that `subscription-lifecycle` was disabled; ADR 0012 remains historical and otherwise preserved

## Source-verified present behavior

- SuperAdmin creates and publishes plan versions, then manually issues and renews CPO subscriptions. The billing period still derives from `billing_interval × interval_count`; no arbitrary end date is accepted.
- Subscription collection takes place outside the platform. There is **no** platform-subscription checkout/payment provider/webhook/automatic debit. The current issue/renew API does **not** expose structured bank-payment evidence or verified collection status; reason, actor, history and platform audit are not a substitute for proof of payment.
- `main.go` **does** start `subscriptions.RunLifecycle`. The worker records warning threshold events, queues mail intents when outbox is configured, and marks a current period `EXPIRED` after its end. It does not auto-renew, auto-collect payments or activate/suspend a CPO.
- CPO suspension is independent and blocks normal CPO/Customer operations; safe Stop continuation is a separate correctness repair, not a new billing policy.
- These are **source observations**, not proof that the current commit is live deployed or that SMTP delivery/physical HAL completion is verified.

## Outstanding correctness work, not silently resolved

- Subsequent-period expiry deduplication, reminder scheduling fairness, original-response idempotency, terminal subscription overview, and the cancellation/no-history admission distinction require their separately scoped repairs and tests.
- Customer Stop during CPO suspension requires secure continuation credentials and HAL acceptance testing.
- CoW extension grant, finance approvals and custom roles remain independent future work.

## Compatibility

Preserve established plan-derived dates, existing subscription history, platform authority boundaries and CPO administrative lifecycle independence. Update human/OpenAPI contracts only for actually implemented changes; never claim structured external collection evidence has been stored when it has not.
