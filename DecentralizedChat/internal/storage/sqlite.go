package storage

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// Storage SQLite 本地存储
type Storage struct {
	db *sql.DB
}

// withRetry 为数据库写操作提供重试机制，解决SQLITE_BUSY锁冲突问题
func withRetry(attempts int, fn func() error) error {
	var err error
	for i := 0; i < attempts; i++ {
		err = fn()
		if err == nil {
			return nil
		}
		// 只有BUSY相关错误才重试，其他错误直接返回
		errStr := strings.ToLower(err.Error())
		if !strings.Contains(errStr, "database is locked") &&
		   !strings.Contains(errStr, "sqlite_busy") &&
		   !strings.Contains(errStr, "busy") {
			return err
		}
		// 指数退避延迟：第1次10ms，第2次20ms，第3次40ms，最大到200ms
		delay := time.Duration(10*(1<<i)) * time.Millisecond
		if delay > 200*time.Millisecond {
			delay = 200 * time.Millisecond
		}
		time.Sleep(delay)
	}
	return fmt.Errorf("after %d attempts: %w", attempts, err)
}

// NewSQLiteStorage 创建 SQLite 存储
func NewSQLiteStorage(path string) (*Storage, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}

	// 开启WAL模式和性能优化，大幅提升并发写入性能，减少锁冲突
	_, err = db.Exec(`
		PRAGMA journal_mode = WAL;        -- 开启WAL模式，支持高并发读写
		PRAGMA busy_timeout = 3000;       -- 内置busy超时3秒
		PRAGMA synchronous = NORMAL;      -- 平衡性能和安全性
		PRAGMA cache_size = -20000;        -- 20MB页面缓存
		PRAGMA temp_store = MEMORY;       -- 临时表存放在内存
		PRAGMA foreign_keys = OFF;        -- 关闭外键约束，兼容现有表结构
	`)
	if err != nil {
		return nil, fmt.Errorf("set sqlite optimization pragmas: %w", err)
	}

	// 执行 schema 初始化
	_, err = db.Exec(schema)
	if err != nil {
		return nil, err
	}

	return &Storage{db: db}, nil
}

// Close 关闭数据库连接
func (s *Storage) Close() error {
	return s.db.Close()
}

