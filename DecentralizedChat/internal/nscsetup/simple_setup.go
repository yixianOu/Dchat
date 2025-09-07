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
	operatorKey nkeys.KeyPair
	accountKey  nkeys.KeyPair
	userKey     nkeys.KeyPair

	operatorJWT string
	accountJWT  string
	userJWT     string
}

// EnsureSimpleSetup 简化版初始化：直接使用Go NATS库，无需NSC CLI
func EnsureSimpleSetup(cfg *config.Config) error {
	if cfg.Server.ResolverConf != "" {
		return nil // 已初始化
	}

	confPath, err := config.GetConfigPath()
	if err != nil {
		return err
	}
	confDir := filepath.Dir(confPath)

	// 1. 生成或加载密钥对
	setup := &SimpleSetup{}
	if err := setup.ensureKeys(confDir); err != nil {
		return fmt.Errorf("ensure keys: %w", err)
	}

	// 2. 生成JWT链
	if err := setup.generateJWTs(); err != nil {
		return fmt.Errorf("generate JWTs: %w", err)
	}

	// 3. 生成resolver配置
	resolverPath := filepath.Join(confDir, "simple_resolver.conf")
	if err := setup.generateResolverConfig(resolverPath); err != nil {
		return fmt.Errorf("generate resolver config: %w", err)
	}

	// 4. 生成creds文件
	credsPath := filepath.Join(confDir, "user.creds")
	if err := setup.generateCreds(credsPath); err != nil {
		return fmt.Errorf("generate creds: %w", err)
	}

	// 5. 保存用户seed
	userSeedPath := filepath.Join(confDir, "user.seed")
	userSeed, _ := setup.userKey.Seed()
	if err := os.WriteFile(userSeedPath, userSeed, 0600); err != nil {
		return fmt.Errorf("save user seed: %w", err)
	}

	// 6. 更新配置
	userPub, _ := setup.userKey.PublicKey()
	cfg.Keys.Operator = "dchat"
	cfg.Keys.Account = "USERS"
	cfg.Keys.User = cfg.User.Nickname // 使用用户昵称而不是硬编码的"default"
	cfg.Keys.KeysDir = confDir
	cfg.Keys.UserCredsPath = credsPath
	cfg.Keys.UserSeedPath = userSeedPath
	cfg.Keys.UserPubKey = userPub
	cfg.Server.ResolverConf = resolverPath

	return config.SaveConfig(cfg)
}

// ensureKeys 生成或加载密钥对
func (s *SimpleSetup) ensureKeys(confDir string) error {
	var err error

	// 操作者密钥
	operatorKeyFile := filepath.Join(confDir, "operator.nk")
	s.operatorKey, err = loadOrGenerateOperatorKey(operatorKeyFile)
	if err != nil {
		return fmt.Errorf("operator key: %w", err)
	}

	// 账户密钥
	accountKeyFile := filepath.Join(confDir, "account.nk")
	s.accountKey, err = loadOrGenerateAccountKey(accountKeyFile)
	if err != nil {
		return fmt.Errorf("account key: %w", err)
	}

	// 用户密钥
	userKeyFile := filepath.Join(confDir, "user.nk")
	s.userKey, err = loadOrGenerateUserKey(userKeyFile)
	if err != nil {
		return fmt.Errorf("user key: %w", err)
	}

	return nil
}

// generateJWTs 生成JWT链
func (s *SimpleSetup) generateJWTs() error {
	now := time.Now()

	// 1. 操作者JWT
	operatorPub, _ := s.operatorKey.PublicKey()
	operatorClaims := jwt.NewOperatorClaims(operatorPub)
	operatorClaims.Name = "dchat"
	operatorClaims.IssuedAt = now.Unix()

	// 设置系统账户
	accountPub, _ := s.accountKey.PublicKey()
	operatorClaims.SystemAccount = accountPub

	operatorJWT, err := operatorClaims.Encode(s.operatorKey)
	if err != nil {
		return fmt.Errorf("encode operator JWT: %w", err)
	}
	s.operatorJWT = operatorJWT

	// 2. 账户JWT
	accountClaims := jwt.NewAccountClaims(accountPub)
	accountClaims.Name = "USERS"
	accountClaims.IssuedAt = now.Unix()
	accountClaims.Issuer = operatorPub

	// 设置基本权限
	accountClaims.Limits.Conn = -1 // 无限连接
	accountClaims.Limits.Subs = -1 // 无限订阅

	accountJWT, err := accountClaims.Encode(s.operatorKey)
	if err != nil {
		return fmt.Errorf("encode account JWT: %w", err)
	}
	s.accountJWT = accountJWT

	// 3. 用户JWT
	userPub, _ := s.userKey.PublicKey()
	userClaims := jwt.NewUserClaims(userPub)
	userClaims.Name = "default"
	userClaims.IssuedAt = now.Unix()
	userClaims.Issuer = accountPub

	// 设置聊天权限
	userClaims.Pub.Allow = []string{"dchat.>", "_INBOX.>"}
	userClaims.Sub.Allow = []string{"dchat.>", "_INBOX.>"}

	userJWT, err := userClaims.Encode(s.accountKey)
	if err != nil {
		return fmt.Errorf("encode user JWT: %w", err)
	}
	s.userJWT = userJWT

	return nil
}

// generateResolverConfig 生成NATS配置文件，指向accounts目录
func (s *SimpleSetup) generateResolverConfig(resolverPath string) error {
	accountPub, _ := s.accountKey.PublicKey()

	// 创建accounts目录
	confDir := filepath.Dir(resolverPath)
	accountsDir := filepath.Join(confDir, "accounts")
	if err := os.MkdirAll(accountsDir, 0755); err != nil {
		return fmt.Errorf("create accounts dir: %w", err)
	}

	// 写入账户JWT文件
	accountFile := filepath.Join(accountsDir, accountPub+".jwt")
	if err := os.WriteFile(accountFile, []byte(s.accountJWT), 0644); err != nil {
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
`, accountsDir, accountPub, s.operatorJWT)

	return os.WriteFile(resolverPath, []byte(natsConfig), 0644)
}

// generateCreds 生成creds文件
func (s *SimpleSetup) generateCreds(credsPath string) error {
	userSeed, _ := s.userKey.Seed()

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
`, s.userJWT, string(userSeed))

	return os.WriteFile(credsPath, []byte(creds), 0600)
}

// 密钥生成辅助函数
func loadOrGenerateOperatorKey(filename string) (nkeys.KeyPair, error) {
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

func loadOrGenerateAccountKey(filename string) (nkeys.KeyPair, error) {
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

func loadOrGenerateUserKey(filename string) (nkeys.KeyPair, error) {
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
