CREATE OR REPLACE FUNCTION public.is_valid_indian_gstin(candidate text)
RETURNS boolean
LANGUAGE sql
IMMUTABLE
STRICT
PARALLEL SAFE
AS $$
    WITH digits AS (
        SELECT position, strpos('0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ', substr(candidate, position, 1)) - 1 AS digit
        FROM generate_series(1, 14) AS positions(position)
    ), weighted AS (
        SELECT CASE WHEN position % 2 = 1 THEN digit ELSE digit * 2 END AS value FROM digits
    ), checksum AS (
        SELECT COALESCE(sum(value / 36 + value % 36), 0) AS value FROM weighted
    )
    SELECT candidate ~ '^(0[1-9]|[12][0-9]|3[0-8])[A-Z]{5}[0-9]{4}[A-Z][1-9A-Z]Z[0-9A-Z]$'
       AND substr(candidate, 15, 1) = substr('0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ', ((36 - checksum.value % 36) % 36) + 1, 1)
    FROM checksum;
$$;

CREATE OR REPLACE FUNCTION public.gstin_matches_indian_state(candidate text, registration_state text)
RETURNS boolean
LANGUAGE sql
IMMUTABLE
STRICT
PARALLEL SAFE
AS $$
    SELECT CASE substr(candidate, 1, 2)
        WHEN '01' THEN registration_state = 'Jammu and Kashmir' WHEN '02' THEN registration_state = 'Himachal Pradesh'
        WHEN '03' THEN registration_state = 'Punjab' WHEN '04' THEN registration_state = 'Chandigarh'
        WHEN '05' THEN registration_state = 'Uttarakhand' WHEN '06' THEN registration_state = 'Haryana'
        WHEN '07' THEN registration_state = 'Delhi (National Capital Territory of Delhi)' WHEN '08' THEN registration_state = 'Rajasthan'
        WHEN '09' THEN registration_state = 'Uttar Pradesh' WHEN '10' THEN registration_state = 'Bihar'
        WHEN '11' THEN registration_state = 'Sikkim' WHEN '12' THEN registration_state = 'Arunachal Pradesh'
        WHEN '13' THEN registration_state = 'Nagaland' WHEN '14' THEN registration_state = 'Manipur'
        WHEN '15' THEN registration_state = 'Mizoram' WHEN '16' THEN registration_state = 'Tripura'
        WHEN '17' THEN registration_state = 'Meghalaya' WHEN '18' THEN registration_state = 'Assam'
        WHEN '19' THEN registration_state = 'West Bengal' WHEN '20' THEN registration_state = 'Jharkhand'
        WHEN '21' THEN registration_state = 'Odisha' WHEN '22' THEN registration_state = 'Chhattisgarh'
        WHEN '23' THEN registration_state = 'Madhya Pradesh' WHEN '24' THEN registration_state = 'Gujarat'
        WHEN '25' THEN registration_state = 'Dadra and Nagar Haveli and Daman and Diu'
        WHEN '26' THEN registration_state = 'Dadra and Nagar Haveli and Daman and Diu'
        WHEN '27' THEN registration_state = 'Maharashtra' WHEN '28' THEN registration_state = 'Andhra Pradesh'
        WHEN '29' THEN registration_state = 'Karnataka' WHEN '30' THEN registration_state = 'Goa'
        WHEN '31' THEN registration_state = 'Lakshadweep' WHEN '32' THEN registration_state = 'Kerala'
        WHEN '33' THEN registration_state = 'Tamil Nadu' WHEN '34' THEN registration_state = 'Puducherry'
        WHEN '35' THEN registration_state = 'Andaman and Nicobar Islands' WHEN '36' THEN registration_state = 'Telangana'
        WHEN '37' THEN registration_state = 'Andhra Pradesh' WHEN '38' THEN registration_state = 'Ladakh'
        ELSE false
    END;
$$;

DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM cpos
        WHERE NOT public.is_valid_indian_gstin(gstin)
           OR NOT public.gstin_matches_indian_state(gstin, state)
           OR pincode !~ '^[1-9][0-9]{5}$'
    ) THEN
        RAISE EXCEPTION 'cannot enforce CPO legal identity validation while invalid GSTIN, state, or PIN records exist';
    END IF;
END
$$;

ALTER TABLE cpos DROP CONSTRAINT chk_cpos_gstin;
ALTER TABLE cpos
    ADD CONSTRAINT chk_cpos_gstin CHECK (public.is_valid_indian_gstin(gstin)),
    ADD CONSTRAINT chk_cpos_gstin_state_matches CHECK (public.gstin_matches_indian_state(gstin, state)),
    ADD CONSTRAINT chk_cpos_pincode_format CHECK (pincode ~ '^[1-9][0-9]{5}$');
