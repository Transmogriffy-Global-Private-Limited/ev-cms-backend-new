-- CPO-local CoW saved service addresses, independent of existing vehicle master data.
-- A saved address is never the source of truth for a historical ServiceJob.
CREATE TABLE cow_customer_addresses (
    id uuid PRIMARY KEY,
    cpo_id uuid NOT NULL,
    customer_id uuid NOT NULL,
    label varchar(80) NOT NULL CHECK (length(btrim(label)) BETWEEN 1 AND 80),
    line1 varchar(255) NOT NULL CHECK (length(btrim(line1)) BETWEEN 1 AND 255),
    line2 varchar(255) NOT NULL DEFAULT '',
    locality varchar(120) NOT NULL DEFAULT '',
    city varchar(120) NOT NULL CHECK (length(btrim(city)) BETWEEN 1 AND 120),
    state varchar(120) NOT NULL CHECK (length(btrim(state)) BETWEEN 1 AND 120),
    postal_code varchar(20) NOT NULL CHECK (length(btrim(postal_code)) BETWEEN 1 AND 20),
    country_code char(2) NOT NULL CHECK (country_code ~ '^[A-Z]{2}$'),
    latitude numeric(10,7),
    longitude numeric(10,7),
    landmark varchar(255) NOT NULL DEFAULT '',
    building_instructions varchar(500) NOT NULL DEFAULT '',
    floor varchar(40) NOT NULL DEFAULT '',
    contact_instructions varchar(500) NOT NULL DEFAULT '',
    geocode_provider varchar(80) NOT NULL DEFAULT '',
    geocode_provider_place_id varchar(255) NOT NULL DEFAULT '',
    is_default boolean NOT NULL DEFAULT false,
    is_active boolean NOT NULL DEFAULT true,
    version bigint NOT NULL DEFAULT 1 CHECK (version >= 1),
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    CONSTRAINT fk_cow_customer_addresses_owner FOREIGN KEY (cpo_id, customer_id)
        REFERENCES customers(cpo_id, id) ON UPDATE CASCADE ON DELETE RESTRICT,
    CONSTRAINT chk_cow_customer_addresses_coordinates CHECK (
      (latitude IS NULL AND longitude IS NULL) OR
      (latitude BETWEEN -90 AND 90 AND longitude BETWEEN -180 AND 180)
    ),
    CONSTRAINT chk_cow_customer_addresses_default_active CHECK (NOT is_default OR is_active)
);
CREATE UNIQUE INDEX uq_cow_customer_addresses_active_default
    ON cow_customer_addresses(cpo_id, customer_id) WHERE is_active AND is_default;
CREATE INDEX ix_cow_customer_addresses_owner_page
    ON cow_customer_addresses(cpo_id, customer_id, created_at DESC, id DESC) WHERE is_active;
