CREATE TABLE agent_sessions (
    id            TEXT PRIMARY KEY,
    goal          TEXT NOT NULL,
    state         TEXT NOT NULL DEFAULT 'IDLE',
    plan          JSONB,
    metrics       JSONB,
    config        JSONB,
    result        TEXT,
    error         TEXT,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    finished_at   TIMESTAMPTZ
);

CREATE INDEX idx_agent_sessions_state ON agent_sessions(state);
CREATE INDEX idx_agent_sessions_created_at ON agent_sessions(created_at);

CREATE TABLE tool_memories (
    id            TEXT PRIMARY KEY,
    session_id    TEXT NOT NULL REFERENCES agent_sessions(id) ON DELETE CASCADE,
    step_id       INT NOT NULL,
    tool_name     TEXT NOT NULL,
    raw_output    TEXT,
    summary       TEXT,
    key_points    JSONB,
    entities      JSONB,
    numbers       JSONB,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_tool_memories_session ON tool_memories(session_id);
CREATE INDEX idx_tool_memories_created ON tool_memories(created_at);

CREATE TABLE episodic_memories (
    id            TEXT PRIMARY KEY,
    goal          TEXT NOT NULL,
    plan          JSONB,
    result        TEXT,
    success       BOOLEAN NOT NULL DEFAULT FALSE,
    summary       TEXT,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE event_logs (
    id            TEXT PRIMARY KEY,
    session_id    TEXT NOT NULL,
    event_type    TEXT NOT NULL,
    payload       JSONB,
    state_before  TEXT,
    state_after   TEXT,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_event_logs_session ON event_logs(session_id);
CREATE INDEX idx_event_logs_created ON event_logs(created_at);
