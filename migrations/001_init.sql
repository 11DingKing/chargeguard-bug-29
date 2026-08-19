CREATE TABLE IF NOT EXISTS schema_version (
    version    INTEGER PRIMARY KEY,
    applied_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS shard_meta (
    shard_id     TEXT PRIMARY KEY,
    date         TEXT NOT NULL,
    route_id     TEXT NOT NULL,
    file_path    TEXT NOT NULL,
    record_count INTEGER NOT NULL DEFAULT 0,
    checksum     TEXT NOT NULL DEFAULT '',
    status       TEXT NOT NULL DEFAULT 'ok',
    data_version INTEGER NOT NULL DEFAULT 1,
    created_at   TEXT NOT NULL,
    updated_at   TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_shard_date ON shard_meta(date);
CREATE INDEX IF NOT EXISTS idx_shard_route ON shard_meta(route_id);
CREATE INDEX IF NOT EXISTS idx_shard_status ON shard_meta(status);

CREATE TABLE IF NOT EXISTS mail_items (
    id             TEXT PRIMARY KEY,
    mail_no        TEXT UNIQUE NOT NULL,
    route_id       TEXT NOT NULL,
    vehicle_no     TEXT,
    state          TEXT NOT NULL,
    handover_id    TEXT,
    disposition_id TEXT,
    origin_station TEXT,
    dest_station   TEXT,
    sender_name    TEXT,
    sender_addr    TEXT,
    receiver_name  TEXT,
    receiver_addr  TEXT,
    responsible    TEXT,
    registered_at  TEXT NOT NULL,
    loaded_at      TEXT,
    in_transit_at  TEXT,
    arrived_at     TEXT,
    signed_at      TEXT,
    completed_at   TEXT,
    version        INTEGER NOT NULL DEFAULT 1,
    shard_id       TEXT NOT NULL,
    data_version   INTEGER NOT NULL DEFAULT 1
);
CREATE INDEX IF NOT EXISTS idx_mail_state ON mail_items(state);
CREATE INDEX IF NOT EXISTS idx_mail_route ON mail_items(route_id);
CREATE INDEX IF NOT EXISTS idx_mail_vehicle ON mail_items(vehicle_no);
CREATE INDEX IF NOT EXISTS idx_mail_handover ON mail_items(handover_id);

CREATE TABLE IF NOT EXISTS handover_forms (
    id                TEXT PRIMARY KEY,
    form_no           TEXT UNIQUE NOT NULL,
    date              TEXT NOT NULL,
    route_id          TEXT NOT NULL,
    vehicle_no        TEXT,
    state             TEXT NOT NULL,
    outbound_station  TEXT,
    outbound_signer   TEXT,
    outbound_signed_at TEXT,
    arrival_station   TEXT,
    arrival_signer    TEXT,
    arrival_signed_at TEXT,
    mail_item_count   INTEGER NOT NULL DEFAULT 0,
    responsible       TEXT,
    registered_at     TEXT NOT NULL,
    updated_at        TEXT NOT NULL,
    version           INTEGER NOT NULL DEFAULT 1,
    shard_id          TEXT NOT NULL,
    data_version      INTEGER NOT NULL DEFAULT 1
);
CREATE INDEX IF NOT EXISTS idx_handover_state ON handover_forms(state);
CREATE INDEX IF NOT EXISTS idx_handover_date ON handover_forms(date);
CREATE INDEX IF NOT EXISTS idx_handover_route ON handover_forms(route_id);

CREATE TABLE IF NOT EXISTS disposition_requests (
    id               TEXT PRIMARY KEY,
    request_no       TEXT UNIQUE NOT NULL,
    mail_id          TEXT NOT NULL,
    mail_no          TEXT NOT NULL,
    type             TEXT NOT NULL,
    target_address   TEXT,
    state            TEXT NOT NULL,
    submitted_by     TEXT NOT NULL,
    submitted_at     TEXT NOT NULL,
    reviewed_by      TEXT,
    reviewed_at      TEXT,
    review_note      TEXT,
    issued_by        TEXT,
    issued_at        TEXT,
    executed_at      TEXT,
    completed_at     TEXT,
    withdrawn_by     TEXT,
    withdrawn_at     TEXT,
    withdrawn_reason TEXT,
    conflict_reason  TEXT,
    lost_at          TEXT,
    version          INTEGER NOT NULL DEFAULT 1,
    shard_id         TEXT NOT NULL,
    data_version     INTEGER NOT NULL DEFAULT 1
);
CREATE INDEX IF NOT EXISTS idx_disp_state ON disposition_requests(state);
CREATE INDEX IF NOT EXISTS idx_disp_mail ON disposition_requests(mail_id);

CREATE TABLE IF NOT EXISTS active_dispositions (
    mail_id        TEXT PRIMARY KEY,
    disposition_id TEXT NOT NULL,
    created_at     TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS batch_records (
    id              TEXT PRIMARY KEY,
    vehicle_no      TEXT NOT NULL,
    date            TEXT NOT NULL,
    route_id        TEXT NOT NULL,
    state           TEXT NOT NULL,
    total_count     INTEGER NOT NULL DEFAULT 0,
    succeeded_count INTEGER NOT NULL DEFAULT 0,
    failed_count    INTEGER NOT NULL DEFAULT 0,
    created_at      TEXT NOT NULL,
    updated_at      TEXT NOT NULL,
    version         INTEGER NOT NULL DEFAULT 1,
    shard_id        TEXT NOT NULL,
    data_version    INTEGER NOT NULL DEFAULT 1
);
CREATE INDEX IF NOT EXISTS idx_batch_state ON batch_records(state);
CREATE INDEX IF NOT EXISTS idx_batch_vehicle ON batch_records(vehicle_no);

CREATE TABLE IF NOT EXISTS batch_items (
    id         TEXT PRIMARY KEY,
    batch_id   TEXT NOT NULL,
    mail_id    TEXT NOT NULL,
    mail_no    TEXT NOT NULL,
    state      TEXT NOT NULL,
    error      TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_batch_item_batch ON batch_items(batch_id);
CREATE INDEX IF NOT EXISTS idx_batch_item_state ON batch_items(state);

CREATE TABLE IF NOT EXISTS ledger_entries (
    id          TEXT PRIMARY KEY,
    date        TEXT NOT NULL,
    route_id    TEXT NOT NULL,
    volume_no   TEXT NOT NULL,
    form_no     TEXT,
    entry_type  TEXT NOT NULL,
    mail_no     TEXT,
    responsible TEXT,
    description TEXT,
    prev_state  TEXT,
    next_state  TEXT,
    created_at  TEXT NOT NULL,
    shard_id    TEXT NOT NULL,
    data_version INTEGER NOT NULL DEFAULT 1
);
CREATE INDEX IF NOT EXISTS idx_ledger_date ON ledger_entries(date);
CREATE INDEX IF NOT EXISTS idx_ledger_route ON ledger_entries(route_id);
CREATE INDEX IF NOT EXISTS idx_ledger_responsible ON ledger_entries(responsible);
CREATE INDEX IF NOT EXISTS idx_ledger_volume ON ledger_entries(volume_no);

CREATE TABLE IF NOT EXISTS events (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    type        TEXT NOT NULL,
    business_key TEXT NOT NULL,
    shard_id    TEXT,
    payload     TEXT NOT NULL,
    created_at  TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_events_type ON events(type);
CREATE INDEX IF NOT EXISTS idx_events_created ON events(created_at);

CREATE TABLE IF NOT EXISTS subscriber_checkpoints (
    id             TEXT PRIMARY KEY,
    type           TEXT NOT NULL,
    name           TEXT,
    last_event_id  INTEGER NOT NULL DEFAULT 0,
    last_active_at TEXT NOT NULL,
    created_at     TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS audit_trail (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    actor        TEXT NOT NULL,
    action       TEXT NOT NULL,
    entity_type  TEXT NOT NULL,
    entity_id    TEXT NOT NULL,
    shard_id     TEXT,
    before_state TEXT,
    after_state  TEXT,
    detail       TEXT,
    timestamp    TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_audit_actor ON audit_trail(actor);
CREATE INDEX IF NOT EXISTS idx_audit_entity ON audit_trail(entity_type, entity_id);
CREATE INDEX IF NOT EXISTS idx_audit_timestamp ON audit_trail(timestamp);

CREATE TABLE IF NOT EXISTS permanent_failures (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    task_type      TEXT NOT NULL,
    entity_id      TEXT NOT NULL,
    shard_id       TEXT,
    last_error     TEXT,
    attempts       INTEGER NOT NULL DEFAULT 0,
    max_attempts   INTEGER NOT NULL DEFAULT 3,
    last_attempt_at TEXT,
    next_retry_at  TEXT,
    status         TEXT NOT NULL DEFAULT 'pending',
    created_at     TEXT NOT NULL,
    resolved_at    TEXT
);
CREATE INDEX IF NOT EXISTS idx_failure_status ON permanent_failures(status);
CREATE INDEX IF NOT EXISTS idx_failure_task ON permanent_failures(task_type);
