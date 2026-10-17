CREATE TABLE projects (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    slug        TEXT NOT NULL UNIQUE,
    name        TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    repo_path   TEXT NOT NULL DEFAULT '',
    archived    INTEGER NOT NULL DEFAULT 0,
    created_at  TEXT NOT NULL
);

CREATE TABLE items (
    id                 INTEGER PRIMARY KEY AUTOINCREMENT,
    title              TEXT NOT NULL,
    notes              TEXT NOT NULL DEFAULT '',
    kind               TEXT NOT NULL,
    project_id         INTEGER REFERENCES projects(id),
    parent_id          INTEGER,
    status             TEXT NOT NULL,
    "when"             TEXT NOT NULL,
    context            TEXT NOT NULL,
    priority           INTEGER NOT NULL DEFAULT 0,
    position           INTEGER NOT NULL DEFAULT 0,
    why_first          TEXT NOT NULL DEFAULT '',
    depends_on         TEXT NOT NULL DEFAULT '[]',
    branch             TEXT NOT NULL DEFAULT '',
    worktree           TEXT NOT NULL DEFAULT '',
    session            TEXT NOT NULL DEFAULT '',
    session_model      TEXT NOT NULL DEFAULT '',
    session_started_at TEXT,
    model              TEXT NOT NULL DEFAULT '',
    effort             TEXT NOT NULL DEFAULT '',
    evidence           TEXT NOT NULL DEFAULT '[]',
    attachments        TEXT NOT NULL DEFAULT '[]',
    created_at         TEXT NOT NULL,
    updated_at         TEXT NOT NULL,
    done_at            TEXT
);
CREATE INDEX items_project ON items(project_id, status);

CREATE TABLE habits (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    name       TEXT NOT NULL,
    icon       TEXT NOT NULL DEFAULT '',
    unit       TEXT NOT NULL DEFAULT '',
    type       TEXT NOT NULL,
    target     REAL NOT NULL,
    step       REAL NOT NULL,
    "order"    INTEGER NOT NULL DEFAULT 0,
    archived   INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL
);

CREATE TABLE habit_logs (
    habit_id INTEGER NOT NULL REFERENCES habits(id),
    date     TEXT NOT NULL,
    value    REAL NOT NULL,
    PRIMARY KEY (habit_id, date)
);

CREATE TABLE settings (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);
