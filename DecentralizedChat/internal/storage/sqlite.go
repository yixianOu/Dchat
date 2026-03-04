package storage

import (
	"database/sql"
	"time"

	_ "modernc.org/sqlite"
)

// Storage SQLite 本地存储
type Storage struct {
	db *sql.DB
}

// NewSQLiteStorage 创建 SQLite 存储
func NewSQLiteStorage(path string) (*Storage, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
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

// SaveMessage 保存消息
func (s *Storage) SaveMessage(msg *StoredMessage) error {
	_, err := s.db.Exec(`
		INSERT OR REPLACE INTO messages
		(id, cid, sender_id, sender_nickname, content, timestamp, is_read, is_group)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, msg.ID, msg.ConversationID, msg.SenderID, msg.SenderNickname,
		msg.Content, msg.Timestamp, msg.IsRead, msg.IsGroup)
	return err
}

// GetMessages 获取会话历史消息
func (s *Storage) GetMessages(cid string, limit int, before *time.Time) ([]*StoredMessage, error) {
	var rows *sql.Rows
	var err error

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

	// 反转顺序（从旧到新）
	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}

	return messages, rows.Err()
}

// MarkAsRead 标记会话消息已读
func (s *Storage) MarkAsRead(cid string, before time.Time) error {
	_, err := s.db.Exec(`
		UPDATE messages
		SET is_read = 1
		WHERE cid = ? AND timestamp <= ?
	`, cid, before)
	return err
}

// SaveConversation 保存会话
func (s *Storage) SaveConversation(conv *StoredConversation) error {
	_, err := s.db.Exec(`
		INSERT OR REPLACE INTO conversations
		(id, type, last_message_at, created_at)
		VALUES (?, ?, ?, ?)
	`, conv.ID, conv.Type, conv.LastMessageAt, conv.CreatedAt)
	return err
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
