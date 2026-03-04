package nscsetup

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"DecentralizedChat/internal/config"

	"github.com/nats-io/jwt/v2"
	"github.com/nats-io/nkeys"
)

// SimpleSetup 简化版NATS设置，无需NSC CLI
type SimpleSetup struct {
	OperatorKey nkeys.KeyPair
	AccountKey  nkeys.KeyPair
	UserKey     nkeys.KeyPair

	OperatorJWT string
	AccountJWT  string
	UserJWT     string
}

// EnsureSimpleSetup 简化版初始化：直接使用Go NATS库，无需NSC CLI
func EnsureSimpleSetup(cfg *config.Config) error {
	// 检查是否已初始化（通过 UserCredsPath 判断）
	if cfg.Keys.UserCredsPath != "" {
		if _, err := os.Stat(cfg.Keys.UserCredsPath); err == nil {
			return nil // 已初始化
		}
	}

	confPath, err := config.GetConfigPath()
	if err != nil {
		return err
	}
	confDir := filepath.Dir(confPath)

	// 确保配置有默认值
	if cfg.Keys.Operator == "" {
		cfg.Keys.Operator = "dchat"
	}
	if cfg.Keys.Account == "" {
		cfg.Keys.Account = "USERS"
	}
	if cfg.Keys.User == "" {
		cfg.Keys.User = cfg.User.Nickname
	}

	// 1. 生成或加载密钥对
	setup := &SimpleSetup{}
	if err := setup.EnsureKeys(confDir); err != nil {
		return fmt.Errorf("ensure keys: %w", err)
	}

	// 2. 生成JWT链
	if err := setup.GenerateJWTs(cfg); err != nil {
		return fmt.Errorf("generate JWTs: %w", err)
	}

	// 3. 生成resolver配置
	resolverPath := filepath.Join(confDir, "simple_resolver.conf")
	if err := setup.GenerateResolverConfig(resolverPath); err != nil {
		return fmt.Errorf("generate resolver config: %w", err)
	}

	// 4. 生成creds文件
	credsPath := filepath.Join(confDir, "user.creds")
	if err := setup.GenerateCreds(credsPath); err != nil {
		return fmt.Errorf("generate creds: %w", err)
	}

	// 5. 保存用户seed
	userSeedPath := filepath.Join(confDir, "user.seed")
	userSeed, _ := setup.UserKey.Seed()
	if err := os.WriteFile(userSeedPath, userSeed, 0600); err != nil {
		return fmt.Errorf("save user seed: %w", err)
	}

	// 6. 更新配置
	userPub, _ := setup.UserKey.PublicKey()

	// 设置默认值（如果配置为空）
	if cfg.Keys.Operator == "" {
		cfg.Keys.Operator = "dchat"
	}
	if cfg.Keys.Account == "" {
		cfg.Keys.Account = "USERS"
	}
	if cfg.Keys.User == "" {
		cfg.Keys.User = cfg.User.Nickname
	}

	cfg.Keys.KeysDir = confDir
	cfg.Keys.UserCredsPath = credsPath
	cfg.Keys.UserSeedPath = userSeedPath
	cfg.Keys.UserPubKey = userPub

	return config.SaveConfig(cfg)
}

// EnsureKeys 生成或加载密钥对
func (s *SimpleSetup) EnsureKeys(confDir string) error {
	var err error

	// 操作者密钥
	operatorKeyFile := filepath.Join(confDir, "operator.nk")
	s.OperatorKey, err = LoadOrGenerateOperatorKey(operatorKeyFile)
	if err != nil {
		return fmt.Errorf("operator key: %w", err)
	}

	// 账户密钥
	accountKeyFile := filepath.Join(confDir, "account.nk")
	s.AccountKey, err = LoadOrGenerateAccountKey(accountKeyFile)
	if err != nil {
		return fmt.Errorf("account key: %w", err)
	}

	// 用户密钥
	userKeyFile := filepath.Join(confDir, "user.nk")
	s.UserKey, err = LoadOrGenerateUserKey(userKeyFile)
	if err != nil {
		return fmt.Errorf("user key: %w", err)
	}

	return nil
}

