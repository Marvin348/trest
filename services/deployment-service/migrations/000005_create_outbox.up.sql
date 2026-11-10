CREATE TABLE
    outbox_messages (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid (),
        type TEXT NOT NULL,
        payload JSONB NOT NULL CHECK (jsonb_typeof (payload) = 'object'),
        status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'published', 'failed')),
        attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
        created_at TIMESTAMPTZ NOT NULL DEFAULT NOW (),
        published_at TIMESTAMPTZ
    );

CREATE INDEX idx_outbox_pending 
ON outbox_messages (created_at)
WHERE status = 'pending';