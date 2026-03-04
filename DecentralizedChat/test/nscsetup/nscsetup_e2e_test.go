// E2E 集成测试：NSC 简化设置
package e2e_test

import (
	"os"
	"path/filepath"
	"testing"

	"DecentralizedChat/internal/config"
	"DecentralizedChat/internal/nscsetup"

	"github.com/nats-io/jwt/v2"
	"github.com/nats-io/nkeys"
)

func TestNSCSetup_EnsureSimpleSetup_E2E(t *testing.T) {
	t.Log("=== E2E 测试: NSC 简化设置 EnsureSimpleSetup ===")
	t.Log("")

	// 1. 创建临时目录用于测试
	tmpDir := t.TempDir()
	t.Logf("✅ 临时目录: %s", tmpDir)

	// 2. 备份原有的 GetConfigPath 函数，测试后恢复
	origGetConfigPath := config.GetConfigPath
	defer func() { config.GetConfigPath = origGetConfigPath }()

	// 3. 重写 GetConfigPath 以使用临时目录
	testConfigPath := filepath.Join(tmpDir, "config.json")
	config.GetConfigPath = func() (string, error) {
		return testConfigPath, nil
	}

	// 4. 创建测试配置
	cfg := &config.Config{
		User: config.UserConfig{
			Nickname: "TestUser",
		},
		Keys: config.KeysConfig{},
	}

	// 5. 运行 EnsureSimpleSetup
	t.Log("运行 EnsureSimpleSetup...")
	err := nscsetup.EnsureSimpleSetup(cfg)
	if err != nil {
		t.Fatalf("EnsureSimpleSetup failed: %v", err)
	}
	t.Log("✅ EnsureSimpleSetup 成功")

	// 6. 验证配置已更新
	if cfg.Keys.Operator == "" {
		t.Error("cfg.Keys.Operator should not be empty")
	}
	if cfg.Keys.Account == "" {
		t.Error("cfg.Keys.Account should not be empty")
	}
	if cfg.Keys.User == "" {
		t.Error("cfg.Keys.User should not be empty")
	}
	if cfg.Keys.UserCredsPath == "" {
		t.Error("cfg.Keys.UserCredsPath should not be empty")
	}
	if cfg.Keys.UserSeedPath == "" {
		t.Error("cfg.Keys.UserSeedPath should not be empty")
	}
	if cfg.Keys.UserPubKey == "" {
		t.Error("cfg.Keys.UserPubKey should not be empty")
	}
	t.Log("✅ 配置已正确更新")

	// 7. 验证文件已创建
	filesToCheck := []string{
		cfg.Keys.UserCredsPath,
		cfg.Keys.UserSeedPath,
		filepath.Join(tmpDir, "operator.nk"),
		filepath.Join(tmpDir, "account.nk"),
		filepath.Join(tmpDir, "user.nk"),
		filepath.Join(tmpDir, "simple_resolver.conf"),
		testConfigPath,
	}
	for _, f := range filesToCheck {
		if _, err := os.Stat(f); err != nil {
			t.Errorf("文件不存在: %s", f)
		} else {
			t.Logf("✅ 文件已创建: %s", filepath.Base(f))
		}
	}

	// 8. 验证 accounts 目录和 JWT 文件
	accountsDir := filepath.Join(tmpDir, "accounts")
	if _, err := os.Stat(accountsDir); err != nil {
		t.Errorf("accounts 目录不存在: %s", accountsDir)
	} else {
		t.Log("✅ accounts 目录已创建")
	}

	// 9. 再次运行 EnsureSimpleSetup（应该跳过，因为已初始化）
	t.Log("再次运行 EnsureSimpleSetup（应跳过）...")
	err = nscsetup.EnsureSimpleSetup(cfg)
	if err != nil {
		t.Fatalf("第二次 EnsureSimpleSetup failed: %v", err)
	}
	t.Log("✅ 第二次 EnsureSimpleSetup 成功（跳过初始化）")

	t.Log("")
	t.Log("=== EnsureSimpleSetup 测试通过 ✅ ===")
}

func TestNSCSetup_KeyGeneration_E2E(t *testing.T) {
	t.Log("=== E2E 测试: NSC 密钥生成 ===")
	t.Log("")

	tmpDir := t.TempDir()

	// 测试 operator 密钥
	operatorKeyFile := filepath.Join(tmpDir, "operator.nk")
	operatorKey, err := nscsetup.LoadOrGenerateOperatorKey(operatorKeyFile)
	if err != nil {
		t.Fatalf("LoadOrGenerateOperatorKey failed: %v", err)
	}
	if operatorKey == nil {
		t.Fatal("operatorKey is nil")
	}
	operatorPub, err := operatorKey.PublicKey()
	if err != nil {
		t.Fatalf("operatorKey.PublicKey failed: %v", err)
	}
	if operatorPub == "" || !nkeys.IsValidPublicOperatorKey(operatorPub) {
		t.Errorf("无效的 operator public key: %s", operatorPub)
	}
	t.Logf("✅ Operator 密钥生成成功: %s", operatorPub)

	// 测试 account 密钥
	accountKeyFile := filepath.Join(tmpDir, "account.nk")
	accountKey, err := nscsetup.LoadOrGenerateAccountKey(accountKeyFile)
	if err != nil {
		t.Fatalf("LoadOrGenerateAccountKey failed: %v", err)
	}
	if accountKey == nil {
		t.Fatal("accountKey is nil")
	}
	accountPub, err := accountKey.PublicKey()
	if err != nil {
		t.Fatalf("accountKey.PublicKey failed: %v", err)
	}
	if accountPub == "" || !nkeys.IsValidPublicAccountKey(accountPub) {
		t.Errorf("无效的 account public key: %s", accountPub)
	}
	t.Logf("✅ Account 密钥生成成功: %s", accountPub)

	// 测试 user 密钥
	userKeyFile := filepath.Join(tmpDir, "user.nk")
	userKey, err := nscsetup.LoadOrGenerateUserKey(userKeyFile)
	if err != nil {
		t.Fatalf("LoadOrGenerateUserKey failed: %v", err)
	}
	if userKey == nil {
		t.Fatal("userKey is nil")
	}
	userPub, err := userKey.PublicKey()
	if err != nil {
		t.Fatalf("userKey.PublicKey failed: %v", err)
	}
	if userPub == "" || !nkeys.IsValidPublicUserKey(userPub) {
		t.Errorf("无效的 user public key: %s", userPub)
	}
	t.Logf("✅ User 密钥生成成功: %s", userPub)

	// 测试从文件加载密钥（应该和之前的一样）
	t.Log("从文件重新加载密钥...")
	loadedOperatorKey, err := nscsetup.LoadOrGenerateOperatorKey(operatorKeyFile)
	if err != nil {
		t.Fatalf("重新加载 operator key 失败: %v", err)
	}
	loadedOperatorPub, _ := loadedOperatorKey.PublicKey()
	if loadedOperatorPub != operatorPub {
		t.Error("重新加载的 operator key 不匹配")
	}
	t.Log("✅ 密钥重新加载成功")

	t.Log("")
	t.Log("=== 密钥生成测试通过 ✅ ===")
}

