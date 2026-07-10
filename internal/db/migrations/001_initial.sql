CREATE EXTENSION IF NOT EXISTS "pgcrypto";

CREATE TABLE IF NOT EXISTS workers (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name            VARCHAR(255) NOT NULL,
    hostname        VARCHAR(255) NOT NULL DEFAULT '',
    ip_address      VARCHAR(45)  NOT NULL UNIQUE,
    ssh_username    VARCHAR(255) NOT NULL,
    ssh_password_encrypted TEXT,
    ssh_public_key  TEXT,
    api_key         VARCHAR(255),
    provision_status VARCHAR(50) NOT NULL DEFAULT 'pending',
    last_heartbeat  TIMESTAMPTZ,
    worker_uuid     VARCHAR(36),
    nomad_node_id   VARCHAR(255) NOT NULL DEFAULT '',
    cpu_usage       FLOAT   NOT NULL DEFAULT 0,
    ram_usage       FLOAT   NOT NULL DEFAULT 0,
    disk_usage      FLOAT   NOT NULL DEFAULT 0,
    network_rx      BIGINT  NOT NULL DEFAULT 0,
    network_tx      BIGINT  NOT NULL DEFAULT 0,
    docker_status   VARCHAR(50) NOT NULL DEFAULT 'unknown',
    nomad_status    VARCHAR(50) NOT NULL DEFAULT 'unknown',
    judge_status    VARCHAR(50) NOT NULL DEFAULT 'unknown',
    running_jobs    INT     NOT NULL DEFAULT 0,
    total_jobs      BIGINT  NOT NULL DEFAULT 0,
    failed_jobs     BIGINT  NOT NULL DEFAULT 0,
    avg_runtime     FLOAT   NOT NULL DEFAULT 0,
    uptime          BIGINT  NOT NULL DEFAULT 0,
    worker_version  VARCHAR(50)  NOT NULL DEFAULT '',
    cpu_model       VARCHAR(255) NOT NULL DEFAULT '',
    cpu_cores       INT     NOT NULL DEFAULT 0,
    total_ram       BIGINT  NOT NULL DEFAULT 0,
    total_disk      BIGINT  NOT NULL DEFAULT 0,
    os_version      VARCHAR(255) NOT NULL DEFAULT '',
    kernel_version  VARCHAR(255) NOT NULL DEFAULT '',
    docker_version  VARCHAR(50)  NOT NULL DEFAULT '',
    nomad_version   VARCHAR(50)  NOT NULL DEFAULT '',
    judge_version   VARCHAR(50)  NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS provision_logs (
    id         BIGSERIAL PRIMARY KEY,
    worker_id  UUID NOT NULL REFERENCES workers(id) ON DELETE CASCADE,
    step       VARCHAR(255) NOT NULL,
    status     VARCHAR(50)  NOT NULL,
    message    TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS audit_logs (
    id           BIGSERIAL PRIMARY KEY,
    action       VARCHAR(255) NOT NULL,
    worker_id    UUID REFERENCES workers(id) ON DELETE SET NULL,
    performed_by VARCHAR(255) NOT NULL DEFAULT 'admin',
    details      TEXT NOT NULL DEFAULT '',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS notifications (
    id         BIGSERIAL PRIMARY KEY,
    type       VARCHAR(100) NOT NULL,
    title      VARCHAR(255) NOT NULL,
    message    TEXT NOT NULL DEFAULT '',
    worker_id  UUID REFERENCES workers(id) ON DELETE CASCADE,
    read       BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_workers_status      ON workers(provision_status);
CREATE INDEX IF NOT EXISTS idx_prov_logs_worker    ON provision_logs(worker_id);
CREATE INDEX IF NOT EXISTS idx_notifications_unread ON notifications(read) WHERE read = FALSE;
CREATE INDEX IF NOT EXISTS idx_audit_logs_worker   ON audit_logs(worker_id);
