-- Create vehicles table
CREATE TABLE IF NOT EXISTS vehicles (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    cpo_id UUID NOT NULL,
    customer_id UUID NOT NULL,
    vehicle_number VARCHAR(50) NOT NULL,
    type VARCHAR(50),
    make VARCHAR(100),
    model VARCHAR(100),
    last_charged TIMESTAMPTZ,
    date_added TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,

    -- Foreign keys
    CONSTRAINT fk_vehicles_cpo FOREIGN KEY (cpo_id) REFERENCES cpos(id) ON UPDATE CASCADE ON DELETE RESTRICT,
    CONSTRAINT fk_vehicles_customer FOREIGN KEY (customer_id) REFERENCES customers(id) ON UPDATE CASCADE ON DELETE RESTRICT
);

-- Indexes for performance
CREATE INDEX idx_vehicles_cpo_id ON vehicles(cpo_id);
CREATE INDEX idx_vehicles_customer_id ON vehicles(customer_id);
CREATE INDEX idx_vehicles_vehicle_number ON vehicles(vehicle_number);