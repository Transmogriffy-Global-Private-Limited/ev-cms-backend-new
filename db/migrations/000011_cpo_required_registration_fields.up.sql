DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM cpos
        WHERE gstin IS NULL
           OR btrim(address) = ''
           OR btrim(city) = ''
           OR btrim(state) = ''
           OR btrim(pincode) = ''
    ) THEN
        RAISE EXCEPTION
            'cannot require CPO GSTIN and address fields while incomplete CPO records exist';
    END IF;
END
$$;

ALTER TABLE cpos
    DROP CONSTRAINT chk_cpos_gstin;

ALTER TABLE cpos
    ALTER COLUMN gstin SET NOT NULL,
    ALTER COLUMN address DROP DEFAULT,
    ALTER COLUMN city DROP DEFAULT,
    ALTER COLUMN state DROP DEFAULT,
    ALTER COLUMN pincode DROP DEFAULT;

ALTER TABLE cpos
    ADD CONSTRAINT chk_cpos_gstin CHECK (gstin ~ '^[0-9A-Z]{15}$'),
    ADD CONSTRAINT chk_cpos_address_not_blank CHECK (btrim(address) <> ''),
    ADD CONSTRAINT chk_cpos_city_not_blank CHECK (btrim(city) <> ''),
    ADD CONSTRAINT chk_cpos_state_not_blank CHECK (btrim(state) <> ''),
    ADD CONSTRAINT chk_cpos_pincode_not_blank CHECK (btrim(pincode) <> '');
