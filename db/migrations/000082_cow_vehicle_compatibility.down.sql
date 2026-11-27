-- Prevent silent loss of customer-declared connector/charging metadata or versions.
DO $$ BEGIN
    RAISE EXCEPTION '000082 cannot be rolled back automatically: vehicle compatibility declarations and versions are durable'
        USING ERRCODE = 'P0001';
END $$;
