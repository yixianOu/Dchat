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
// 2) Ensure default user (sys) exists under SYS account and generate creds
// 3) Generate resolver.conf -> ~/.dchat/resolver.conf and persist path into config
// 4) Collect & persist user level artifacts: user JWT, user creds, user seed
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

	// ensure default user before resolver (creds may be used by clients immediately)
	_ = run("nsc", "add", "user", "--account", "SYS", "--name", "sys")
	_ = run("nsc", "generate", "config", "--nats-resolver", "--sys-account", "SYS") // warm-up
	if err := generateResolverConfig(confDir, cfg); err != nil {
		return err
	}

	keysDir, storeDir := readEnvPaths() // existing approach

	userMeta, err := collectUserArtifacts(storeDir, keysDir, confDir, cfg)
	if err != nil {
		return err
	}

	acctMeta, err := collectAccountArtifacts(storeDir, keysDir, confDir, cfg, userMeta.Account)
	if err != nil {
		return err
	}

	// Persist
	cfg.NSC.Operator = "local"
	cfg.NSC.StoreDir = storeDir
	cfg.NSC.KeysDir = keysDir
	cfg.NSC.Account = userMeta.Account
	cfg.NSC.User = userMeta.User
	cfg.NSC.UserSeedPath = userMeta.UserSeedPath
	cfg.NSC.UserCredsPath = userMeta.UserCredsPath
	cfg.NSC.AccountCredsPath = acctMeta.AccountCredsPath
	cfg.NSC.AccountSeedPath = acctMeta.AccountSeedPath

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

// userArtifacts holds resolved paths related to a specific user
type userArtifacts struct {
	Account       string
	User          string
	UserCredsPath string
	UserSeedPath  string
}

// accountArtifacts holds account-level artifacts
type accountArtifacts struct {
	Account          string
	AccountCredsPath string
	AccountSeedPath  string
}

// collectUserArtifacts locates user-level JWT/creds/seed under SYS account (default user: sys)
func collectUserArtifacts(storeDir, keysDir, confDir string, cfg *config.Config) (*userArtifacts, error) {
	accountName := "SYS"
	userName := "sys"
	// Confirm user exists (best-effort create earlier). Try json describe to get pub key
	var userPubKey string
	if b, err := runOut("nsc", "describe", "user", userName, "--account", accountName, "--json"); err == nil {
		var desc map[string]any
		if json.Unmarshal(b, &desc) == nil {
			if pk, ok := desc["sub"].(string); ok {
				userPubKey = pk
			}
			if nm, ok := desc["name"].(string); ok && nm != "" {
				userName = nm
			}
		}
	}
	userCredsPath := findUserCredsFile(keysDir, cfg.NSC.Operator, accountName, userName)
	var userSeedPath string
	if userPubKey != "" { // export user key
		if p, err := exportUserSeed(accountName, userName, userPubKey, confDir); err == nil && p != "" {
			userSeedPath = p
		}
	}
	return &userArtifacts{Account: accountName, User: userName, UserCredsPath: userCredsPath, UserSeedPath: userSeedPath}, nil
}

// collectAccountArtifacts gathers account-level jwt/creds/seed (seed export optional)
func collectAccountArtifacts(storeDir, keysDir, confDir string, cfg *config.Config, accountName string) (*accountArtifacts, error) {
	if accountName == "" {
		accountName = "SYS"
	}
	var acctPubKey string
	if b, err := runOut("nsc", "describe", "account", accountName, "--json"); err == nil {
		var desc map[string]any
		if json.Unmarshal(b, &desc) == nil {
			if pk, ok := desc["sub"].(string); ok {
				acctPubKey = pk
			}
		}
	}
	acctCreds := findAccountCredsFile(keysDir, cfg.NSC.Operator, accountName)
	var acctSeed string
	if acctPubKey != "" {
		if p, err := exportAccountSeed(accountName, acctPubKey, confDir); err == nil && p != "" {
			acctSeed = p
		}
	}
	return &accountArtifacts{Account: accountName, AccountCredsPath: acctCreds, AccountSeedPath: acctSeed}, nil
}

