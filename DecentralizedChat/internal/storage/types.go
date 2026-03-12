package storage

import "time"

// StoredMessage 存储的消息
type StoredMessage struct {
	ID             string    `json:"id"`
	ConversationID string    `json:"conversation_id"`
	SenderID       string    `json:"sender_id"`
	SenderNickname string    `json:"sender_nickname"`
	Content        string    `json:"content"`
	Timestamp      time.Time `json:"timestamp"`
	IsRead         bool      `json:"is_read"`
	IsGroup        bool      `json:"is_group"`
	NatsSeq        uint64    `json:"nats_seq"` // NATS消息序列ID，用于去重
}

// StoredConversation 存储的会话
type StoredConversation struct {
	ID             string    `json:"id"`
	Type           string    `json:"type"` // "dm" or "group"
	LastMessageAt  time.Time `json:"last_message_at"`
	CreatedAt      time.Time `json:"created_at"`
}
