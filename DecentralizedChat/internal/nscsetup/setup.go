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
	"strings"
	"time"

	"DecentralizedChat/internal/config"
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
	cfg.NSC.SysCredsPath = sysMeta.CredsPath

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
	JWTPath   string
	CredsPath string
	SeedPath  string
}

// collectSysAccountArtifacts locates JWT/public key/seed for SYS account
func collectSysAccountArtifacts(storeDir, keysDir, confDir string, cfg *config.Config) (*sysArtifacts, error) {
	// 简化策略：仅通过已知目录结构推导 SYS 账户 JWT 路径，不做多轮回退解析。
	// 仍尝试获取公钥以便后续查找种子文件，但不依赖其来确定 JWT 路径。
	accountName := "SYS"
	var sysPubKey string
	if sysDescJSON, jerr := runOut("nsc", "describe", "account", "SYS", "--json"); jerr == nil { // best-effort
		var desc map[string]any
		if err := json.Unmarshal(sysDescJSON, &desc); err == nil {
			if pk, ok := desc["sub"].(string); ok {
				sysPubKey = pk
			}
			if nm, ok := desc["name"].(string); ok && nm != "" {
				accountName = nm
			}
		}
	}
	// 直接推导路径（stores/<operator>/accounts/<ACCOUNT>/<ACCOUNT>.jwt）
	sysJWTPath := findAccountJWTPath(storeDir, cfg.NSC.Operator, accountName, "")

	// Locate creds file (keys/creds/<operator>/<ACCOUNT>/*.creds)
	var sysCredsPath string
	if keysDir != "" {
		if p := findAccountCredsFile(keysDir, cfg.NSC.Operator, accountName); p != "" {
			sysCredsPath = p
		}
	}

	// Export seed via nsc instead of walking filesystem
	var sysSeedPath string
	if sysPubKey != "" { // best-effort
		if p, err := exportAccountSeed(accountName, sysPubKey, confDir); err == nil && p != "" {
			sysSeedPath = p
		}
	}

	return &sysArtifacts{JWTPath: sysJWTPath, CredsPath: sysCredsPath, SeedPath: sysSeedPath}, nil
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
// exportAccountSeed uses `nsc export keys --accounts --account <name>` to obtain the account seed for the identity key.
// It writes/updates a stable file under confDir (e.g. sys.seed) with 0600 permission and returns its path.
func exportAccountSeed(accountName, pubKey, confDir string) (string, error) {
	if accountName == "" || pubKey == "" || confDir == "" {
		return "", nil
	}
	tmpDir, err := os.MkdirTemp("", "nsc-exp-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmpDir)
	// Export only account keys of given account
	if err := run("nsc", "export", "keys", "--accounts", "--account", accountName, "--dir", tmpDir, "--force"); err != nil {
		return "", err
	}
	seedFile := filepath.Join(tmpDir, pubKey+".nk")
	if st, err := os.Stat(seedFile); err != nil || st.IsDir() {
		return "", nil // not found (maybe signing key only)
	}
	data, err := os.ReadFile(seedFile)
	if err != nil {
		return "", err
	}
	seed := strings.TrimSpace(string(data))
	if seed == "" || !strings.HasPrefix(seed, "S") { // basic sanity
		return "", nil
	}
	dest := filepath.Join(confDir, strings.ToLower(accountName)+".seed")
	// Write idempotently
	_ = os.WriteFile(dest, []byte(seed+"\n"), 0600)
	return dest, nil
}

// findAccountCredsFile locates a creds file for the SYS (or any) account.
// Expected layout: <keysDir>/creds/<operator>/<ACCOUNT>/<user>.creds (e.g. sys.creds)
func findAccountCredsFile(keysDir, operator, accountName string) string {
	if keysDir == "" || operator == "" || accountName == "" {
		return ""
	}
	base := filepath.Join(keysDir, "creds", operator, accountName)
	entries, err := os.ReadDir(base)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasSuffix(name, ".creds") { // first creds file is returned
			return filepath.Join(base, name)
		}
	}
	return ""
}
