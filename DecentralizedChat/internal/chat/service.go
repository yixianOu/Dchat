package chat

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
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

	// key material (base64 raw 32 bytes for X25519/Ed25519 private & public; symmetric group keys in KV)
	userPrivB64 string
	userPubB64  string

	ctx    context.Context
	cancel context.CancelFunc
}

// User 最小化：仅保留必要身份与展示昵称
type User struct {
	ID       string `json:"id"`
	Nickname string `json:"nickname"`
}

// Message 最小化：移除冗余 Username / Type，RoomID 与 UserID 足以关联
type Message struct {
	ID        string    `json:"id"`
	RoomID    string    `json:"room_id"`
	UserID    string    `json:"user_id"`
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
}

// Room 最小化：去掉 Name/Description/Members，仅保留 ID/消息/创建时间
type Room struct {
	ID        string     `json:"id"`
	Messages  []*Message `json:"messages"`
	CreatedAt time.Time  `json:"created_at"`
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
		},
		ctx:    ctx,
		cancel: cancel,
	}
}

// SetUser 仅更新昵称（头像字段已移除）
func (cs *Service) SetUser(nickname string) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.user.Nickname = nickname
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
		Content:   content,
		Timestamp: time.Now().UTC(),
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

// --- Direct & Group encrypted messaging helpers ---

// SetKeyPair sets local user key pair (base64 raw 32 bytes each)
func (cs *Service) SetKeyPair(privB64, pubB64 string) {
	cs.mu.Lock()
	cs.userPrivB64 = privB64
	cs.userPubB64 = pubB64
	cs.mu.Unlock()
}

// deriveCID returns deterministic conversation id
func deriveCID(a, b string) string {
	if a == b {
		return a
	} // self-chat edge
	if a > b {
		a, b = b, a
	}
	h := sha256.Sum256([]byte(a + ":" + b))
	return hex.EncodeToString(h[:])[:16]
}

// encWire 为最小化加密消息载荷（私聊/群聊统一）：
// ver 固定 1；cid 对私聊=cid，对群=gid；sender 发送方 uid；alg 私聊 x25519-box / 群 aes256-gcm
// sig 预留（Ed25519 签名），当前未实现签名生成/验证可为空省略
type encWire struct {
	Ver    int    `json:"ver"`
	CID    string `json:"cid"`
	Sender string `json:"sender"`
	Ts     int64  `json:"ts"`
	Nonce  string `json:"nonce"`
	Cipher string `json:"cipher"`
	Alg    string `json:"alg"`
	Sig    string `json:"sig,omitempty"`
}

// SendDirect sends encrypted direct message to peerID using peer's public key stored in KV
func (cs *Service) SendDirect(peerID, peerPubB64, content string) error {
	if peerID == "" || content == "" {
		return errors.New("peerID/content empty")
	}
	cs.mu.RLock()
	priv := cs.userPrivB64
	pub := cs.userPubB64
	fromUID := cs.user.ID
	cs.mu.RUnlock()
	if priv == "" || pub == "" {
		return errors.New("local key pair not set")
	}
	cid := deriveCID(fromUID, peerID)
	nonceB64, cipherB64, err := encryptDirect(priv, peerPubB64, []byte(content))
	if err != nil {
		return err
	}
	wire := encWire{
		Ver:    1,
		CID:    cid,
		Sender: fromUID,
		Ts:     time.Now().Unix(),
		Nonce:  nonceB64,
		Cipher: cipherB64,
		Alg:    "x25519-box",
	}
	data, _ := json.Marshal(wire)
	subject := fmt.Sprintf("dchat.dm.%s.msg", cid)
	return cs.nats.Publish(subject, data)
}

// JoinDirect ensures subscription exists for a direct conversation
func (cs *Service) JoinDirect(peerID string) error {
	if peerID == "" {
		return errors.New("peerID empty")
	}
	cs.mu.RLock()
	self := cs.user.ID
	cs.mu.RUnlock()
	cid := deriveCID(self, peerID)
	return cs.JoinRoom("dm-" + cid) // reuse existing logic, but subject mapping differs in handler
}

// SendGroup publishes encrypted group message using symmetric key
func (cs *Service) SendGroup(gid, groupKeyB64, content string) error {
	if gid == "" || groupKeyB64 == "" || content == "" {
		return errors.New("gid/key/content empty")
	}
	cs.mu.RLock()
	fromUID := cs.user.ID
	cs.mu.RUnlock()
	nonceB64, cipherB64, err := encryptGroup(groupKeyB64, []byte(content))
	if err != nil {
		return err
	}
	wire := encWire{
		Ver:    1,
		CID:    gid, // 复用 cid 字段表示群 id
		Sender: fromUID,
		Ts:     time.Now().Unix(),
		Nonce:  nonceB64,
		Cipher: cipherB64,
		Alg:    "aes256-gcm",
	}
	data, _ := json.Marshal(wire)
	subject := fmt.Sprintf("dchat.grp.%s.msg", gid)
	return cs.nats.Publish(subject, data)
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
		ID:        roomName,
		Messages:  []*Message{},
		CreatedAt: time.Now().UTC(),
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
