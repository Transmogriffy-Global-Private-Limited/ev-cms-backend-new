-- Strengthen the CPO-scoped relational boundaries of the existing charging,
-- HAL and invoice schema before adding CoW's new Customer/Driver entrypoints.
-- All new constraints are validated; incompatible legacy rows cause failure.
-- No existing record is updated, deleted, reassigned or silently fixed.
-- Run ONLY after 000078, through the repo's versioned migration runner.

-- Referenced identity tuples: PostgreSQL compound FKs require EXACT matching
-- nonpartial unique keys, even when a smaller primary key is already unique.
ALTER TABLE wallets
    ADD CONSTRAINT uq_wallets_cpo_customer_wallet_identity UNIQUE (cpo_id, customer_id, id);
ALTER TABLE charging_start_intents
    ADD CONSTRAINT uq_start_intents_cpo_identity UNIQUE (cpo_id, id),
    ADD CONSTRAINT uq_start_intents_cpo_intent_wallet UNIQUE (cpo_id, id, wallet_id);
ALTER TABLE charging_sessions
    ADD CONSTRAINT uq_charging_sessions_cpo_customer_session UNIQUE (cpo_id, customer_id, id);

-- Each charging intent must belong to one CPO, and its wallet must belong to
-- the SAME CUSTOMER, not merely a different customer within the same CPO.
ALTER TABLE charging_start_intents
    ADD CONSTRAINT fk_start_intents_customer_tenant
        FOREIGN KEY (cpo_id, customer_id) REFERENCES customers(cpo_id, id)
        ON UPDATE RESTRICT ON DELETE RESTRICT NOT VALID,
    ADD CONSTRAINT fk_start_intents_charger_tenant
        FOREIGN KEY (cpo_id, charger_id) REFERENCES chargers(cpo_id, id)
        ON UPDATE RESTRICT ON DELETE RESTRICT NOT VALID,
    ADD CONSTRAINT fk_start_intents_connector_tenant_charger
        FOREIGN KEY (cpo_id, charger_id, connector_id)
        REFERENCES connectors(cpo_id, charger_id, id)
        ON UPDATE RESTRICT ON DELETE RESTRICT NOT VALID,
    ADD CONSTRAINT fk_start_intents_wallet_customer_tenant
        FOREIGN KEY (cpo_id, customer_id, wallet_id)
        REFERENCES wallets(cpo_id, customer_id, id)
        ON UPDATE RESTRICT ON DELETE RESTRICT NOT VALID,
    ADD CONSTRAINT fk_start_intents_tariff_tenant
        FOREIGN KEY (cpo_id, tariff_id) REFERENCES tariffs(cpo_id, id)
        ON UPDATE RESTRICT ON DELETE RESTRICT NOT VALID,
    ADD CONSTRAINT fk_start_intents_materialized_session_tenant
        FOREIGN KEY (cpo_id, materialized_session_id)
        REFERENCES charging_sessions(cpo_id, id)
        ON UPDATE RESTRICT ON DELETE RESTRICT NOT VALID;

-- A wallet hold must reserve the wallet of the exact charging intent.
ALTER TABLE wallet_holds
    ADD CONSTRAINT fk_wallet_holds_intent_wallet_tenant
        FOREIGN KEY (cpo_id, start_intent_id, wallet_id)
        REFERENCES charging_start_intents(cpo_id, id, wallet_id)
        ON UPDATE RESTRICT ON DELETE RESTRICT NOT VALID;

ALTER TABLE hal_command_records
    ADD CONSTRAINT fk_hal_commands_start_intent_tenant
        FOREIGN KEY (cpo_id, start_intent_id)
        REFERENCES charging_start_intents(cpo_id, id)
        ON UPDATE RESTRICT ON DELETE RESTRICT NOT VALID,
    ADD CONSTRAINT fk_hal_commands_session_tenant
        FOREIGN KEY (cpo_id, charging_session_id)
        REFERENCES charging_sessions(cpo_id, id)
        ON UPDATE RESTRICT ON DELETE RESTRICT NOT VALID;

