CREATE TYPE tariff_assignment_type AS ENUM ('usergroup', 'hub', 'charger');

ALTER TABLE tariffs
ADD COLUMN assigned_to tariff_assignment_type;
