-- Non-empty serial numbers identify a physical charger within its CPO. Empty
-- values are excluded because legacy provisioning may use the default value.
CREATE UNIQUE INDEX uq_chargers_cpo_serial_number
    ON chargers (cpo_id, serial_number)
    WHERE serial_number <> '';

CREATE TABLE customer_ratings (
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
    updated_at      timestamptz   NOT NULL DEFAULT now(),

    CONSTRAINT fk_customer_ratings_cpo
        FOREIGN KEY (cpo_id) REFERENCES cpos(id) ON UPDATE CASCADE ON DELETE RESTRICT,
    CONSTRAINT fk_customer_ratings_customer
        FOREIGN KEY (cpo_id, customer_id) REFERENCES customers(cpo_id, id)
        ON UPDATE CASCADE ON DELETE RESTRICT,
    CONSTRAINT fk_customer_ratings_charger
        FOREIGN KEY (cpo_id, charger_id) REFERENCES chargers(cpo_id, id)
        ON UPDATE CASCADE ON DELETE RESTRICT,
    CONSTRAINT fk_customer_ratings_hub
        FOREIGN KEY (cpo_id, hub_id) REFERENCES hubs(cpo_id, id)
        ON UPDATE CASCADE ON DELETE RESTRICT,
    CONSTRAINT fk_customer_ratings_session
        FOREIGN KEY (cpo_id, session_id) REFERENCES charging_sessions(cpo_id, id)
        ON UPDATE CASCADE ON DELETE RESTRICT,
    CONSTRAINT chk_customer_ratings_overall CHECK (overall_rating BETWEEN 1 AND 5),
    CONSTRAINT chk_customer_ratings_station
        CHECK (station_rating IS NULL OR station_rating BETWEEN 1 AND 5),
    CONSTRAINT chk_customer_ratings_charger
        CHECK (charger_rating IS NULL OR charger_rating BETWEEN 1 AND 5)
);

CREATE INDEX idx_customer_ratings_cpo_id ON customer_ratings (cpo_id);
CREATE INDEX idx_customer_ratings_customer ON customer_ratings (cpo_id, customer_id);
CREATE INDEX idx_customer_ratings_charger ON customer_ratings (cpo_id, charger_id);
CREATE INDEX idx_customer_ratings_hub ON customer_ratings (cpo_id, hub_id) WHERE hub_id IS NOT NULL;
CREATE INDEX idx_customer_ratings_session ON customer_ratings (cpo_id, session_id) WHERE session_id IS NOT NULL;

-- A session accepts at most one rating from a given customer within a CPO.
CREATE UNIQUE INDEX uq_customer_ratings_session_customer
    ON customer_ratings (cpo_id, session_id, customer_id)
    WHERE session_id IS NOT NULL;
