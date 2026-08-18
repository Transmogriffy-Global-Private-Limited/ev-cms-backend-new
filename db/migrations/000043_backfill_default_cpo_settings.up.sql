-- Existing CPOs must have the same blank, zero-default settings row created
-- transactionally for every new CPO by the application.
INSERT INTO settings (
    id,
    cpo_id,
    wallet_min_balance,
    wallet_buffer_min_balance,
    created_at,
    updated_at
)
SELECT
    gen_random_uuid(),
    cpos.id,
    0,
    0,
    NOW(),
    NOW()
FROM cpos
LEFT JOIN settings ON settings.cpo_id = cpos.id
WHERE settings.cpo_id IS NULL;
