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

	// inline resolveConfigDir
	confPath, err := config.GetConfigPath()
	if err != nil {
		return err
	}
	confDir := filepath.Dir(confPath)

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
	cfg.NSC.AccountSeedPath = acctMeta.AccountSeedPath

	if err := config.SaveConfig(cfg); err != nil {
		return fmt.Errorf("save config failed: %w", err)
	}
	return nil
}

// removed resolveConfigDir: logic inlined in EnsureSysAccountSetup

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
	Account         string
	AccountSeedPath string
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
	userCredsPath := findCredsFile(keysDir, cfg.NSC.Operator, accountName, userName)
	var userSeedPath string
	if userPubKey != "" { // export user key
		if p, err := exportSeed("user", accountName, userName, userPubKey, confDir); err == nil && p != "" {
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
	var acctSeed string
	if acctPubKey != "" {
		if p, err := exportSeed("account", accountName, "", acctPubKey, confDir); err == nil && p != "" {
			acctSeed = p
		}
	}
	return &accountArtifacts{Account: accountName, AccountSeedPath: acctSeed}, nil
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
// exportSeed exports a user or account seed using nsc and writes it under confDir.
// kind: "user" or "account". For user kind, userName must be provided.
func exportSeed(kind, accountName, userName, pubKey, confDir string) (string, error) {
	if accountName == "" || pubKey == "" || confDir == "" {
		return "", nil
	}
	if kind == "user" && userName == "" { // invalid user invocation
		return "", nil
	}
	prefix := "nsc-exp-" + kind + "-*"
	tmpDir, err := os.MkdirTemp("", prefix)
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmpDir)
	args := []string{"export", "keys"}
	switch kind {
	case "user":
		args = append(args, "--users", "--account", accountName, "--user", userName)
	case "account":
		args = append(args, "--accounts", "--account", accountName)
	default:
		return "", nil
	}
	args = append(args, "--dir", tmpDir, "--force")
	if err := run("nsc", args...); err != nil {
		return "", err
	}
	seedFile := filepath.Join(tmpDir, pubKey+".nk")
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
	var dest string
	if kind == "user" {
		dest = filepath.Join(confDir, fmt.Sprintf("%s_%s.seed", strings.ToLower(accountName), strings.ToLower(userName)))
	} else {
		dest = filepath.Join(confDir, fmt.Sprintf("%s_account.seed", strings.ToLower(accountName)))
	}
	_ = os.WriteFile(dest, []byte(seed+"\n"), 0600)
	return dest, nil
}

// findCredsFile locates a creds file. If userName provided, returns that user's creds; otherwise first creds under account.
// Layout: <keysDir>/creds/<operator>/<ACCOUNT>/<user>.creds
func findCredsFile(keysDir, operator, accountName, userName string) string {
	if keysDir == "" || operator == "" || accountName == "" {
		return ""
	}
	base := filepath.Join(keysDir, "creds", operator, accountName)
	entries, err := os.ReadDir(base)
	if err != nil {
		return ""
	}
	var first string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(strings.ToLower(name), ".creds") {
			continue
		}
		if userName != "" {
			if strings.EqualFold(name, userName+".creds") {
				return filepath.Join(base, name)
			}
		} else if first == "" { // capture first for account-level
			first = filepath.Join(base, name)
		}
	}
	if userName == "" {
		return first
	}
	return "" // not found
}