// SaveMessage 保存消息，自动去重（基于NATS序列ID）
func (s *Storage) SaveMessage(msg *StoredMessage) error {
	return withRetry(5, func() error {
		_, err := s.db.Exec(`
			INSERT OR IGNORE INTO messages
			(id, cid, sender_id, sender_nickname, content, timestamp, is_read, is_group, nats_seq)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, msg.ID, msg.ConversationID, msg.SenderID, msg.SenderNickname,
			msg.Content, msg.Timestamp, msg.IsRead, msg.IsGroup, msg.NatsSeq)
		return err
	})
}

// GetMessages 获取会话历史消息，cid为空时返回所有消息
func (s *Storage) GetMessages(cid string, limit int, before *time.Time) ([]*StoredMessage, error) {
	var rows *sql.Rows
	var err error

	if cid == "" {
		// 空cid返回所有消息
		if before != nil {
			rows, err = s.db.Query(`
				SELECT id, cid, sender_id, sender_nickname, content, timestamp, is_read, is_group
				FROM messages
				WHERE timestamp < ?
				ORDER BY timestamp DESC
				LIMIT ?
			`, *before, limit)
		} else {
			rows, err = s.db.Query(`
				SELECT id, cid, sender_id, sender_nickname, content, timestamp, is_read, is_group
				FROM messages
				ORDER BY timestamp DESC
				LIMIT ?
			`, limit)
		}
	} else {
		// 返回指定会话的消息
		if before != nil {
			rows, err = s.db.Query(`
				SELECT id, cid, sender_id, sender_nickname, content, timestamp, is_read, is_group
				FROM messages
				WHERE cid = ? AND timestamp < ?
				ORDER BY timestamp DESC
				LIMIT ?
			`, cid, *before, limit)
		} else {
			rows, err = s.db.Query(`
				SELECT id, cid, sender_id, sender_nickname, content, timestamp, is_read, is_group
				FROM messages
				WHERE cid = ?
				ORDER BY timestamp DESC
				LIMIT ?
			`, cid, limit)
		}
	}

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []*StoredMessage
	for rows.Next() {
		msg := &StoredMessage{}
		err := rows.Scan(
			&msg.ID, &msg.ConversationID, &msg.SenderID, &msg.SenderNickname,
			&msg.Content, &msg.Timestamp, &msg.IsRead, &msg.IsGroup,
		)
		if err != nil {
			return nil, err
		}
		messages = append(messages, msg)
	}

	// 反转顺序，得到 旧→新 的排序，这样渲染时最旧的消息在最上面，最新的在最下面
	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}

	return messages, rows.Err()
}

// MarkAsRead 标记会话消息已读
func (s *Storage) MarkAsRead(cid string, before time.Time) error {
	return withRetry(5, func() error {
		_, err := s.db.Exec(`
			UPDATE messages
			SET is_read = 1
			WHERE cid = ? AND timestamp <= ?
		`, cid, before)
		return err
	})
}

// SaveConversation 保存会话
func (s *Storage) SaveConversation(conv *StoredConversation) error {
	return withRetry(5, func() error {
		_, err := s.db.Exec(`
			INSERT OR REPLACE INTO conversations
			(id, type, last_message_at, created_at)
			VALUES (?, ?, ?, ?)
		`, conv.ID, conv.Type, conv.LastMessageAt, conv.CreatedAt)
		return err
	})
}

// GetConversation 获取会话
func (s *Storage) GetConversation(id string) (*StoredConversation, error) {
	conv := &StoredConversation{}
	err := s.db.QueryRow(`
		SELECT id, type, last_message_at, created_at
		FROM conversations
		WHERE id = ?
	`, id).Scan(&conv.ID, &conv.Type, &conv.LastMessageAt, &conv.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return conv, err
}

// SearchMessages 搜索消息
func (s *Storage) SearchMessages(query string, limit int) ([]*StoredMessage, error) {
	rows, err := s.db.Query(`
		SELECT id, cid, sender_id, sender_nickname, content, timestamp, is_read, is_group
		FROM messages
		WHERE content LIKE ?
		ORDER BY timestamp DESC
		LIMIT ?
	`, "%"+query+"%", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []*StoredMessage
	for rows.Next() {
		msg := &StoredMessage{}
		err := rows.Scan(
			&msg.ID, &msg.ConversationID, &msg.SenderID, &msg.SenderNickname,
			&msg.Content, &msg.Timestamp, &msg.IsRead, &msg.IsGroup,
		)
		if err != nil {
			return nil, err
		}
		messages = append(messages, msg)
	}

	return messages, rows.Err()
}

// SaveFriendPubKey 保存好友公钥
func (s *Storage) SaveFriendPubKey(userID, pubKey string) error {
	return withRetry(5, func() error {
		_, err := s.db.Exec(`
			INSERT OR REPLACE INTO friend_pub_keys
			(user_id, pub_key)
			VALUES (?, ?)
		`, userID, pubKey)
		return err
	})
}

// GetFriendPubKey 获取好友公钥
func (s *Storage) GetFriendPubKey(userID string) (string, error) {
	var pubKey string
	err := s.db.QueryRow(`
		SELECT pub_key FROM friend_pub_keys WHERE user_id = ?
	`, userID).Scan(&pubKey)
	if err == sql.ErrNoRows {
		return "", fmt.Errorf("friend pub key not found: %s", userID)
	}
	return pubKey, err
}

// SaveGroupSymKey 保存群聊对称密钥
func (s *Storage) SaveGroupSymKey(groupID, symKey string) error {
	return withRetry(5, func() error {
		_, err := s.db.Exec(`
			INSERT OR REPLACE INTO group_sym_keys
			(group_id, sym_key)
			VALUES (?, ?)
		`, groupID, symKey)
		return err
	})
}

// GetGroupSymKey 获取群聊对称密钥
func (s *Storage) GetGroupSymKey(groupID string) (string, error) {
	var symKey string
	err := s.db.QueryRow(`
		SELECT sym_key FROM group_sym_keys WHERE group_id = ?
	`, groupID).Scan(&symKey)
	if err == sql.ErrNoRows {
		return "", fmt.Errorf("group sym key not found: %s", groupID)
	}
	return symKey, err
}

// GetAllFriends 获取所有好友ID列表
func (s *Storage) GetAllFriends() ([]string, error) {
	rows, err := s.db.Query(`SELECT user_id FROM friend_pub_keys`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var friends []string
	for rows.Next() {
		var userID string
		if err := rows.Scan(&userID); err != nil {
			return nil, err
		}
		friends = append(friends, userID)
	}
	return friends, rows.Err()
}

// GetAllGroups 获取所有群聊ID列表
func (s *Storage) GetAllGroups() ([]string, error) {
	rows, err := s.db.Query(`SELECT group_id FROM group_sym_keys`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var groups []string
	for rows.Next() {
		var groupID string
		if err := rows.Scan(&groupID); err != nil {
			return nil, err
		}
		groups = append(groups, groupID)
	}
	return groups, rows.Err()
}

// GetAllConversations 获取所有会话列表，按最后消息时间倒序排列
func (s *Storage) GetAllConversations() ([]*StoredConversation, error) {
	rows, err := s.db.Query(`
		SELECT id, type, last_message_at, created_at
		FROM conversations
		ORDER BY last_message_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var convs []*StoredConversation
	for rows.Next() {
		conv := &StoredConversation{}
		if err := rows.Scan(&conv.ID, &conv.Type, &conv.LastMessageAt, &conv.CreatedAt); err != nil {
			return nil, err
		}
		convs = append(convs, conv)
	}
	return convs, rows.Err()
}
