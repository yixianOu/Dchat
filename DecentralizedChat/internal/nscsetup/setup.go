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

// EnsureSysAccountSetup performs first-run initialization:
// 1) Initialize local nsc operator (with SYS) enabling signing keys & account resolver URL
// 2) Generate resolver.conf -> ~/.dchat/resolver.conf and persist path into config
// 3) Collect & persist SYS account JWT, public key (write ~/.dchat/sys.pub) and seed path if discoverable
func EnsureSysAccountSetup(cfg *config.Config) error {
	// Skip if already initialized
	if cfg.Routes.ResolverConfig != "" {
		return nil
	}

	// Determine config directory
	confPath, err := config.GetConfigPath()
	if err != nil {
		return err
	}
	confDir := filepath.Dir(confPath)

	// Ensure NATS URL (for account resolver URL)
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

	// Idempotent nsc initialization sequence
	_ = run("nsc", "add", "operator", "--generate-signing-key", "--sys", "--name", "local")
	_ = run("nsc", "edit", "operator", "--require-signing-keys", "--account-jwt-server-url", natsURL)
	_ = run("nsc", "edit", "account", "SYS", "--sk", "generate")

	// Generate resolver.conf content
	resolverOut, err := runOut("nsc", "generate", "config", "--nats-resolver", "--sys-account", "SYS")
	if err != nil {
		return fmt.Errorf("nsc generate config failed: %w", err)
	}
	resolverPath := filepath.Join(confDir, "resolver.conf")
	if err := os.WriteFile(resolverPath, resolverOut, 0644); err != nil {
		return fmt.Errorf("write resolver.conf failed: %w", err)
	}
	cfg.Routes.ResolverConfig = resolverPath

	// Inspect nsc environment (JSON) to obtain store & keys directories
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

	// Describe SYS account to obtain JWT path & public key
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

	// Write public key to file for path persistence
	var sysPubPath string
	if sysPubKey != "" {
		sysPubPath = filepath.Join(confDir, "sys.pub")
		_ = os.WriteFile(sysPubPath, []byte(sysPubKey+"\n"), 0644)
	}

	// Attempt to locate SYS seed file by matching public key under KeysDir
	var sysSeedPath string
	if keysDir != "" && sysPubKey != "" {
		if p, _ := findSeedByPublicKey(keysDir, sysPubKey); p != "" {
			sysSeedPath = p
		}
	}

	// Persist collected paths into config
	cfg.NSC.Operator = "local"
	cfg.NSC.StoreDir = storeDir
	cfg.NSC.KeysDir = keysDir
	cfg.NSC.SysAccountJWT = sysJWTPath
	cfg.NSC.SysSeedPath = sysSeedPath
	cfg.NSC.SysPubPath = sysPubPath

	// Save updated config
	if err := config.SaveConfig(cfg); err != nil {
		return fmt.Errorf("save config failed: %w", err)
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

// findSeedByPublicKey walks keysDir to locate seed file matching the provided public key
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
