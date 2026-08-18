ALTER TABLE tariffs
    RENAME COLUMN price_per_kwh TO price_per_unit;

ALTER TABLE tariffs
    RENAME CONSTRAINT chk_tariffs_price TO chk_tariffs_price_per_unit;
