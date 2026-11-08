CREATE TABLE
    processed_requests (
        operation TEXT NOT NULL,
        request_id UUID NOT NULL,
        deployment_id UUID NOT NULL,
        fingerprint BYTEA NOT NULL,
        created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

        PRIMARY KEY (operation, request_id)
    );