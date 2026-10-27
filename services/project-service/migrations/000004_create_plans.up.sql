CREATE TABLE
    plans (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid (),
        name VARCHAR(50) NOT NULL UNIQUE,
        cpu_request_m INT NOT NULL CHECK (cpu_request_m > 0),
        cpu_limit_m INT NOT NULL CHECK (cpu_limit_m >= cpu_request_m),
        memory_request_mi INT NOT NULL CHECK (memory_request_mi > 0),
        memory_limit_mi INT NOT NULL CHECK (memory_limit_mi >= memory_request_mi),
        replicas INT NOT NULL DEFAULT 1 CHECK (replicas > 0),
        created_at TIMESTAMPTZ NOT NULL DEFAULT NOW (),
        updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW ()
    );

INSERT INTO
    plans (
        name,
        cpu_request_m,
        cpu_limit_m,
        memory_request_mi,
        memory_limit_mi,
        replicas
    )
VALUES
    ('Hobby', 100, 250, 256, 512, 1),
    ('Pro', 500, 1000, 1024, 2048, 2),
    ('Enterprise', 2000, 4000, 4096, 8192, 3);