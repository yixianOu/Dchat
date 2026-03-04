package storage

const schema = `
CREATE TABLE IF NOT EXISTS conversations (
    id TEXT PRIMARY KEY,
    type TEXT NOT NULL,
    last_message_at TIMESTAMP,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS messages (
    id TEXT PRIMARY KEY,
    cid TEXT NOT NULL,
    sender_id TEXT NOT NULL,
    sender_nickname TEXT,
    content TEXT NOT NULL,
    timestamp TIMESTAMP NOT NULL,
    is_read BOOLEAN DEFAULT 0,
    is_group BOOLEAN DEFAULT 0,
    FOREIGN KEY (cid) REFERENCES conversations(id)
);

CREATE INDEX IF NOT EXISTS idx_messages_cid_time ON messages(cid, timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_messages_sender ON messages(sender_id);
`
