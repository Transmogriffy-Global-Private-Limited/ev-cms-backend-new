DROP INDEX IF EXISTS uq_customer_signup_challenges_current_identity;
DROP INDEX IF EXISTS uq_customer_auth_challenges_current_identity_purpose;
DROP INDEX IF EXISTS uq_auth_challenges_current_identity_purpose;

ALTER TABLE gsts ALTER COLUMN state DROP NOT NULL;
