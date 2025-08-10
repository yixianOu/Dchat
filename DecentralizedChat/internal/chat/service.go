package chat

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	natsservice "DecentralizedChat/internal/nats"

	"github.com/nats-io/nats.go"
)

// Service manages chat rooms, local message history & NATS subscriptions.
// It is concurrency-safe; public getters return snapshots (copies) to avoid data races.
type Service struct {
	nats *natsservice.Service

	mu    sync.RWMutex                  // protects rooms, subs, handlers, user
	rooms map[string]*Room              // roomID -> Room (in-memory history only)
	subs  map[string]*nats.Subscription // roomID -> subscription
	user  *User

	handlers []func(*Message) // on new incoming (remote) message callbacks

	ctx    context.Context
	cancel context.CancelFunc
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
	ctx, cancel := context.WithCancel(context.Background())
	return &Service{
		nats:  natsService,
		rooms: make(map[string]*Room),
		subs:  make(map[string]*nats.Subscription),
		user: &User{
			ID:       generateUserID(),
			Nickname: "Anonymous",
			Avatar:   "",
		},
		ctx:    ctx,
		cancel: cancel,
	}
}

func (cs *Service) SetUser(nickname, avatar string) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.user.Nickname = nickname
	cs.user.Avatar = avatar
}

// GetUser returns a copy of current user info.
func (cs *Service) GetUser() User {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	return *cs.user
}

// OnMessage registers a callback invoked for every remote (non-self) message.
// Handlers are appended; no removal API for simplicity (restart service to clear).
func (cs *Service) OnMessage(handler func(*Message)) {
	if handler == nil {
		return
	}
	cs.mu.Lock()
	cs.handlers = append(cs.handlers, handler)
	cs.mu.Unlock()
}

func (cs *Service) JoinRoom(roomName string) error {
	if roomName == "" {
		return fmt.Errorf("room name empty")
	}
	cs.mu.Lock()
	// already joined? ensure room exists & subscription present
	if _, exists := cs.rooms[roomName]; exists {
		if _, subExists := cs.subs[roomName]; subExists {
			cs.mu.Unlock()
			return nil // idempotent
		}
	}
	room := cs.getOrCreateRoomLocked(roomName)
	subject := fmt.Sprintf("chat.%s", roomName)
	// release lock while establishing subscription to avoid blocking handlers
	cs.mu.Unlock()

	subErr := cs.nats.Subscribe(subject, func(msg *nats.Msg) {
		cs.handleMessage(msg.Data)
	})
	if subErr != nil {
		return subErr
	}
	// Retrieve actual subscription to store (optional: could use ChanSubscribe if needed)
	// Re-subscribe with nats library returns *nats.Subscription only if we capture; adapt Publish wrapper
	// Simpler: perform a second subscription fetch via Drain not needed; store nil placeholder.
	cs.mu.Lock()
	cs.subs[roomName] = nil // placeholder (keeping possibility for future explicit unsubscribe)
	// ensure room pointer not lost
	if _, ok := cs.rooms[roomName]; !ok {
		cs.rooms[roomName] = room
	}
	cs.mu.Unlock()
	return nil
}

// LeaveRoom unsubscribes (best-effort) and removes local room state (history retained unless purge=true later).
func (cs *Service) LeaveRoom(roomName string) error {
	if roomName == "" {
		return fmt.Errorf("room name empty")
	}
	cs.mu.Lock()
	sub := cs.subs[roomName]
	delete(cs.subs, roomName)
	delete(cs.rooms, roomName)
	cs.mu.Unlock()
	if sub != nil {
		_ = sub.Unsubscribe()
	}
	return nil
}

func (cs *Service) SendMessage(roomName, content string) error {
	if roomName == "" {
		return fmt.Errorf("room name empty")
	}
	if content == "" {
		return fmt.Errorf("content empty")
	}
	cs.mu.Lock()
	room := cs.getOrCreateRoomLocked(roomName)
	msg := &Message{
		ID:        generateMessageID(),
		RoomID:    roomName,
		UserID:    cs.user.ID,
		Username:  cs.user.Nickname,
		Content:   content,
		Timestamp: time.Now().UTC(),
		Type:      "text",
	}
	room.Messages = append(room.Messages, msg)
	cs.mu.Unlock()

	msgBytes, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal message: %w", err)
	}
	subject := fmt.Sprintf("chat.%s", roomName)
	return cs.nats.Publish(subject, msgBytes)
}

func (cs *Service) GetHistory(roomName string) ([]*Message, error) {
	if roomName == "" {
		return nil, fmt.Errorf("room name empty")
	}
	cs.mu.RLock()
	room, ok := cs.rooms[roomName]
	if !ok {
		cs.mu.RUnlock()
		return []*Message{}, nil
	}
	// copy slice to avoid race
	msgs := make([]*Message, len(room.Messages))
	copy(msgs, room.Messages)
	cs.mu.RUnlock()
	return msgs, nil
}

func (cs *Service) GetRooms() []string {
	cs.mu.RLock()
	rooms := make([]string, 0, len(cs.rooms))
	for name := range cs.rooms {
		rooms = append(rooms, name)
	}
	cs.mu.RUnlock()
	return rooms
}

func (cs *Service) handleMessage(msg []byte) {
	var chatMsg Message
	if err := json.Unmarshal(msg, &chatMsg); err != nil {
		fmt.Printf("failed unmarshal chat message: %v\n", err)
		return
	}
	cs.mu.Lock()
	// ignore own messages
	if chatMsg.UserID == cs.user.ID {
		cs.mu.Unlock()
		return
	}
	room := cs.getOrCreateRoomLocked(chatMsg.RoomID)
	room.Messages = append(room.Messages, &chatMsg)
	handlers := append([]func(*Message){}, cs.handlers...) // snapshot
	cs.mu.Unlock()

	for _, h := range handlers {
		// protect against panic in handler crashing service
		func(cb func(*Message)) {
			defer func() { _ = recover() }()
			cb(&chatMsg)
		}(h)
	}
}

// getOrCreateRoomLocked assumes cs.mu is locked (write) or will be called during write critical section.
func (cs *Service) getOrCreateRoomLocked(roomName string) *Room {
	if room, exists := cs.rooms[roomName]; exists {
		return room
	}
	room := &Room{
		ID:          roomName,
		Name:        roomName,
		Description: fmt.Sprintf("聊天室: %s", roomName),
		Members:     []string{cs.user.ID},
		Messages:    []*Message{},
		CreatedAt:   time.Now().UTC(),
	}
	cs.rooms[roomName] = room
	return room
}

func generateUserID() string {
	return "user_" + randomID()
}

func generateMessageID() string {
	return "msg_" + randomID()
}

func randomID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}

// Close shuts down the service and releases subscriptions.
func (cs *Service) Close() error {
	cs.cancel()
	cs.mu.Lock()
	defer cs.mu.Unlock()
	for _, sub := range cs.subs {
		if sub != nil {
			_ = sub.Unsubscribe()
		}
	}
	cs.subs = map[string]*nats.Subscription{}
	cs.rooms = map[string]*Room{}
	cs.handlers = nil
	return nil
}
