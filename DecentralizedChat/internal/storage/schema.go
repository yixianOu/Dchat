// Package storage 实现了本地消息存储模块，使用 SQLite 数据库存储聊天记录和会话信息
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
    nats_seq INTEGER DEFAULT 0, -- NATS消息序列ID，用于去重
    FOREIGN KEY (cid) REFERENCES conversations(id)
);

CREATE INDEX IF NOT EXISTS idx_messages_cid_time ON messages(cid, timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_messages_sender ON messages(sender_id);
-- 唯一约束，防止重复存储同一条消息（NATS序列ID+会话ID全局唯一）
CREATE UNIQUE INDEX IF NOT EXISTS idx_messages_unique ON messages(cid, nats_seq);

-- 好友公钥存储表
CREATE TABLE IF NOT EXISTS friend_pub_keys (
    user_id TEXT PRIMARY KEY,
    pub_key TEXT NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- 群聊对称密钥存储表
CREATE TABLE IF NOT EXISTS group_sym_keys (
    group_id TEXT PRIMARY KEY,
    sym_key TEXT NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
`
