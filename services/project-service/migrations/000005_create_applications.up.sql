CREATE TABLE
    applications (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid (),
        plan_id UUID NOT NULL,
        project_id UUID NOT NULL,
        name VARCHAR(50) NOT NULL,
        image TEXT NOT NULL,
        replicas INT NOT NULL DEFAULT 1 CHECK (replicas > 0),
        cpu_request_m INT NOT NULL CHECK (cpu_request_m > 0),
        cpu_limit_m INT NOT NULL CHECK (cpu_limit_m >= cpu_request_m),
        memory_request_mi INT NOT NULL CHECK (memory_request_mi > 0),
        memory_limit_mi INT NOT NULL CHECK (memory_limit_mi >= memory_request_mi),
        container_port INT NOT NULL CHECK (container_port BETWEEN 1 AND 65535),
        created_at TIMESTAMPTZ NOT NULL DEFAULT NOW (),
        updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW (),

        CONSTRAINT fk_application_project
            FOREIGN KEY (project_id) REFERENCES projects (id)
            ON DELETE CASCADE,

        CONSTRAINT fk_application_plan
            FOREIGN KEY (plan_id) REFERENCES plans(id),
            
        CONSTRAINT uq_application_project_name 
            UNIQUE (project_id, name),
            
        CONSTRAINT chk_application_name
            CHECK (LENGTH(TRIM(name)) >= 3),

        CONSTRAINT chk_application_image
            CHECK (LENGTH(TRIM(image)) > 0)        
    );

CREATE INDEX idx_application_project_id
    ON applications(project_id);