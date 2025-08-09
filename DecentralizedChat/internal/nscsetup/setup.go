package nscsetup

import (
	"bufio"
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

const (
	DefaultClientPort = 4222
)

// EnsureSysAccountSetup performs first-run initialization:
// 1) Initialize local nsc operator (with SYS) enabling signing keys & account resolver URL
// 2) Generate resolver.conf -> ~/.dchat/resolver.conf and persist path into config
// 3) Collect & persist SYS account JWT, public key (write ~/.dchat/sys.pub) and seed path if discoverable
func EnsureSysAccountSetup(cfg *config.Config) error {
	if cfg.Routes.ResolverConfig != "" { // already initialized
		return nil
	}

	confDir, err := resolveConfigDir()
	if err != nil {
		return err
	}

	natsURL := ensureNATSURL(cfg)

	if err := initNSCOperatorAndSys(natsURL); err != nil { // idempotent
		return err
	}

	if err := generateResolverConfig(confDir, cfg); err != nil {
		return err
	}

	keysDir, storeDir := readEnvPaths() // existing approach

	sysMeta, err := collectSysAccountArtifacts(storeDir, keysDir, confDir, cfg)
	if err != nil {
		return err
	}

	// Persist
	cfg.NSC.Operator = "local"
	cfg.NSC.StoreDir = storeDir
	cfg.NSC.KeysDir = keysDir
	cfg.NSC.SysAccountJWT = sysMeta.JWTPath
	cfg.NSC.SysSeedPath = sysMeta.SeedPath
	cfg.NSC.SysPubPath = sysMeta.PubPath

	if err := config.SaveConfig(cfg); err != nil {
		return fmt.Errorf("save config failed: %w", err)
	}
	return nil
}

// resolveConfigDir determines the runtime config directory path
func resolveConfigDir() (string, error) {
	confPath, err := config.GetConfigPath()
	if err != nil {
		return "", err
	}
	return filepath.Dir(confPath), nil
}

// ensureNATSURL builds and persists the NATS URL if empty; returns the URL
func ensureNATSURL(cfg *config.Config) string {
	if cfg.NATS.URL != "" {
		return cfg.NATS.URL
	}
	host := cfg.Routes.Host
	if host == "" {
		host = cfg.Network.LocalIP
	}
	if cfg.Routes.ClientPort == 0 {
		cfg.Routes.ClientPort = DefaultClientPort
	}
	cfg.NATS.URL = fmt.Sprintf("nats://%s:%d", host, cfg.Routes.ClientPort)
	return cfg.NATS.URL
}

// initNSCOperatorAndSys performs idempotent operator/SYS initialization
func initNSCOperatorAndSys(natsURL string) error {
	_ = run("nsc", "add", "operator", "--generate-signing-key", "--sys", "--name", "local")
	if err := run("nsc", "edit", "operator", "--require-signing-keys", "--account-jwt-server-url", natsURL); err != nil {
		return err
	}
	_ = run("nsc", "edit", "account", "SYS", "--sk", "generate")
	return nil
}

// generateResolverConfig writes resolver.conf and updates cfg.Routes.ResolverConfig
func generateResolverConfig(confDir string, cfg *config.Config) error {
	resolverOut, err := runOut("nsc", "generate", "config", "--nats-resolver", "--sys-account", "SYS")
	if err != nil {
		return fmt.Errorf("nsc generate config failed: %w", err)
	}
	resolverPath := filepath.Join(confDir, "resolver.conf")
	if err := os.WriteFile(resolverPath, resolverOut, 0644); err != nil {
		return fmt.Errorf("write resolver.conf failed: %w", err)
	}
	cfg.Routes.ResolverConfig = resolverPath
	return nil
}

// sysArtifacts holds resolved paths related to SYS account
type sysArtifacts struct {
	JWTPath  string
	PubPath  string
	SeedPath string
}

// collectSysAccountArtifacts locates JWT/public key/seed for SYS account
func collectSysAccountArtifacts(storeDir, keysDir, confDir string, cfg *config.Config) (*sysArtifacts, error) {
	var sysJWTPath, sysPubKey, accountName string = "", "", "SYS"
	sysDescJSON, jerr := runOut("nsc", "describe", "account", "SYS", "--json")
	if jerr == nil {
		var desc map[string]any
		if err := json.Unmarshal(sysDescJSON, &desc); err == nil {
			if pk, ok := desc["sub"].(string); ok {
				sysPubKey = pk
			}
			if nm, ok := desc["name"].(string); ok && nm != "" {
				accountName = nm
			}
		}
		candidate := findAccountJWTPath(storeDir, cfg.NSC.Operator, accountName, sysPubKey)
		if candidate != "" {
			sysJWTPath = candidate
		}
	}
	if sysJWTPath == "" { // fallback plaintext parsing
		sysDescText, _ := runOut("nsc", "describe", "account", "SYS")
		sysJWTPath = firstMatch(string(sysDescText), `JWT\s+file:\s+(.+)`)
		if sysPubKey == "" {
			sysPubKey = firstMatch(string(sysDescText), `Public\s+key:\s+([A-Z0-9]+)`)
		}
		if sysJWTPath == "" { // final attempt
			candidate := findAccountJWTPath(storeDir, cfg.NSC.Operator, accountName, sysPubKey)
			if candidate != "" {
				sysJWTPath = candidate
			}
		}
	}

	var sysPubPath string
	if sysPubKey != "" {
		sysPubPath = filepath.Join(confDir, "sys.pub")
		_ = os.WriteFile(sysPubPath, []byte(sysPubKey+"\n"), 0644)
	}

	var sysSeedPath string
	if keysDir != "" && sysPubKey != "" {
		if p, _ := findSeedByPublicKey(keysDir, sysPubKey); p != "" {
			sysSeedPath = p
		}
	}

	return &sysArtifacts{JWTPath: sysJWTPath, PubPath: sysPubPath, SeedPath: sysSeedPath}, nil
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

// readEnvPaths executes `nsc env` (no JSON flag available) and parses key/store directories.
func readEnvPaths() (keysDir, storeDir string) {
	out, err := runOut("nsc", "env")
	if err != nil {
		return defaultKeysDir(), defaultStoresDir()
	}
	scanner := bufio.NewScanner(bytes.NewReader(out))
	var rawKeys, rawStore string
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, "Current Store Dir") {
			// columns separated by '|'
			parts := strings.Split(line, "|")
			if len(parts) >= 4 {
				rawStore = strings.TrimSpace(parts[3])
			}
		} else if strings.Contains(line, "NKEYS_PATH") { // deprecated row shows effective default keys dir
			parts := strings.Split(line, "|")
			if len(parts) >= 4 {
				rawKeys = strings.TrimSpace(parts[3])
			}
		}
	}
	keysDir = expandHomeOrDefault(rawKeys, defaultKeysDir())
	storeDir = expandHomeOrDefault(rawStore, defaultStoresDir())
	return
}

func expandHomeOrDefault(p string, def string) string {
	if p == "" {
		return def
	}
	if strings.HasPrefix(p, "~") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, strings.TrimPrefix(p, "~"))
		}
		return def
	}
	return p
}

func defaultKeysDir() string {
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".local", "share", "nats", "nsc", "keys")
	}
	return ""
}

func defaultStoresDir() string {
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".local", "share", "nats", "nsc", "stores")
	}
	return ""
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

// findAccountJWTPath simplified: only support current observed layout
// stores/<operator>/accounts/<ACCOUNT>/<ACCOUNT>.jwt
func findAccountJWTPath(storeDir, operator, accountName, _ string) string {
	if storeDir == "" || operator == "" || accountName == "" {
		return ""
	}
	p := filepath.Join(storeDir, operator, "accounts", accountName, accountName+".jwt")
	if st, err := os.Stat(p); err == nil && !st.IsDir() {
		return p
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
