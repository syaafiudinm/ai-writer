-- +goose Up
CREATE TABLE users (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    email         text NOT NULL UNIQUE CHECK (email = lower(email)),
    name          text NOT NULL,
    password_hash text NOT NULL,
    role          text NOT NULL CHECK (role IN ('admin', 'user')),
    is_active     boolean NOT NULL DEFAULT true,
    created_at    timestamptz NOT NULL DEFAULT now(),
    last_login_at timestamptz
);

-- token asli tidak pernah disimpan, hanya hash SHA-256-nya
CREATE TABLE sessions (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    token_hash bytea NOT NULL UNIQUE,
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    ip         text,
    user_agent text
);
CREATE INDEX sessions_user_id_idx ON sessions (user_id);
CREATE INDEX sessions_expires_at_idx ON sessions (expires_at);

-- hanya metadata; facts dan isi draf sengaja tidak disimpan
CREATE TABLE generation_logs (
    id            bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id       uuid NOT NULL REFERENCES users (id),
    type          text NOT NULL CHECK (type IN ('berita', 'artikel')),
    tone          text NOT NULL,
    length        text NOT NULL,
    language      text NOT NULL,
    topic         text NOT NULL,
    model         text NOT NULL,
    status        text NOT NULL CHECK (status IN ('success', 'error', 'cancelled', 'timeout')),
    error_code    text,
    input_tokens  integer NOT NULL DEFAULT 0,
    output_tokens integer NOT NULL DEFAULT 0,
    ttft_ms       integer,
    duration_ms   integer NOT NULL,
    ip            text,
    created_at    timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX generation_logs_created_at_idx ON generation_logs (created_at);
CREATE INDEX generation_logs_user_id_created_at_idx ON generation_logs (user_id, created_at);
CREATE INDEX generation_logs_status_idx ON generation_logs (status);

-- actor_id NULL untuk login_failed dengan email yang tidak terdaftar
CREATE TABLE audit_logs (
    id         bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    actor_id   uuid REFERENCES users (id),
    action     text NOT NULL,
    target_id  uuid REFERENCES users (id),
    metadata   jsonb NOT NULL DEFAULT '{}',
    ip         text,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX audit_logs_created_at_idx ON audit_logs (created_at);
CREATE INDEX audit_logs_action_created_at_idx ON audit_logs (action, created_at);

-- +goose Down
DROP TABLE audit_logs;
DROP TABLE generation_logs;
DROP TABLE sessions;
DROP TABLE users;
