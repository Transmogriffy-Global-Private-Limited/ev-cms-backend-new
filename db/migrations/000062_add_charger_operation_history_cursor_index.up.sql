-- CPO operation-history pagination is rooted in tenant-owned durable CMS state.
CREATE INDEX ix_charger_operations_cpo_created_id
    ON charger_operations (cpo_id, created_at DESC, id DESC);
