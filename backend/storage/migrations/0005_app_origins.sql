-- 接入应用的允许来源：出题与提交请求的 Origin 必须在所属应用的名单内（精确匹配，已规范化）
ALTER TABLE apps ADD COLUMN is_system INTEGER NOT NULL DEFAULT 0;

CREATE TABLE app_origins (
    app_id TEXT NOT NULL REFERENCES apps (id) ON DELETE CASCADE,
    origin TEXT NOT NULL,
    PRIMARY KEY (app_id, origin)
);

-- 内置系统应用：管理后台自己的验证码，只接受同源请求；没有密钥，不能用于 S2S，也不能删除或修改
INSERT INTO apps (id, name, secret_hash, secret_hint, enabled, created_at, updated_at, secret_rotated_at, is_system)
VALUES ('app_console', '管理后台', '', '', 1, CAST(strftime('%s', 'now') AS INTEGER), CAST(strftime('%s', 'now') AS INTEGER), CAST(strftime('%s', 'now') AS INTEGER), 1);
