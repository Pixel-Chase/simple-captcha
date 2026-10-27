-- 管理员账号：单行，没有这一行即为"未初始化"，由控制台初始化页面写入
CREATE TABLE admin_account (
    id            INTEGER PRIMARY KEY CHECK (id = 1),
    username      TEXT    NOT NULL,
    password_hash TEXT    NOT NULL,
    -- 每次改密都会更换；会话记录签发时的代次，代次不一致即失效
    generation    TEXT    NOT NULL,
    updated_at    INTEGER NOT NULL
);

-- 初始化口令：未初始化时每次启动都会重新生成，只存 SHA-256 哈希，初始化成功后删除
CREATE TABLE admin_setup (
    id         INTEGER PRIMARY KEY CHECK (id = 1),
    code_hash  TEXT    NOT NULL,
    created_at INTEGER NOT NULL
);

-- 管理会话：只存令牌的 SHA-256 哈希
CREATE TABLE admin_sessions (
    token_hash TEXT    PRIMARY KEY,
    username   TEXT    NOT NULL,
    generation TEXT    NOT NULL,
    created_at INTEGER NOT NULL,
    expires_at INTEGER NOT NULL
);
CREATE INDEX admin_sessions_expires_at ON admin_sessions (expires_at);

-- 接入应用：密钥只存 SHA-256 哈希
CREATE TABLE apps (
    id                TEXT    PRIMARY KEY,
    name              TEXT    NOT NULL,
    secret_hash       TEXT    NOT NULL UNIQUE,
    secret_hint       TEXT    NOT NULL,
    enabled           INTEGER NOT NULL DEFAULT 1,
    created_at        INTEGER NOT NULL,
    updated_at        INTEGER NOT NULL,
    secret_rotated_at INTEGER NOT NULL,
    last_used_at      INTEGER
);
