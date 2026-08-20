-- A current challenge is unconsumed and uninvalidated. Expiry is deliberately
-- not part of these predicates because PostgreSQL partial-index predicates
-- cannot depend on the current time. Application replacement transitions
-- expired records explicitly before issuing a successor.
DO $$
DECLARE
    duplicate_key record;
BEGIN
    SELECT user_id, purpose, count(*) AS duplicate_count
      INTO duplicate_key
      FROM auth_challenges
     WHERE consumed_at IS NULL AND invalidated_at IS NULL
     GROUP BY user_id, purpose
    HAVING count(*) > 1
     LIMIT 1;
    IF FOUND THEN
        RAISE EXCEPTION '000046 refuses duplicate current auth challenges for user_id %, purpose %, count %',
            duplicate_key.user_id, duplicate_key.purpose, duplicate_key.duplicate_count;
    END IF;

    SELECT cpo_id, customer_id, purpose, count(*) AS duplicate_count
      INTO duplicate_key
      FROM customer_auth_challenges
     WHERE consumed_at IS NULL AND invalidated_at IS NULL
     GROUP BY cpo_id, customer_id, purpose
    HAVING count(*) > 1
     LIMIT 1;
    IF FOUND THEN
        RAISE EXCEPTION '000046 refuses duplicate current customer challenges for cpo_id %, customer_id %, purpose %, count %',
            duplicate_key.cpo_id, duplicate_key.customer_id, duplicate_key.purpose, duplicate_key.duplicate_count;
    END IF;

    SELECT cpo_id, lower(btrim(email)) AS normalized_email, count(*) AS duplicate_count
      INTO duplicate_key
      FROM customer_signup_challenges
     WHERE consumed_at IS NULL AND invalidated_at IS NULL
     GROUP BY cpo_id, lower(btrim(email))
    HAVING count(*) > 1
     LIMIT 1;
    IF FOUND THEN
        RAISE EXCEPTION '000046 refuses duplicate current customer signup challenges for cpo_id %, email %, count %',
            duplicate_key.cpo_id, duplicate_key.normalized_email, duplicate_key.duplicate_count;
    END IF;
END $$;

CREATE UNIQUE INDEX uq_auth_challenges_current_identity_purpose
    ON auth_challenges (user_id, purpose)
    WHERE consumed_at IS NULL AND invalidated_at IS NULL;

CREATE UNIQUE INDEX uq_customer_auth_challenges_current_identity_purpose
    ON customer_auth_challenges (cpo_id, customer_id, purpose)
    WHERE consumed_at IS NULL AND invalidated_at IS NULL;

CREATE UNIQUE INDEX uq_customer_signup_challenges_current_identity
    ON customer_signup_challenges (cpo_id, lower(btrim(email)))
    WHERE consumed_at IS NULL AND invalidated_at IS NULL;

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM gsts WHERE state IS NULL) THEN
        RAISE EXCEPTION '000046 cannot require gsts.state while NULL GST state rows exist; repair their business state explicitly first';
    END IF;
END $$;

ALTER TABLE gsts ALTER COLUMN state SET NOT NULL;
