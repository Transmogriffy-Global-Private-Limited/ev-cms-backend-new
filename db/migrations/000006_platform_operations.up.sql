CREATE TABLE platform_events (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    event_type varchar(150) NOT NULL,
    actor_user_id uuid
        REFERENCES users(id) ON UPDATE CASCADE ON DELETE SET NULL,
    resource_type varchar(100) NOT NULL,
    resource_id varchar(255),
    data jsonb NOT NULL DEFAULT '{}'::jsonb,
    occurred_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL,
    CONSTRAINT chk_platform_events_type_not_blank
        CHECK (btrim(event_type) <> ''),
    CONSTRAINT chk_platform_events_resource_not_blank
        CHECK (btrim(resource_type) <> ''),
    CONSTRAINT chk_platform_events_data_object
        CHECK (jsonb_typeof(data) = 'object'),
    CONSTRAINT chk_platform_events_expiry
        CHECK (expires_at > occurred_at)
);

CREATE INDEX ix_platform_events_occurred
    ON platform_events (occurred_at DESC, id DESC);
CREATE INDEX ix_platform_events_type_id
    ON platform_events (event_type, id);
CREATE INDEX ix_platform_events_expiry
    ON platform_events (expires_at);

CREATE TABLE worker_instances (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    worker_name varchar(100) NOT NULL,
    instance_key varchar(255) NOT NULL,
    required boolean NOT NULL DEFAULT true,
    reported_status varchar(20) NOT NULL DEFAULT 'HEALTHY',
    started_at timestamptz NOT NULL DEFAULT now(),
    last_heartbeat_at timestamptz NOT NULL DEFAULT now(),
    last_job_completed_at timestamptz,
    metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT uq_worker_instances_identity
        UNIQUE (worker_name, instance_key),
    CONSTRAINT chk_worker_instances_name_not_blank
        CHECK (btrim(worker_name) <> ''),
    CONSTRAINT chk_worker_instances_key_not_blank
        CHECK (btrim(instance_key) <> ''),
    CONSTRAINT chk_worker_instances_status
        CHECK (reported_status IN ('HEALTHY', 'DEGRADED', 'DISABLED')),
    CONSTRAINT chk_worker_instances_metadata_object
        CHECK (jsonb_typeof(metadata) = 'object')
);

CREATE INDEX ix_worker_instances_name_heartbeat
    ON worker_instances (worker_name, last_heartbeat_at DESC);
CREATE INDEX ix_worker_instances_required_heartbeat
    ON worker_instances (required, last_heartbeat_at);
