-- 验证码与风控策略：单行，data 为 RuntimeSettings 的 JSON；只保存当前值，不做修改历史
CREATE TABLE settings (
    id         INTEGER PRIMARY KEY CHECK (id = 1),
    data       TEXT    NOT NULL,
    updated_at INTEGER NOT NULL
);
