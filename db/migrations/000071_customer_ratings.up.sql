

BEGIN;

CREATE TABLE IF NOT EXISTS customer_ratings (
    id              uuid          PRIMARY KEY DEFAULT gen_random_uuid(),
    cpo_id          uuid          NOT NULL,
    customer_id     uuid          NOT NULL,
    charger_id      uuid          NOT NULL,
    hub_id          uuid          NULL,
    session_id      uuid          NULL,

    overall_rating  smallint      NOT NULL,
    station_rating  smallint      NULL,
    charger_rating  smallint      NULL,
    reason          varchar(1000) NULL,

    created_at      timestamptz   NOT NULL DEFAULT now(),
    updated_at      timestamptz   NOT NULL DEFAULT now()
);

-- Foreign keys (matching existing OnUpdate:CASCADE / OnDelete:RESTRICT convention).
ALTER TABLE customer_ratings
    ADD CONSTRAINT fk_customer_ratings_cpo
        FOREIGN KEY (cpo_id)      REFERENCES cpos(id)              ON UPDATE CASCADE ON DELETE RESTRICT,
    ADD CONSTRAINT fk_customer_ratings_customer
        FOREIGN KEY (customer_id) REFERENCES customers(id)         ON UPDATE CASCADE ON DELETE RESTRICT,
    ADD CONSTRAINT fk_customer_ratings_charger
        FOREIGN KEY (charger_id)  REFERENCES chargers(id)          ON UPDATE CASCADE ON DELETE RESTRICT,
    ADD CONSTRAINT fk_customer_ratings_hub
        FOREIGN KEY (hub_id)      REFERENCES hubs(id)              ON UPDATE CASCADE ON DELETE RESTRICT,
    ADD CONSTRAINT fk_customer_ratings_session
        FOREIGN KEY (session_id)  REFERENCES charging_sessions(id) ON UPDATE CASCADE ON DELETE RESTRICT;

-- Rating range enforcement (1..5). NULL is allowed for optional fields.
ALTER TABLE customer_ratings
    ADD CONSTRAINT chk_customer_ratings_overall
        CHECK (overall_rating BETWEEN 1 AND 5),
    ADD CONSTRAINT chk_customer_ratings_station
        CHECK (station_rating IS NULL OR station_rating BETWEEN 1 AND 5),
    ADD CONSTRAINT chk_customer_ratings_charger
        CHECK (charger_rating IS NULL OR charger_rating BETWEEN 1 AND 5);

-- Query-path indexes (CPO scoping is the leading column for all of them).
CREATE INDEX IF NOT EXISTS idx_customer_ratings_cpo_id
    ON customer_ratings (cpo_id);

CREATE INDEX IF NOT EXISTS idx_customer_ratings_customer
    ON customer_ratings (cpo_id, customer_id);

CREATE INDEX IF NOT EXISTS idx_customer_ratings_charger
    ON customer_ratings (cpo_id, charger_id);

CREATE INDEX IF NOT EXISTS idx_customer_ratings_hub
    ON customer_ratings (cpo_id, hub_id)
    WHERE hub_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_customer_ratings_session
    ON customer_ratings (cpo_id, session_id)
    WHERE session_id IS NOT NULL;

-- One rating per charging session, per customer. Drop this index if a
-- session is intentionally allowed to accept multiple ratings.
CREATE UNIQUE INDEX IF NOT EXISTS uq_customer_ratings_session_customer
    ON customer_ratings (cpo_id, session_id, customer_id)
    WHERE session_id IS NOT NULL;

COMMIT;