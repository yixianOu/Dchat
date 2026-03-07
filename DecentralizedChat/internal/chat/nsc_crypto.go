package chat

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"github.com/nats-io/nkeys"
	"golang.org/x/crypto/curve25519"
	"filippo.io/edwards25519"
)

// NSCKeyManager 处理NSC密钥和所有派生密钥的统一管理器
type NSCKeyManager struct {
	userSeed   string // NSC用户seed（主密钥源）
	userPubKey string // NSC用户公钥
}

// KeyDomain 定义不同密钥的用途域
type KeyDomain string

const (
	DomainAuth KeyDomain = "auth" // NSC身份认证（原始用途）
	DomainChat KeyDomain = "chat" // 聊天端到端加密
)

// DerivedKeyPair 派生的密钥对
type DerivedKeyPair struct {
	PrivateKeyB64 string    `json:"private_key"`
	PublicKeyB64  string    `json:"public_key"`
	Domain        KeyDomain `json:"domain"`
	KeyType       string    `json:"key_type"` // "X25519", "Ed25519"
}


// NewNSCKeyManager 从NSC seed创建密钥管理器
func NewNSCKeyManager(seed string) (*NSCKeyManager, error) {
	if seed == "" || !strings.HasPrefix(seed, "SU") {
		return nil, errors.New("invalid NSC user seed format")
	}

	// 从seed解析nkey
	userKey, err := nkeys.FromSeed([]byte(seed))
	if err != nil {
		return nil, fmt.Errorf("parse nkey from seed: %w", err)
	}

	pubKey, err := userKey.PublicKey()
	if err != nil {
		return nil, fmt.Errorf("get public key: %w", err)
	}

	return &NSCKeyManager{
		userSeed:   seed,
		userPubKey: pubKey,
	}, nil
}

// deriveKeyMaterial 统一的密钥派生函数
// 使用 HKDF-like 方式：SHA256(seed + domain + salt)
func (km *NSCKeyManager) deriveKeyMaterial(domain KeyDomain, salt []byte) ([32]byte, error) {
	// 获取NSC原始seed
	userKey, err := nkeys.FromSeed([]byte(km.userSeed))
	if err != nil {
		return [32]byte{}, fmt.Errorf("parse nkey: %w", err)
	}

	seed, err := userKey.Seed()
	if err != nil {
		return [32]byte{}, fmt.Errorf("get seed: %w", err)
	}

	// 构造派生输入：seed + domain + salt
	input := append(seed, []byte(domain)...)
	if salt != nil {
		input = append(input, salt...)
	}

	// 使用SHA256进行确定性派生
	hash := sha256.Sum256(input)
	return hash, nil
}

// Ed25519PrivateKeyToX25519 将Ed25519私钥转换为X25519私钥
func Ed25519PrivateKeyToX25519(ed25519Priv ed25519.PrivateKey) []byte {
	hash := sha512.Sum512(ed25519Priv[:32])
	hash[0] &= 248
	hash[31] &= 127
	hash[31] |= 64
	return hash[:32]
}

// GetChatKeyPair 获取聊天加密密钥对 (X25519) - 从NSC Ed25519密钥直接转换
func (km *NSCKeyManager) GetChatKeyPair() (privB64, pubB64 string, err error) {
	// 从NSC seed解析Ed25519私钥
	userKey, err := nkeys.FromSeed([]byte(km.userSeed))
	if err != nil {
		return "", "", fmt.Errorf("parse nkey from seed: %w", err)
	}
	// 从seed直接生成标准Ed25519私钥（正确方式）
	seedStr, err := userKey.Seed()
	if err != nil {
		return "", "", fmt.Errorf("get seed: %w", err)
	}
	// 解码NKey seed得到原始32字节Ed25519 seed
	rawSeed, err := nkeys.Decode(nkeys.PrefixByteSeed, []byte(seedStr))
	if err != nil {
		return "", "", fmt.Errorf("decode seed: %w", err)
	}
	// 去掉第一个字节的前缀，后面32字节是真正的seed
	ed25519Priv := ed25519.NewKeyFromSeed(rawSeed[1:])

	// 转换为X25519私钥
	x25519Priv := Ed25519PrivateKeyToX25519(ed25519Priv)

	// 计算X25519公钥
	x25519Pub, err := curve25519.X25519(x25519Priv, curve25519.Basepoint)
	if err != nil {
		return "", "", fmt.Errorf("generate X25519 public key: %w", err)
	}

	return base64.StdEncoding.EncodeToString(x25519Priv),
		base64.StdEncoding.EncodeToString(x25519Pub), nil
}


// GetChatPubKeyFromNSCPub 从NSC公钥派生聊天公钥 - 修复版，和私钥转换逻辑严格配对
func GetChatPubKeyFromNSCPub(nscPubKey string) (string, error) {
	if !strings.HasPrefix(nscPubKey, "U") || len(nscPubKey) < 2 {
		return "", errors.New("invalid NSC user public key format")
	}

	// 1. 解码NSC公钥（nkeys库自动处理base32解码、CRC校验）
	raw, err := nkeys.Decode(nkeys.PrefixByteUser, []byte(nscPubKey))
	if err != nil {
		return "", fmt.Errorf("decode NSC public key: %w", err)
	}

	// 2. raw就是32字节的Ed25519公钥
	ed25519Pub := raw

	// 3. 标准Ed25519公钥转X25519公钥（RFC7748标准）
	p, err := edwards25519.NewIdentityPoint().SetBytes(ed25519Pub)
	if err != nil {
		return "", fmt.Errorf("invalid Ed25519 public key: %w, raw bytes: %x", err, ed25519Pub)
	}
	x25519Pub := p.BytesMontgomery()

	return base64.StdEncoding.EncodeToString(x25519Pub), nil
}


// ValidateNSCSeed 验证NSC seed格式
func ValidateNSCSeed(seed string) error {
	if seed == "" {
		return errors.New("seed is empty")
	}

	if !strings.HasPrefix(seed, "SU") {
		return errors.New("invalid NSC user seed prefix")
	}

	// 尝试解析nkey
	_, err := nkeys.FromSeed([]byte(seed))
	if err != nil {
		return fmt.Errorf("invalid NSC seed: %w", err)
	}

	return nil
}

// GenerateGroupKey 生成群聊对称密钥
func GenerateGroupKey() (string, error) {
	key := make([]byte, 32) // 256-bit key for AES-256
	if _, err := rand.Read(key); err != nil {
		return "", fmt.Errorf("generate group key: %w", err)
	}
	return base64.StdEncoding.EncodeToString(key), nil
}