// execCommand executes a command with common NSC environment settings and returns stdout/stderr buffers.
func execCommand(name string, args ...string) (stdout bytes.Buffer, stderr bytes.Buffer, err error) {
	cmd := exec.Command(name, args...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.Env = os.Environ()
	cmd.Env = append(cmd.Env, "NSC_NO_GITHUB=1", "NO_COLOR=1")
	cmd.WaitDelay = 10 * time.Second
	if e := cmd.Run(); e != nil {
		err = e
	}
	return
}

// run wraps execCommand discarding stdout and mapping stderr into error context.
func run(name string, args ...string) error {
	_, errb, err := execCommand(name, args...)
	if err != nil {
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
	out, errb, err := execCommand(name, args...)
	if err != nil {
		return nil, errors.New(strings.TrimSpace(errb.String()))
	}
	return out.Bytes(), nil
}

// Removed JWT path persistence: we intentionally do not record user/account JWT file locations; only creds & seeds retained.

// findSeedByPublicKey walks keysDir to locate seed file matching the provided public key
// exportAccountSeed uses `nsc export keys --accounts --account <name>` to obtain the account seed for the identity key.
// It writes/updates a stable file under confDir (e.g. sys.seed) with 0600 permission and returns its path.
func exportUserSeed(accountName, userName, userPubKey, confDir string) (string, error) {
	if accountName == "" || userName == "" || userPubKey == "" || confDir == "" {
		return "", nil
	}
	tmpDir, err := os.MkdirTemp("", "nsc-exp-user-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmpDir)
	if err := run("nsc", "export", "keys", "--users", "--account", accountName, "--user", userName, "--dir", tmpDir, "--force"); err != nil {
		return "", err
	}
	seedFile := filepath.Join(tmpDir, userPubKey+".nk")
	st, err := os.Stat(seedFile)
	if err != nil || st.IsDir() {
		return "", nil
	}
	data, err := os.ReadFile(seedFile)
	if err != nil {
		return "", err
	}
	seed := strings.TrimSpace(string(data))
	if seed == "" || !strings.HasPrefix(seed, "S") {
		return "", nil
	}
	dest := filepath.Join(confDir, fmt.Sprintf("%s_%s.seed", strings.ToLower(accountName), strings.ToLower(userName)))
	_ = os.WriteFile(dest, []byte(seed+"\n"), 0600)
	return dest, nil
}

// exportAccountSeed parallels user seed export but for account keys
func exportAccountSeed(accountName, acctPubKey, confDir string) (string, error) {
	if accountName == "" || acctPubKey == "" || confDir == "" {
		return "", nil
	}
	tmpDir, err := os.MkdirTemp("", "nsc-exp-acct-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmpDir)
	if err := run("nsc", "export", "keys", "--accounts", "--account", accountName, "--dir", tmpDir, "--force"); err != nil {
		return "", err
	}
	seedFile := filepath.Join(tmpDir, acctPubKey+".nk")
	st, err := os.Stat(seedFile)
	if err != nil || st.IsDir() {
		return "", nil
	}
	data, err := os.ReadFile(seedFile)
	if err != nil {
		return "", err
	}
	seed := strings.TrimSpace(string(data))
	if seed == "" || !strings.HasPrefix(seed, "S") {
		return "", nil
	}
	dest := filepath.Join(confDir, fmt.Sprintf("%s_account.seed", strings.ToLower(accountName)))
	_ = os.WriteFile(dest, []byte(seed+"\n"), 0600)
	return dest, nil
}

// findAccountCredsFile locates a creds file for the SYS (or any) account.
// Expected layout: <keysDir>/creds/<operator>/<ACCOUNT>/<user>.creds (e.g. sys.creds)
func findUserCredsFile(keysDir, operator, accountName, userName string) string {
	if keysDir == "" || operator == "" || accountName == "" || userName == "" {
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
		if strings.EqualFold(name, userName+".creds") {
			return filepath.Join(base, name)
		}
	}
	return "" // not found (maybe no creds generated yet)
}

// findAccountCredsFile returns first creds file under an account (any user) for reference
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
		if strings.HasSuffix(strings.ToLower(name), ".creds") {
			return filepath.Join(base, name)
		}
	}
	return ""
}
