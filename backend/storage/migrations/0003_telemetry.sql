-- 风控观察：出题评估（kind=issue）与校验评分（kind=verify）记录，已脱敏，按条数保留最近 N 条
CREATE TABLE telemetry (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    created_at INTEGER NOT NULL,
    kind       TEXT    NOT NULL,
    app_id     TEXT,
    payload    TEXT    NOT NULL
);