func TestNSCSetup_JWTGeneration_E2E(t *testing.T) {
	t.Log("=== E2E 测试: NSC JWT 生成 ===")
	t.Log("")

	tmpDir := t.TempDir()

	// 创建密钥
	operatorKey, _ := nkeys.CreateOperator()
	accountKey, _ := nkeys.CreateAccount()
	userKey, _ := nkeys.CreateUser()

	setup := &nscsetup.SimpleSetup{
		OperatorKey: operatorKey,
		AccountKey:  accountKey,
		UserKey:     userKey,
	}

	// 生成 JWT
	cfg := &config.Config{
		User: config.UserConfig{
			Nickname: "JTWTestUser",
		},
		Keys: config.KeysConfig{
			Operator: "test-operator",
			Account:  "test-account",
			User:     "test-user",
		},
	}

	err := setup.GenerateJWTs(cfg)
	if err != nil {
		t.Fatalf("GenerateJWTs failed: %v", err)
	}
	t.Log("✅ JWT 生成成功")

	// 验证 Operator JWT
	if setup.OperatorJWT == "" {
		t.Error("OperatorJWT is empty")
	} else {
		operatorPub, _ := operatorKey.PublicKey()
		claims, err := jwt.DecodeOperatorClaims(setup.OperatorJWT)
		if err != nil {
			t.Errorf("DecodeOperatorClaims failed: %v", err)
		} else {
			if claims.Subject != operatorPub {
				t.Error("Operator JWT subject mismatch")
			}
			if claims.Name != "test-operator" {
				t.Error("Operator JWT name mismatch")
			}
			t.Log("✅ Operator JWT 验证成功")
		}
	}

	// 验证 Account JWT
	if setup.AccountJWT == "" {
		t.Error("AccountJWT is empty")
	} else {
		accountPub, _ := accountKey.PublicKey()
		claims, err := jwt.DecodeAccountClaims(setup.AccountJWT)
		if err != nil {
			t.Errorf("DecodeAccountClaims failed: %v", err)
		} else {
			if claims.Subject != accountPub {
				t.Error("Account JWT subject mismatch")
			}
			if claims.Name != "test-account" {
				t.Error("Account JWT name mismatch")
			}
			if claims.Limits.Conn != -1 {
				t.Error("Account JWT Conn limit should be -1")
			}
			if claims.Limits.Subs != -1 {
				t.Error("Account JWT Subs limit should be -1")
			}
			t.Log("✅ Account JWT 验证成功")
		}
	}

	// 验证 User JWT
	if setup.UserJWT == "" {
		t.Error("UserJWT is empty")
	} else {
		userPub, _ := userKey.PublicKey()
		claims, err := jwt.DecodeUserClaims(setup.UserJWT)
		if err != nil {
			t.Errorf("DecodeUserClaims failed: %v", err)
		} else {
			if claims.Subject != userPub {
				t.Error("User JWT subject mismatch")
			}
			if claims.Name != "test-user" {
				t.Error("User JWT name mismatch")
			}
			if len(claims.Pub.Allow) < 2 {
				t.Error("User JWT Pub.Allow should have at least 2 entries")
			}
			if len(claims.Sub.Allow) < 2 {
				t.Error("User JWT Sub.Allow should have at least 2 entries")
			}
			t.Log("✅ User JWT 验证成功")
		}
	}

	// 测试生成 creds 文件
	credsPath := filepath.Join(tmpDir, "user.creds")
	err = setup.GenerateCreds(credsPath)
	if err != nil {
		t.Fatalf("GenerateCreds failed: %v", err)
	}
	if _, err := os.Stat(credsPath); err != nil {
		t.Error("creds file not created")
	}
	t.Log("✅ Creds 文件生成成功")

	// 测试生成 resolver 配置
	resolverPath := filepath.Join(tmpDir, "resolver.conf")
	err = setup.GenerateResolverConfig(resolverPath)
	if err != nil {
		t.Fatalf("GenerateResolverConfig failed: %v", err)
	}
	if _, err := os.Stat(resolverPath); err != nil {
		t.Error("resolver config not created")
	}
	t.Log("✅ Resolver 配置生成成功")

	t.Log("")
	t.Log("=== JWT 生成测试通过 ✅ ===")
}
