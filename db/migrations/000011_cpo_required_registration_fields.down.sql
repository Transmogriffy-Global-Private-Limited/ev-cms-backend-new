ALTER TABLE cpos
    DROP CONSTRAINT IF EXISTS chk_cpos_pincode_not_blank,
    DROP CONSTRAINT IF EXISTS chk_cpos_state_not_blank,
    DROP CONSTRAINT IF EXISTS chk_cpos_city_not_blank,
    DROP CONSTRAINT IF EXISTS chk_cpos_address_not_blank,
    DROP CONSTRAINT IF EXISTS chk_cpos_gstin;

ALTER TABLE cpos
    ALTER COLUMN gstin DROP NOT NULL,
    ALTER COLUMN address SET DEFAULT '',
    ALTER COLUMN city SET DEFAULT '',
    ALTER COLUMN state SET DEFAULT '',
    ALTER COLUMN pincode SET DEFAULT '';

ALTER TABLE cpos
    ADD CONSTRAINT chk_cpos_gstin CHECK (
        gstin IS NULL OR gstin ~ '^[0-9A-Z]{15}$'
    );