// GenerateJWTs 生成JWT链
func (s *SimpleSetup) GenerateJWTs(cfg *config.Config) error {
	now := time.Now()

	// 1. 操作者JWT
	operatorPub, _ := s.OperatorKey.PublicKey()
	operatorClaims := jwt.NewOperatorClaims(operatorPub)
	operatorClaims.Name = cfg.Keys.Operator // 使用配置中的值
	operatorClaims.IssuedAt = now.Unix()

	// 设置系统账户
	accountPub, _ := s.AccountKey.PublicKey()
	operatorClaims.SystemAccount = accountPub

	operatorJWT, err := operatorClaims.Encode(s.OperatorKey)
	if err != nil {
		return fmt.Errorf("encode operator JWT: %w", err)
	}
	s.OperatorJWT = operatorJWT

	// 2. 账户JWT
	accountClaims := jwt.NewAccountClaims(accountPub)
	accountClaims.Name = cfg.Keys.Account // 使用配置中的值
	accountClaims.IssuedAt = now.Unix()
	accountClaims.Issuer = operatorPub

	// 设置基本权限
	accountClaims.Limits.Conn = -1 // 无限连接
	accountClaims.Limits.Subs = -1 // 无限订阅

	accountJWT, err := accountClaims.Encode(s.OperatorKey)
	if err != nil {
		return fmt.Errorf("encode account JWT: %w", err)
	}
	s.AccountJWT = accountJWT

	// 3. 用户JWT
	userPub, _ := s.UserKey.PublicKey()
	userClaims := jwt.NewUserClaims(userPub)
	userClaims.Name = cfg.Keys.User // 使用配置中的值
	userClaims.IssuedAt = now.Unix()
	userClaims.Issuer = accountPub

	// 设置聊天权限 - 使用操作者名称作为主题前缀
	subjectPrefix := cfg.Keys.Operator + ".>"
	userClaims.Pub.Allow = []string{subjectPrefix, "_INBOX.>"}
	userClaims.Sub.Allow = []string{subjectPrefix, "_INBOX.>"}

	userJWT, err := userClaims.Encode(s.AccountKey)
	if err != nil {
		return fmt.Errorf("encode user JWT: %w", err)
	}
	s.UserJWT = userJWT

	return nil
}

// GenerateResolverConfig 生成NATS配置文件，指向accounts目录
func (s *SimpleSetup) GenerateResolverConfig(resolverPath string) error {
	accountPub, _ := s.AccountKey.PublicKey()

	// 创建accounts目录
	confDir := filepath.Dir(resolverPath)
	accountsDir := filepath.Join(confDir, "accounts")
	if err := os.MkdirAll(accountsDir, 0755); err != nil {
		return fmt.Errorf("create accounts dir: %w", err)
	}

	// 写入账户JWT文件
	accountFile := filepath.Join(accountsDir, accountPub+".jwt")
	if err := os.WriteFile(accountFile, []byte(s.AccountJWT), 0644); err != nil {
		return fmt.Errorf("write account JWT: %w", err)
	}

	// 生成NATS配置文件
	natsConfig := fmt.Sprintf(`# NATS Configuration with JWT Resolver
resolver {
  type: full
  dir: %q
}

# System Account
system_account: %q

# JWT-based authentication
operator: %q
`, accountsDir, accountPub, s.OperatorJWT)

	return os.WriteFile(resolverPath, []byte(natsConfig), 0644)
}

// GenerateCreds 生成creds文件
func (s *SimpleSetup) GenerateCreds(credsPath string) error {
	userSeed, _ := s.UserKey.Seed()

	creds := fmt.Sprintf(`-----BEGIN NATS USER JWT-----
%s
------END NATS USER JWT------

************************* IMPORTANT *************************
NKEY Seed printed below can be used to sign and prove identity.
NKEYs are sensitive and should be treated as secrets.

-----BEGIN USER NKEY SEED-----
%s
------END USER NKEY SEED------

*************************************************************
`, s.UserJWT, string(userSeed))

	return os.WriteFile(credsPath, []byte(creds), 0600)
}

// 密钥生成辅助函数
func LoadOrGenerateOperatorKey(filename string) (nkeys.KeyPair, error) {
	if data, err := os.ReadFile(filename); err == nil {
		if key, err := nkeys.FromSeed(data); err == nil {
			return key, nil
		}
	}

	key, err := nkeys.CreateOperator()
	if err != nil {
		return nil, err
	}

	seed, err := key.Seed()
	if err != nil {
		return nil, err
	}

	if err := os.WriteFile(filename, seed, 0600); err != nil {
		return nil, err
	}

	return key, nil
}

func LoadOrGenerateAccountKey(filename string) (nkeys.KeyPair, error) {
	if data, err := os.ReadFile(filename); err == nil {
		if key, err := nkeys.FromSeed(data); err == nil {
			return key, nil
		}
	}

	key, err := nkeys.CreateAccount()
	if err != nil {
		return nil, err
	}

	seed, err := key.Seed()
	if err != nil {
		return nil, err
	}

	if err := os.WriteFile(filename, seed, 0600); err != nil {
		return nil, err
	}

	return key, nil
}

func LoadOrGenerateUserKey(filename string) (nkeys.KeyPair, error) {
	if data, err := os.ReadFile(filename); err == nil {
		if key, err := nkeys.FromSeed(data); err == nil {
			return key, nil
		}
	}

	key, err := nkeys.CreateUser()
	if err != nil {
		return nil, err
	}

	seed, err := key.Seed()
	if err != nil {
		return nil, err
	}

	if err := os.WriteFile(filename, seed, 0600); err != nil {
		return nil, err
	}

	return key, nil
}
