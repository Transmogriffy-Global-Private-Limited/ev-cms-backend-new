ALTER TABLE vehicles
    DROP CONSTRAINT IF EXISTS fk_vehicles_customer_tenant;

ALTER TABLE vehicles
    ADD CONSTRAINT fk_vehicles_customer
    FOREIGN KEY (customer_id)
    REFERENCES customers(id)
    ON UPDATE CASCADE
    ON DELETE RESTRICT;
