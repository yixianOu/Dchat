package chat

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	natsservice "DecentralizedChat/internal/nats"

	"github.com/nats-io/nats.go"
)

// User 基础身份
type User struct {
	ID       string `json:"id"`
	Nickname string `json:"nickname"`
}

// encWire 最小载荷（见 README）：{cid,sender,ts,nonce,cipher}
type encWire struct {
	CID    string `json:"cid"`
	Sender string `json:"sender"`
	Ts     int64  `json:"ts"`
	Nonce  string `json:"nonce"`
	Cipher string `json:"cipher"`
}

// DecryptedMessage 统一回调结构
type DecryptedMessage struct {
	CID     string    // 会话 cid 或 群 gid
	Sender  string    // 发送者 uid
	Ts      time.Time // 原始发送秒级时间戳转时间
	Plain   string    // 解密后明文
	IsGroup bool      // 是否群聊
	RawWire encWire   // 原始载荷
	Subject string    // 原始 NATS subject
}

// Service 精简：仅支持私聊/群聊加密收发，无本地房间/历史存储。
type Service struct {
	nats *natsservice.Service

	mu sync.RWMutex

	user        *User
	userPrivB64 string
	userPubB64  string

	// key caches
	friendPubKeys map[string]string // uid -> pub (b64)
	groupSymKeys  map[string]string // gid -> sym (b64)

	// active subscriptions
	directSubs map[string]*nats.Subscription // cid -> sub
	groupSubs  map[string]*nats.Subscription // gid -> sub

	handlers []func(*DecryptedMessage)

	ctx    context.Context
	cancel context.CancelFunc
}

// NewService create minimal service
func NewService(n *natsservice.Service) *Service {
	ctx, cancel := context.WithCancel(context.Background())
	return &Service{
		nats:          n,
		user:          &User{ID: generateUserID(), Nickname: "Anonymous"},
		friendPubKeys: make(map[string]string),
		groupSymKeys:  make(map[string]string),
		directSubs:    make(map[string]*nats.Subscription),
		groupSubs:     make(map[string]*nats.Subscription),
		ctx:           ctx,
		cancel:        cancel,
	}
}

func (s *Service) SetUser(nickname string) {
	s.mu.Lock()
	s.user.Nickname = nickname
	s.mu.Unlock()
}

func (s *Service) GetUser() User { s.mu.RLock(); defer s.mu.RUnlock(); return *s.user }

// SetKeyPair 设置本地密钥对
func (s *Service) SetKeyPair(privB64, pubB64 string) {
	s.mu.Lock()
	s.userPrivB64 = privB64
	s.userPubB64 = pubB64
	s.mu.Unlock()
}

// AddFriendKey 缓存好友公钥
func (s *Service) AddFriendKey(uid, pubB64 string) {
	if uid == "" || pubB64 == "" {
		return
	}
	s.mu.Lock()
	s.friendPubKeys[uid] = pubB64
	s.mu.Unlock()
}

// AddGroupKey 缓存群对称密钥
func (s *Service) AddGroupKey(gid, symB64 string) {
	if gid == "" || symB64 == "" {
		return
	}
	s.mu.Lock()
	s.groupSymKeys[gid] = symB64
	s.mu.Unlock()
}

// deriveCID 生成私聊 cid
func deriveCID(a, b string) string {
	if a > b {
		a, b = b, a
	}
	h := sha256.Sum256([]byte(a + ":" + b))
	return hex.EncodeToString(h[:])[:16]
}

// OnDecrypted 注册解密后回调
func (s *Service) OnDecrypted(h func(*DecryptedMessage)) {
	if h == nil {
		return
	}
	s.mu.Lock()
	s.handlers = append(s.handlers, h)
	s.mu.Unlock()
}

// JoinDirect 订阅一个私聊会话（需要先 AddFriendKey）
func (s *Service) JoinDirect(peerID string) error {
	if peerID == "" {
		return errors.New("peerID empty")
	}
	s.mu.RLock()
	self := s.user.ID
	_, ok := s.friendPubKeys[peerID]
	s.mu.RUnlock()
	if !ok {
		return errors.New("friend pub key missing; call AddFriendKey first")
	}
	cid := deriveCID(self, peerID)
	s.mu.RLock()
	if _, exists := s.directSubs[cid]; exists {
		s.mu.RUnlock()
		return nil
	}
	s.mu.RUnlock()
	subj := fmt.Sprintf("dchat.dm.%s.msg", cid)
	if err := s.nats.Subscribe(subj, func(m *nats.Msg) { s.handleEncrypted(subj, m.Data) }); err != nil {
		return err
	}
	s.mu.Lock()
	s.directSubs[cid] = nil
	s.mu.Unlock()
	return nil
}

