PRAGMA foreign_keys = OFF;

BEGIN TRANSACTION;

CREATE TABLE user_new (
  id TEXT PRIMARY KEY,
  username TEXT NOT NULL UNIQUE,
  password_hash TEXT NOT NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO user_new (id, username, password_hash, created_at)
SELECT id, username, password_hash, created_at
FROM user;

DROP TABLE user;

ALTER TABLE user_new RENAME TO user;

COMMIT;

PRAGMA foreign_keys = ON;
