-- Create sessions table for single device login enforcement
CREATE TABLE IF NOT EXISTS Session (
    id TEXT PRIMARY KEY,
    userId TEXT NOT NULL,
    token TEXT NOT NULL UNIQUE,
    deviceInfo TEXT,
    ipAddress TEXT,
    isActive INTEGER DEFAULT 1,
    expiresAt INTEGER NOT NULL,
    createdAt INTEGER DEFAULT (strftime('%s', 'now') * 1000),
    updatedAt INTEGER DEFAULT (strftime('%s', 'now') * 1000),
    FOREIGN KEY (userId) REFERENCES User(id) ON DELETE CASCADE
);

-- Create index for faster lookups
CREATE INDEX IF NOT EXISTS idx_session_user_id ON Session(userId);
CREATE INDEX IF NOT EXISTS idx_session_token ON Session(token);
CREATE INDEX IF NOT EXISTS idx_session_active ON Session(isActive);