// JoinGroup 订阅群（需要先 AddGroupKey）
func (s *Service) JoinGroup(gid string) error {
	if gid == "" {
		return errors.New("gid empty")
	}
	s.mu.RLock()
	_, ok := s.groupSymKeys[gid]
	s.mu.RUnlock()
	if !ok {
		return errors.New("group key missing; call AddGroupKey first")
	}
	s.mu.RLock()
	if _, exists := s.groupSubs[gid]; exists {
		s.mu.RUnlock()
		return nil
	}
	s.mu.RUnlock()
	subj := fmt.Sprintf("dchat.grp.%s.msg", gid)
	if err := s.nats.Subscribe(subj, func(m *nats.Msg) { s.handleEncrypted(subj, m.Data) }); err != nil {
		return err
	}
	s.mu.Lock()
	s.groupSubs[gid] = nil
	s.mu.Unlock()
	return nil
}

// SendDirect 发送私聊
func (s *Service) SendDirect(peerID, content string) error {
	if peerID == "" || content == "" {
		return errors.New("peerID/content empty")
	}
	s.mu.RLock()
	priv := s.userPrivB64
	from := s.user.ID
	peerPub, ok := s.friendPubKeys[peerID]
	s.mu.RUnlock()
	if !ok {
		return errors.New("friend pub key missing")
	}
	if priv == "" {
		return errors.New("local priv key empty")
	}
	cid := deriveCID(from, peerID)
	nonceB64, cipherB64, err := encryptDirect(priv, peerPub, []byte(content))
	if err != nil {
		return err
	}
	wire := encWire{CID: cid, Sender: from, Ts: time.Now().Unix(), Nonce: nonceB64, Cipher: cipherB64}
	data, _ := json.Marshal(wire)
	subj := fmt.Sprintf("dchat.dm.%s.msg", cid)
	return s.nats.Publish(subj, data)
}

// SendGroup 发送群聊
func (s *Service) SendGroup(gid, content string) error {
	if gid == "" || content == "" {
		return errors.New("gid/content empty")
	}
	s.mu.RLock()
	sym, ok := s.groupSymKeys[gid]
	from := s.user.ID
	s.mu.RUnlock()
	if !ok {
		return errors.New("group key missing")
	}
	nonceB64, cipherB64, err := encryptGroup(sym, []byte(content))
	if err != nil {
		return err
	}
	wire := encWire{CID: gid, Sender: from, Ts: time.Now().Unix(), Nonce: nonceB64, Cipher: cipherB64}
	data, _ := json.Marshal(wire)
	subj := fmt.Sprintf("dchat.grp.%s.msg", gid)
	return s.nats.Publish(subj, data)
}

// handleEncrypted 解密并派发
func (s *Service) handleEncrypted(subject string, data []byte) {
	var w encWire
	if err := json.Unmarshal(data, &w); err != nil {
		return
	}
	s.mu.RLock()
	priv := s.userPrivB64
	peerPub := s.friendPubKeys[w.Sender]
	sym := s.groupSymKeys[w.CID]
	selfID := s.user.ID
	handlers := append([]func(*DecryptedMessage){}, s.handlers...)
	s.mu.RUnlock()
	if w.Sender == selfID {
		return
	} // 忽略自发回环
	var pt []byte
	var err error
	isGroup := strings.HasPrefix(subject, "dchat.grp.")
	if isGroup {
		if sym == "" {
			return
		}
		pt, err = decryptGroup(sym, w.Nonce, w.Cipher)
	} else { // direct
		if priv == "" || peerPub == "" {
			return
		}
		pt, err = decryptDirect(priv, peerPub, w.Nonce, w.Cipher)
	}
	if err != nil {
		return
	}
	msg := &DecryptedMessage{CID: w.CID, Sender: w.Sender, Ts: time.Unix(w.Ts, 0), Plain: string(pt), IsGroup: isGroup, RawWire: w, Subject: subject}
	for _, h := range handlers {
		func(cb func(*DecryptedMessage)) { defer func() { _ = recover() }(); cb(msg) }(h)
	}
}

// Close 关闭所有订阅
func (s *Service) Close() error {
	s.cancel()
	s.mu.Lock()
	defer s.mu.Unlock()
	// stored subscriptions are nil placeholders (nats service doesn't expose *nats.Subscription)
	s.directSubs = map[string]*nats.Subscription{}
	s.groupSubs = map[string]*nats.Subscription{}
	s.handlers = nil
	return nil
}

// --- helpers ---
func generateUserID() string { return "user_" + randomID() }
func randomID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}
