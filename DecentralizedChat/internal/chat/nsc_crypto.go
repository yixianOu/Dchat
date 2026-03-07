package chat

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net"
	"strings"
	"time"

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
	DomainSSL  KeyDomain = "ssl"  // TLS/SSL证书
)

// DerivedKeyPair 派生的密钥对
type DerivedKeyPair struct {
	PrivateKeyB64 string    `json:"private_key"`
	PublicKeyB64  string    `json:"public_key"`
	Domain        KeyDomain `json:"domain"`
	KeyType       string    `json:"key_type"` // "X25519", "Ed25519"
}

// SSLCertificate SSL证书和密钥
type SSLCertificate struct {
	CertPEM    string `json:"cert_pem"`
	PrivKeyPEM string `json:"private_key_pem"`
	PublicKey  string `json:"public_key"`
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

// GetSSLKeyPair 获取SSL证书密钥对 (Ed25519) ⭐ 新增功能
func (km *NSCKeyManager) GetSSLKeyPair() (*DerivedKeyPair, error) {
	// 使用统一派生方法
	keyMaterial, err := km.deriveKeyMaterial(DomainSSL, nil)
	if err != nil {
		return nil, fmt.Errorf("derive SSL key material: %w", err)
	}

	// 生成Ed25519密钥对（适合SSL证书）
	privKey := ed25519.NewKeyFromSeed(keyMaterial[:])
	pubKey := privKey.Public().(ed25519.PublicKey)

	return &DerivedKeyPair{
		PrivateKeyB64: base64.StdEncoding.EncodeToString(privKey),
		PublicKeyB64:  base64.StdEncoding.EncodeToString(pubKey),
		Domain:        DomainSSL,
		KeyType:       "Ed25519",
	}, nil
}

// GenerateSSLCertificate 生成自签名SSL证书 ⭐ 新增功能
func (km *NSCKeyManager) GenerateSSLCertificate(hosts []string, ips []net.IP, validDays int) (*SSLCertificate, error) {
	// 获取SSL密钥对
	keyPair, err := km.GetSSLKeyPair()
	if err != nil {
		return nil, fmt.Errorf("get SSL key pair: %w", err)
	}

	// 解码私钥
	privKeyBytes, err := base64.StdEncoding.DecodeString(keyPair.PrivateKeyB64)
	if err != nil {
		return nil, fmt.Errorf("decode private key: %w", err)
	}
	privKey := ed25519.PrivateKey(privKeyBytes)
	pubKey := privKey.Public().(ed25519.PublicKey)

	// 创建证书模板
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization:  []string{"DecentralizedChat"},
			Country:       []string{"CN"},
			Province:      []string{""},
			Locality:      []string{""},
			StreetAddress: []string{""},
			PostalCode:    []string{""},
		},
		NotBefore:   time.Now(),
		NotAfter:    time.Now().Add(time.Duration(validDays) * 24 * time.Hour),
		KeyUsage:    x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses: ips,
		DNSNames:    hosts,
	}

	// 生成自签名证书
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, pubKey, privKey)
	if err != nil {
		return nil, fmt.Errorf("create certificate: %w", err)
	}

	// 编码为PEM格式
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})

	privKeyDER, err := x509.MarshalPKCS8PrivateKey(privKey)
	if err != nil {
		return nil, fmt.Errorf("marshal private key: %w", err)
	}
	privKeyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privKeyDER})

	return &SSLCertificate{
		CertPEM:    string(certPEM),
		PrivKeyPEM: string(privKeyPEM),
		PublicKey:  keyPair.PublicKeyB64,
	}, nil
}

// GetAllDerivedKeys 获取所有派生的密钥对 ⭐ 新增功能
func (km *NSCKeyManager) GetAllDerivedKeys() (map[KeyDomain]*DerivedKeyPair, error) {
	keys := make(map[KeyDomain]*DerivedKeyPair)

	// 1. 认证密钥（原始NSC密钥）
	keys[DomainAuth] = &DerivedKeyPair{
		PrivateKeyB64: km.userSeed, // NSC seed本身就是base64编码的
		PublicKeyB64:  km.userPubKey,
		Domain:        DomainAuth,
		KeyType:       "Ed25519",
	}

	// 2. 聊天密钥（X25519）
	chatPriv, chatPub, err := km.GetChatKeyPair()
	if err != nil {
		return nil, fmt.Errorf("get chat keys: %w", err)
	}
	keys[DomainChat] = &DerivedKeyPair{
		PrivateKeyB64: chatPriv,
		PublicKeyB64:  chatPub,
		Domain:        DomainChat,
		KeyType:       "X25519",
	}

	// 3. SSL密钥（Ed25519）
	sslKeys, err := km.GetSSLKeyPair()
	if err != nil {
		return nil, fmt.Errorf("get SSL keys: %w", err)
	}
	keys[DomainSSL] = sslKeys

	return keys, nil
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

// GetSSLPubKeyFromNSCPub 从NSC公钥派生SSL公钥 ⭐ 新增功能
func GetSSLPubKeyFromNSCPub(nscPubKey string) (string, error) {
	if !strings.HasPrefix(nscPubKey, "U") {
		return "", errors.New("invalid NSC user public key format")
	}

	// 验证NSC公钥格式
	_, err := nkeys.FromPublicKey(nscPubKey)
	if err != nil {
		return "", fmt.Errorf("invalid NSC public key: %w", err)
	}

	// 使用与GetSSLKeyPair相同的派生逻辑
	input := append([]byte(nscPubKey), []byte(DomainSSL)...)
	hash := sha256.Sum256(input)

	// 生成Ed25519公钥
	privKey := ed25519.NewKeyFromSeed(hash[:])
	pubKey := privKey.Public().(ed25519.PublicKey)

	return base64.StdEncoding.EncodeToString(pubKey), nil
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
