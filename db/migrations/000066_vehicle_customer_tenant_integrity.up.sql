ALTER TABLE vehicles
    DROP CONSTRAINT IF EXISTS fk_vehicles_customer;

ALTER TABLE vehicles
    ADD CONSTRAINT fk_vehicles_customer_tenant
    FOREIGN KEY (cpo_id, customer_id)
    REFERENCES customers(cpo_id, id)
    ON UPDATE CASCADE
    ON DELETE RESTRICT;
