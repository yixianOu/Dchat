package chat

import (
	"encoding/json"
	"fmt"
	"time"

	natsservice "DecentralizedChat/internal/nats"

	"github.com/nats-io/nats.go"
)

type Service struct {
	nats  *natsservice.Service
	rooms map[string]*Room
	user  *User
}

type User struct {
	ID       string `json:"id"`
	Nickname string `json:"nickname"`
	Avatar   string `json:"avatar"`
}

type Message struct {
	ID        string    `json:"id"`
	RoomID    string    `json:"room_id"`
	UserID    string    `json:"user_id"`
	Username  string    `json:"username"`
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
	Type      string    `json:"type"` // text, image, file
}

type Room struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Members     []string   `json:"members"`
	Messages    []*Message `json:"messages"`
	CreatedAt   time.Time  `json:"created_at"`
}

func NewService(natsService *natsservice.Service) *Service {
	return &Service{
		nats:  natsService,
		rooms: make(map[string]*Room),
		user: &User{
			ID:       generateUserID(),
			Nickname: "Anonymous",
			Avatar:   "",
		},
	}
}

func (cs *Service) SetUser(nickname, avatar string) {
	cs.user.Nickname = nickname
	cs.user.Avatar = avatar
}

func (cs *Service) JoinRoom(roomName string) error {
	// 创建或获取聊天室
	room := cs.getOrCreateRoom(roomName)
	_ = room // 使用room变量，避免编译错误

	// 订阅聊天室主题
	subject := fmt.Sprintf("chat.%s", roomName)
	return cs.nats.Subscribe(subject, func(msg *nats.Msg) {
		cs.handleMessage(msg.Data)
	})
}

func (cs *Service) SendMessage(roomName, content string) error {
	room := cs.getOrCreateRoom(roomName)

	msg := &Message{
		ID:        generateMessageID(),
		RoomID:    roomName,
		UserID:    cs.user.ID,
		Username:  cs.user.Nickname,
		Content:   content,
		Timestamp: time.Now(),
		Type:      "text",
	}

	// 添加到本地消息历史
	room.Messages = append(room.Messages, msg)

	// 序列化消息
	msgBytes, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	// 发布到NATS
	subject := fmt.Sprintf("chat.%s", roomName)
	return cs.nats.Publish(subject, msgBytes)
}

func (cs *Service) GetHistory(roomName string) ([]*Message, error) {
	room := cs.getOrCreateRoom(roomName)
	return room.Messages, nil
}

func (cs *Service) GetRooms() []string {
	var rooms []string
	for name := range cs.rooms {
		rooms = append(rooms, name)
	}
	return rooms
}

func (cs *Service) handleMessage(msg []byte) {
	var chatMsg Message
	if err := json.Unmarshal(msg, &chatMsg); err != nil {
		fmt.Printf("Failed to unmarshal message: %v\n", err)
		return
	}

	// 忽略自己发送的消息
	if chatMsg.UserID == cs.user.ID {
		return
	}

	// 添加到聊天室历史
	room := cs.getOrCreateRoom(chatMsg.RoomID)
	room.Messages = append(room.Messages, &chatMsg)

	// TODO: 通知前端新消息到达
	fmt.Printf("New message in %s from %s: %s\n",
		chatMsg.RoomID, chatMsg.Username, chatMsg.Content)
}

func (cs *Service) getOrCreateRoom(roomName string) *Room {
	if room, exists := cs.rooms[roomName]; exists {
		return room
	}

	room := &Room{
		ID:          roomName,
		Name:        roomName,
		Description: fmt.Sprintf("聊天室: %s", roomName),
		Members:     []string{cs.user.ID},
		Messages:    []*Message{},
		CreatedAt:   time.Now(),
	}

	cs.rooms[roomName] = room
	return room
}

func generateUserID() string {
	return fmt.Sprintf("user_%d", time.Now().UnixNano())
}

func generateMessageID() string {
	return fmt.Sprintf("msg_%d", time.Now().UnixNano())
}
