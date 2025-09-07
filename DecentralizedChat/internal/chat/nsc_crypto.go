package chat

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"github.com/nats-io/nkeys"
	"golang.org/x/crypto/curve25519"
)

// NSCKeyManager 处理NSC密钥和聊天加密的转换
type NSCKeyManager struct {
	userSeed   string // NSC用户seed
	userPubKey string // NSC用户公钥
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

// GetChatKeyPair 从NSC密钥派生聊天加密密钥对 (X25519)
func (km *NSCKeyManager) GetChatKeyPair() (privB64, pubB64 string, err error) {
	// 从NSC seed派生Ed25519私钥
	userKey, err := nkeys.FromSeed([]byte(km.userSeed))
	if err != nil {
		return "", "", fmt.Errorf("parse nkey: %w", err)
	}

	// 获取Ed25519私钥的原始字节
	seed, err := userKey.Seed()
	if err != nil {
		return "", "", fmt.Errorf("get seed: %w", err)
	}

	// 使用seed的SHA256作为X25519私钥的源
	// 这确保了从同一个NSC seed总是生成相同的X25519密钥对
	hash := sha256.Sum256(seed)

	// 将前32字节作为X25519私钥
	var x25519Priv [32]byte
	copy(x25519Priv[:], hash[:])

	// 计算对应的X25519公钥
	x25519Pub, err := curve25519.X25519(x25519Priv[:], curve25519.Basepoint)
	if err != nil {
		return "", "", fmt.Errorf("generate X25519 public key: %w", err)
	}

	return base64.StdEncoding.EncodeToString(x25519Priv[:]),
		base64.StdEncoding.EncodeToString(x25519Pub), nil
}

// GetChatPubKeyFromNSCPub 从NSC公钥派生聊天公钥
// 这个函数用于获取其他用户的聊天公钥（只有NSC公钥的情况）
func GetChatPubKeyFromNSCPub(nscPubKey string) (string, error) {
	if !strings.HasPrefix(nscPubKey, "U") {
		return "", errors.New("invalid NSC user public key format")
	}

	// 验证NSC公钥格式
	_, err := nkeys.FromPublicKey(nscPubKey)
	if err != nil {
		return "", fmt.Errorf("invalid NSC public key: %w", err)
	}

	// 对于公钥，我们使用确定性派生方法
	// 使用公钥字符串的SHA256作为X25519公钥的种子
	hash := sha256.Sum256([]byte(nscPubKey))

	// 我们不能直接从Ed25519公钥计算X25519公钥
	// 所以这里使用一个确定性的派生方法
	// 注意：这种方法要求对方也使用相同的派生逻辑

	// 使用hash作为X25519私钥，计算对应公钥
	x25519Pub, err := curve25519.X25519(hash[:], curve25519.Basepoint)
	if err != nil {
		return "", fmt.Errorf("derive X25519 public key: %w", err)
	}

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
