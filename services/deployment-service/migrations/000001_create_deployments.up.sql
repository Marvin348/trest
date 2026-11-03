CREATE TABLE
    deployments (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        request_id UUID NOT NULL UNIQUE,
        project_id UUID NOT NULL,
        application_id UUID NOT NULL,

        image TEXT NOT NULL,
        replicas INT NOT NULL DEFAULT 1 CHECK (replicas > 0),
        container_port INT NOT NULL CHECK (container_port BETWEEN 1 AND 65535),

        cpu_request_m INT NOT NULL CHECK (cpu_request_m > 0),
        cpu_limit_m INT NOT NULL CHECK (cpu_limit_m >= cpu_request_m),
        memory_request_mi INT NOT NULL CHECK (memory_request_mi > 0),
        memory_limit_mi INT NOT NULL CHECK (memory_limit_mi >= memory_request_mi),

        environment JSONB NOT NULL,

        status TEXT NOT NULL DEFAULT 'QUEUED'
            CHECK (status IN ('QUEUED', 'RUNNING', 'SUCCEEDED', 'FAILED')),

        created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
        updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

        CONSTRAINT chk_deployment_image_not_empty
            CHECK (LENGTH(TRIM(image)) > 0)
    );

CREATE INDEX idx_deployments_application_created
    ON deployments(application_id, created_at DESC);