package nscsetup

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"DecentralizedChat/internal/config"

	"github.com/nats-io/nkeys"
)

// EnsureSysAccountSetup 在首次运行时：
// 1) 初始化本地 nsc operator（带 sys）并启用签名密钥与账号解析 URL
// 2) 生成 resolver.conf 写入到 ~/.dchat/resolver.conf 并回写到配置
// 3) 采集并持久化 SYS 账户的 JWT 与密钥路径（将公钥写入 ~/.dchat/sys.pub，以路径形式保存）
func EnsureSysAccountSetup(cfg *config.Config) error {
	// 如果已经配置了 resolver.conf，认为已完成初始化
	if cfg.Routes.ResolverConfig != "" {
		return nil
	}

	// 获取配置目录
	confPath, err := config.GetConfigPath()
	if err != nil {
		return err
	}
	confDir := filepath.Dir(confPath)

	// 设置 NATS URL（账号解析 URL）
	natsURL := cfg.NATS.URL
	if natsURL == "" {
		host := cfg.Routes.Host
		if host == "" {
			host = cfg.Network.LocalIP
		}
		if cfg.Routes.ClientPort == 0 {
			cfg.Routes.ClientPort = 4222
		}
		natsURL = fmt.Sprintf("nats://%s:%d", host, cfg.Routes.ClientPort)
		cfg.NATS.URL = natsURL
	}

	// 执行 nsc 初始化流程（幂等）
	_ = run("nsc", "add", "operator", "--generate-signing-key", "--sys", "--name", "local")
	_ = run("nsc", "edit", "operator", "--require-signing-keys", "--account-jwt-server-url", natsURL)
	_ = run("nsc", "edit", "account", "SYS", "--sk", "generate")

	// 生成 resolver.conf 内容
	resolverOut, err := runOut("nsc", "generate", "config", "--nats-resolver", "--sys-account", "SYS")
	if err != nil {
		return fmt.Errorf("nsc generate config 失败: %w", err)
	}
	resolverPath := filepath.Join(confDir, "resolver.conf")
	if err := os.WriteFile(resolverPath, resolverOut, 0644); err != nil {
		return fmt.Errorf("写入 resolver.conf 失败: %w", err)
	}
	cfg.Routes.ResolverConfig = resolverPath

	// 获取 nsc 环境信息（JSON）以获得存储目录
	envJSON, _ := runOut("nsc", "env", "-J")
	var env map[string]any
	_ = json.Unmarshal(envJSON, &env)
	var keysDir, storeDir string
	if v, ok := env["KeysDir"].(string); ok {
		keysDir = v
	}
	if v, ok := env["StoreRoot"].(string); ok {
		storeDir = v
	}

	// 描述 SYS 账户，尽量拿到 JWT 路径与公钥
	sysDescJSON, jerr := runOut("nsc", "describe", "account", "SYS", "-J")
	var sysJWTPath, sysPubKey string
	if jerr == nil {
		var desc map[string]any
		if err := json.Unmarshal(sysDescJSON, &desc); err == nil {
			if jwtObj, ok := desc["jwt"].(map[string]any); ok {
				if p, ok := jwtObj["path"].(string); ok {
					sysJWTPath = p
				}
			}
			if pk, ok := desc["public_key"].(string); ok {
				sysPubKey = pk
			}
		}
	}
	if sysJWTPath == "" {
		sysDescText, _ := runOut("nsc", "describe", "account", "SYS")
		sysJWTPath = firstMatch(string(sysDescText), `JWT\s+file:\s+(.+)`)
		if sysPubKey == "" {
			sysPubKey = firstMatch(string(sysDescText), `Public\s+key:\s+([A-Z0-9]+)`)
		}
	}

	// 将公钥写入文件，便于"路径"持久化
	var sysPubPath string
	if sysPubKey != "" {
		sysPubPath = filepath.Join(confDir, "sys.pub")
		_ = os.WriteFile(sysPubPath, []byte(sysPubKey+"\n"), 0644)
	}

	// 尝试定位 SYS 账户的私钥种子文件（在 KeysDir 下扫描并匹配公钥）
	var sysSeedPath string
	if keysDir != "" && sysPubKey != "" {
		if p, _ := findSeedByPublicKey(keysDir, sysPubKey); p != "" {
			sysSeedPath = p
		}
	}

	// 将路径写回配置（NSC 子配置）
	cfg.NSC.Operator = "local"
	cfg.NSC.StoreDir = storeDir
	cfg.NSC.KeysDir = keysDir
	cfg.NSC.SysAccountJWT = sysJWTPath
	cfg.NSC.SysSeedPath = sysSeedPath
	cfg.NSC.SysPubPath = sysPubPath

	// 保存配置
	if err := config.SaveConfig(cfg); err != nil {
		return fmt.Errorf("保存配置失败: %w", err)
	}
	return nil
}

func run(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	var out bytes.Buffer
	var errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	cmd.Env = os.Environ()
	cmd.Env = append(cmd.Env, "NSC_NO_GITHUB=1")
	cmd.Env = append(cmd.Env, "NO_COLOR=1")
	cmd.WaitDelay = 10 * time.Second
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(errb.String()))
	}
	return nil
}

func runOut(name string, args ...string) ([]byte, error) {
	cmd := exec.Command(name, args...)
	var out bytes.Buffer
	var errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	cmd.Env = os.Environ()
	cmd.Env = append(cmd.Env, "NSC_NO_GITHUB=1")
	cmd.Env = append(cmd.Env, "NO_COLOR=1")
	cmd.WaitDelay = 10 * time.Second
	if err := cmd.Run(); err != nil {
		return nil, errors.New(strings.TrimSpace(errb.String()))
	}
	return out.Bytes(), nil
}

func firstMatch(s, pattern string) string {
	re := regexp.MustCompile(pattern)
	m := re.FindStringSubmatch(s)
	if len(m) >= 2 {
		return strings.TrimSpace(m[1])
	}
	return ""
}

// findSeedByPublicKey 遍历 keysDir 寻找与 pubKey 匹配的种子文件路径。
func findSeedByPublicKey(keysDir, pubKey string) (string, error) {
	var matched string
	_ = filepath.WalkDir(keysDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d == nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		// 仅尝试可能的种子文件：文件名以 "S" 开头
		base := filepath.Base(path)
		if !strings.HasPrefix(base, "S") {
			return nil
		}
		// 读取少量内容（种子通常很短）
		b, rerr := os.ReadFile(path)
		if rerr != nil {
			return nil
		}
		seed := strings.TrimSpace(string(b))
		if seed == "" {
			return nil
		}
		// 解析 seed 并比较公钥
		kp, kerr := nkeys.FromSeed([]byte(seed))
		if kerr != nil {
			return nil
		}
		pk, perr := kp.PublicKey()
		if perr != nil {
			return nil
		}
		if pk == pubKey {
			matched = path
			// 终止遍历
			return errors.New("found")
		}
		return nil
	})
	if matched != "" {
		return matched, nil
	}
	return "", nil
}
