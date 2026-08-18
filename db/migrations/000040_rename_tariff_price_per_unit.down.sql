ALTER TABLE tariffs
    RENAME CONSTRAINT chk_tariffs_price_per_unit TO chk_tariffs_price;

ALTER TABLE tariffs
    RENAME COLUMN price_per_unit TO price_per_kwh;
