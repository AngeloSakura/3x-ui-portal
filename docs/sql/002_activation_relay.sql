-- 仅适用于门户 schema_version=1；停止门户并备份后显式执行一次。
-- 已有用户密码保持不变，不产生任何免密码会话。
PRAGMA foreign_keys = ON;
BEGIN IMMEDIATE;
CREATE TEMP TABLE migration_guard (version INTEGER CHECK (version = 1));
INSERT INTO migration_guard SELECT COALESCE(MAX(version), 0) FROM schema_version;
ALTER TABLE site ADD COLUMN relay_origins TEXT NOT NULL DEFAULT '';
CREATE TABLE activations (
  user_id INTEGER PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
  token_hash TEXT NOT NULL UNIQUE,
  expires_at INTEGER NOT NULL
);
UPDATE schema_version SET version = 2 WHERE version = 1;
DROP TABLE migration_guard;
COMMIT;
