CREATE TABLE
    deployment_events (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid (),
        deployment_id UUID NOT NULL,
        version INT NOT NULL CHECK (version > 0),
        type TEXT NOT NULL,
        schema_version INTEGER NOT NULL DEFAULT 1 CHECK (schema_version > 0),
        payload JSONB NOT NULL CHECK (jsonb_typeof(payload) = 'object'),
        created_at TIMESTAMPTZ NOT NULL DEFAULT NOW (),

        UNIQUE (deployment_id, version)
    );