ALTER TABLE hal_charger_mappings
    ADD CONSTRAINT fk_hal_mappings_charger_tenant
        FOREIGN KEY (cpo_id, cms_charger_id)
        REFERENCES chargers(cpo_id, id)
        ON UPDATE RESTRICT ON DELETE RESTRICT NOT VALID;
ALTER TABLE hal_charger_runtime
    ADD CONSTRAINT fk_hal_runtime_charger_tenant
        FOREIGN KEY (cpo_id, cms_charger_id)
        REFERENCES chargers(cpo_id, id)
        ON UPDATE RESTRICT ON DELETE RESTRICT NOT VALID;
ALTER TABLE hal_connector_runtime
    ADD CONSTRAINT fk_hal_runtime_connector_tenant_charger
        FOREIGN KEY (cpo_id, cms_charger_id, cms_connector_id)
        REFERENCES connectors(cpo_id, charger_id, id)
        ON UPDATE RESTRICT ON DELETE RESTRICT NOT VALID;

-- Recorded charging sessions and management operations must not refer to
-- an intent/asset operated by a different CPO.
ALTER TABLE charging_sessions
    ADD CONSTRAINT fk_charging_sessions_start_intent_tenant
        FOREIGN KEY (cpo_id, start_intent_id)
        REFERENCES charging_start_intents(cpo_id, id)
        ON UPDATE RESTRICT ON DELETE RESTRICT NOT VALID;
ALTER TABLE charger_operations
    ADD CONSTRAINT fk_charger_operations_charger_tenant
        FOREIGN KEY (cpo_id, charger_id)
        REFERENCES chargers(cpo_id, id)
        ON UPDATE RESTRICT ON DELETE RESTRICT NOT VALID,
    ADD CONSTRAINT fk_charger_operations_connector_tenant_charger
        FOREIGN KEY (cpo_id, charger_id, connector_id)
        REFERENCES connectors(cpo_id, charger_id, id)
        ON UPDATE RESTRICT ON DELETE RESTRICT NOT VALID;

-- Invoice's Customer must be the actual Customer of the billed Session.
ALTER TABLE charging_session_invoices
    ADD CONSTRAINT fk_session_invoice_exact_session_customer_tenant
        FOREIGN KEY (cpo_id, customer_id, session_id)
        REFERENCES charging_sessions(cpo_id, customer_id, id)
        ON UPDATE RESTRICT ON DELETE RESTRICT NOT VALID;

-- No historical data is silently grandfathered under NOT VALID. Fully verify
-- existing rows; the migration transaction rolls back if any check fails.
ALTER TABLE charging_start_intents
    VALIDATE CONSTRAINT fk_start_intents_customer_tenant,
    VALIDATE CONSTRAINT fk_start_intents_charger_tenant,
    VALIDATE CONSTRAINT fk_start_intents_connector_tenant_charger,
    VALIDATE CONSTRAINT fk_start_intents_wallet_customer_tenant,
    VALIDATE CONSTRAINT fk_start_intents_tariff_tenant,
    VALIDATE CONSTRAINT fk_start_intents_materialized_session_tenant;
ALTER TABLE wallet_holds VALIDATE CONSTRAINT fk_wallet_holds_intent_wallet_tenant;
ALTER TABLE hal_command_records
    VALIDATE CONSTRAINT fk_hal_commands_start_intent_tenant,
    VALIDATE CONSTRAINT fk_hal_commands_session_tenant;
ALTER TABLE hal_charger_mappings VALIDATE CONSTRAINT fk_hal_mappings_charger_tenant;
ALTER TABLE hal_charger_runtime VALIDATE CONSTRAINT fk_hal_runtime_charger_tenant;
ALTER TABLE hal_connector_runtime VALIDATE CONSTRAINT fk_hal_runtime_connector_tenant_charger;
ALTER TABLE charging_sessions VALIDATE CONSTRAINT fk_charging_sessions_start_intent_tenant;
ALTER TABLE charger_operations
    VALIDATE CONSTRAINT fk_charger_operations_charger_tenant,
    VALIDATE CONSTRAINT fk_charger_operations_connector_tenant_charger;
ALTER TABLE charging_session_invoices VALIDATE CONSTRAINT fk_session_invoice_exact_session_customer_tenant;
