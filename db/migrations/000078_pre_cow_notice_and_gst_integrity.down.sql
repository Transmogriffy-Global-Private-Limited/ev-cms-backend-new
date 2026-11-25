-- An automatic rollback would reintroduce the original duplicate-recipient
-- and tax-profile deletion defects. No historical data is disposable.
DO $$
BEGIN
    RAISE EXCEPTION '000078 must not be rolled back automatically; use a reviewed forward recovery migration'
        USING ERRCODE = 'P0001';
END;
$$;
