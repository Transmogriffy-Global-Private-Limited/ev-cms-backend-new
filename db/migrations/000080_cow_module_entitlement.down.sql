-- Do not implicitly discard live module grants or their audit history.
-- A reversible development-only cleanup must be reviewed separately.
DO $$
BEGIN
    RAISE EXCEPTION '000080 cannot be rolled back automatically: CoW entitlement history is durable'
        USING ERRCODE = 'P0001';
END;
$$;
