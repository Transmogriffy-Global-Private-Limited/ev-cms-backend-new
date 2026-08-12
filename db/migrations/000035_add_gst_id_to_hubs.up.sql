ALTER TABLE hubs ADD COLUMN gst_id UUID;
ALTER TABLE hubs ADD CONSTRAINT fk_hubs_gst FOREIGN KEY (cpo_id, gst_id) REFERENCES gsts(cpo_id, id) ON DELETE SET NULL;
