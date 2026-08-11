
CREATE TYPE tariff_type AS ENUM ('fixed');
CREATE TYPE price_type AS ENUM ('sessions', 'time', 'energy');
CREATE TYPE units AS ENUM ('minutes', 'watt/hour');

ALTER TABLE tariffs
ADD COLUMN tariff_type tariff_type,
ADD COLUMN price_type price_type,
ADD COLUMN units units;
