ALTER TABLE cpos
    DROP CONSTRAINT IF EXISTS chk_cpos_pincode_format,
    DROP CONSTRAINT IF EXISTS chk_cpos_gstin_state_matches,
    DROP CONSTRAINT IF EXISTS chk_cpos_gstin;

ALTER TABLE cpos ADD CONSTRAINT chk_cpos_gstin CHECK (gstin ~ '^[0-9A-Z]{15}$');

DROP FUNCTION IF EXISTS public.gstin_matches_indian_state(text, text);
DROP FUNCTION IF EXISTS public.is_valid_indian_gstin(text);
