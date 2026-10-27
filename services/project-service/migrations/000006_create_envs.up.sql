CREATE TABLE
    application_env_vars (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid (),
        application_id UUID NOT NULL,
        key VARCHAR(255) NOT NULL,
        value TEXT NOT NULL,
        created_at TIMESTAMPTZ NOT NULL DEFAULT NOW (),
        updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW (),

        CONSTRAINT fk_env_application
            FOREIGN KEY (application_id)
            REFERENCES applications (id)
            ON DELETE CASCADE,

        CONSTRAINT uq_env_application_key
            UNIQUE (application_id, key),

        CONSTRAINT chk_env_key
            CHECK (LENGTH(TRIM(key)) > 0)
    );