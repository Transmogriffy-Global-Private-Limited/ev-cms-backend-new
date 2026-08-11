
ALTER TABLE tariffs
DROP COLUMN tariff_type,
DROP COLUMN price_type,
DROP COLUMN units;

DROP TYPE tariff_type;
DROP TYPE price_type;
DROP TYPE units;
