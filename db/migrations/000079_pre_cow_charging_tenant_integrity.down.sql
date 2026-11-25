-- Rolling this back would silently weaken tenant and wallet ownership.
-- Repairs should be additive/forward-only, not a broad constraint removal.
DO $$
BEGIN
    RAISE EXCEPTION '000079 must not be rolled back automatically; use a reviewed forward recovery migration'
        USING ERRCODE = 'P0001';
END;
$$;
