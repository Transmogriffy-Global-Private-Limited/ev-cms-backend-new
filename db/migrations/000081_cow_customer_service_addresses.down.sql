-- No automatic destructive rollback of customer address data.
DO $$ BEGIN
    RAISE EXCEPTION '000081 cannot be rolled back automatically: customer saved addresses are durable'
        USING ERRCODE = 'P0001';
END $$;
