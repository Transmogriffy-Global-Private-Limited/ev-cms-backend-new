
BEGIN;

DROP INDEX IF EXISTS uq_customer_ratings_session_customer;
DROP INDEX IF EXISTS idx_customer_ratings_session;
DROP INDEX IF EXISTS idx_customer_ratings_hub;
DROP INDEX IF EXISTS idx_customer_ratings_charger;
DROP INDEX IF EXISTS idx_customer_ratings_customer;
DROP INDEX IF EXISTS idx_customer_ratings_cpo_id;

DROP TABLE IF EXISTS customer_ratings;

COMMIT;