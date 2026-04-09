CREATE TABLE IF NOT EXISTS users (
    id INTEGER PRIMARY KEY,
    tg_id INTEGER NOT NULL UNIQUE,
    username TEXT,
    status TEXT NOT NULL DEFAULT 'active',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS groups (
    id INTEGER PRIMARY KEY,
    tg_id INTEGER NOT NULL UNIQUE,
    title TEXT,
    type TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'inactive',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS inputs_outputs (
    id INTEGER PRIMARY KEY,
    chat_id INTEGER NOT NULL,
    item_json TEXT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_inputs_outputs_chat_id_id
    ON inputs_outputs (chat_id, id);

CREATE TABLE IF NOT EXISTS cron_tasks (
    id INTEGER PRIMARY KEY,
    chat_id INTEGER NOT NULL,
    created_by_tg_id INTEGER NOT NULL,
    schedule TEXT NOT NULL,
    prompt TEXT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_cron_tasks_chat_id_id
    ON cron_tasks (chat_id, id);

CREATE TABLE IF NOT EXISTS chats (
    id INTEGER PRIMARY KEY,
    tg_id INTEGER NOT NULL UNIQUE,
    title TEXT,
    username TEXT,
    type TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'inactive',
    memory_text TEXT NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
