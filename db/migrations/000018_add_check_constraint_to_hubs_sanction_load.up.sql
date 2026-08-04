ALTER TABLE hubs ADD CONSTRAINT hubs_sanction_load_check CHECK (sanction_load >= 0);
