PRAGMA foreign_keys = ON;
BEGIN IMMEDIATE;
CREATE TABLE IF NOT EXISTS schema_version (version INTEGER PRIMARY KEY);
INSERT OR IGNORE INTO schema_version VALUES (2);
CREATE TABLE IF NOT EXISTS site (
  id INTEGER PRIMARY KEY CHECK (id = 1),
  name TEXT NOT NULL,
  suffix TEXT NOT NULL UNIQUE,
  panel_url TEXT NOT NULL,
  panel_token TEXT NOT NULL,
  support_url TEXT NOT NULL DEFAULT '',
  allow_rotation INTEGER NOT NULL DEFAULT 0,
  relay_origins TEXT NOT NULL DEFAULT '',
  installed_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS users (
  id INTEGER PRIMARY KEY,
  username TEXT NOT NULL COLLATE NOCASE UNIQUE,
  password_hash TEXT NOT NULL,
  role TEXT NOT NULL CHECK (role IN ('admin', 'user')),
  disabled INTEGER NOT NULL DEFAULT 0,
  client_id INTEGER UNIQUE,
  client_email TEXT UNIQUE,
  created_at INTEGER NOT NULL,
  CHECK ((role = 'admin' AND client_id IS NULL AND client_email IS NULL)
    OR (role = 'user' AND client_id > 0 AND client_email IS NOT NULL))
);
CREATE TABLE IF NOT EXISTS sessions (
  token_hash TEXT PRIMARY KEY,
  user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  csrf TEXT NOT NULL,
  expires_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS sessions_user ON sessions(user_id);
CREATE TABLE IF NOT EXISTS activations (
  user_id INTEGER PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
  token_hash TEXT NOT NULL UNIQUE,
  expires_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS audits (
  id INTEGER PRIMARY KEY,
  actor_id INTEGER NOT NULL,
  action TEXT NOT NULL,
  target_id INTEGER NOT NULL,
  result TEXT NOT NULL,
  created_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS rotations (
  user_id INTEGER PRIMARY KEY REFERENCES users(id),
  target_secret TEXT NOT NULL,
  protected_hash TEXT NOT NULL,
  status TEXT NOT NULL CHECK (status IN ('pending', 'confirmed', 'review')),
  created_at INTEGER NOT NULL
);
COMMIT;
