package nscsetup

import (
	"bufio"
	"bytes"
	"encoding/json"
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

	// derive names (allow user override via config; fallback to defaults)
	operatorName := cfg.NSC.Operator
	if operatorName == "" {
		operatorName = "local"
	}
	sysAccountName := cfg.NSC.Account
	if sysAccountName == "" {
		sysAccountName = "SYS"
	}
	sysUserName := cfg.NSC.User
	if sysUserName == "" {
		sysUserName = "sys"
	}
	resolverFileName := fmt.Sprintf("%s_resolver.conf", strings.ToLower(sysAccountName))

	natsURL := ensureNATSURL(cfg)

	if err := initNSCOperatorAndSys(natsURL, operatorName, sysAccountName); err != nil { // idempotent
		return err
	}

	// ensure default user before resolver (creds may be used by clients immediately)
	_, _, _ = execCommand("nsc", "add", "user", "--account", sysAccountName, "--name", sysUserName)
	_, _, _ = execCommand("nsc", "generate", "config", "--nats-resolver", "--sys-account", sysAccountName) // warm-up
	if err := generateResolverConfig(confDir, cfg, sysAccountName, resolverFileName); err != nil {
		return err
	}

	keysDir := readEnvPaths() // storeDir removed

	// set operator early so artifact discovery uses it
	cfg.NSC.Operator = operatorName
	userMeta, err := collectUserArtifacts(keysDir, confDir, operatorName, sysAccountName, sysUserName)
	if err != nil {
		return err
	}

	// Persist
	cfg.NSC.Operator = operatorName
	cfg.NSC.KeysDir = keysDir
	cfg.NSC.Account = userMeta.Account
	cfg.NSC.User = userMeta.User
	cfg.NSC.UserSeedPath = userMeta.UserSeedPath
	cfg.NSC.UserCredsPath = userMeta.UserCredsPath
	cfg.NSC.UserPubKey = userMeta.UserPubKey

	if err := config.SaveConfig(cfg); err != nil {
		return fmt.Errorf("save config failed: %w", err)
	}
	return nil
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
func initNSCOperatorAndSys(natsURL, operatorName, sysAccountName string) error {
	// create operator (idempotent) with provided name and sys account flag
	_, _, _ = execCommand("nsc", "add", "operator", "--generate-signing-key", "--sys", "--name", operatorName)
	// set operator context (nsc uses implicit current operator; ensure consistent name not required for edit)
	_, errb, err := execCommand("nsc", "edit", "operator", "--require-signing-keys", "--account-jwt-server-url", natsURL)
	if err != nil {
		return fmt.Errorf("edit operator failed: %s", strings.TrimSpace(errb.String()))
	}
	_, _, _ = execCommand("nsc", "edit", "account", sysAccountName, "--sk", "generate")
	return nil
}

// generateResolverConfig writes resolver.conf and updates cfg.Routes.ResolverConfig
func generateResolverConfig(confDir string, cfg *config.Config, sysAccountName, resolverFileName string) error {
	out, errb, err := execCommand("nsc", "generate", "config", "--nats-resolver", "--sys-account", sysAccountName)
	if err != nil {
		return fmt.Errorf("nsc generate config failed: %s", strings.TrimSpace(errb.String()))
	}
	name := resolverFileName
	if name == "" {
		name = "resolver.conf"
	}
	resolverPath := filepath.Join(confDir, name)
	if err := os.WriteFile(resolverPath, out.Bytes(), 0644); err != nil {
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
	UserPubKey    string
}

// accountArtifacts removed (no longer tracking account seed)

// collectUserArtifacts locates user-level JWT/creds/seed under SYS account (default user: sys)
func collectUserArtifacts(keysDir, confDir, operatorName, accountName, userName string) (*userArtifacts, error) {
	if accountName == "" {
		accountName = "SYS"
	}
	if userName == "" {
		userName = "sys"
	}
	// Confirm user exists (best-effort create earlier). Try json describe to get pub key
	var userPubKey string
	if out, errb, err := execCommand("nsc", "describe", "user", userName, "--account", accountName, "--json"); err == nil {
		var desc map[string]any
		if json.Unmarshal(out.Bytes(), &desc) == nil {
			if pk, ok := desc["sub"].(string); ok {
				userPubKey = pk
			}
			if nm, ok := desc["name"].(string); ok && nm != "" {
				userName = nm
			}
		}
	} else {
		_ = errb
	}
	userCredsPath := findCredsFile(keysDir, operatorName, accountName, userName)
	var userSeedPath string
	if userPubKey != "" { // export user key
		if p, err := exportSeed("user", accountName, userName, userPubKey, confDir); err == nil && p != "" {
			userSeedPath = p
		}
	}
	return &userArtifacts{Account: accountName, User: userName, UserCredsPath: userCredsPath, UserSeedPath: userSeedPath, UserPubKey: userPubKey}, nil
}

// collectAccountArtifacts removed (account seed no longer exported)

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

// readEnvPaths executes `nsc env` (no JSON flag available) and parses keys directory.
func readEnvPaths() (keysDir string) {
	out, errb, err := execCommand("nsc", "env")
	if err != nil {
		_ = errb
		return defaultKeysDir()
	}
	scanner := bufio.NewScanner(bytes.NewReader(out.Bytes()))
	var rawKeys string
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, "NKEYS_PATH") { // deprecated row shows effective default keys dir
			parts := strings.Split(line, "|")
			if len(parts) >= 4 {
				rawKeys = strings.TrimSpace(parts[3])
			}
		}
	}
	keysDir = expandHomeOrDefault(rawKeys, defaultKeysDir())
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
	if kind != "user" { // only user seed supported now
		return "", nil
	}
	args = append(args, "--users", "--account", accountName, "--user", userName)
	args = append(args, "--dir", tmpDir, "--force")
	if _, errb, err := execCommand("nsc", args...); err != nil {
		return "", fmt.Errorf("export keys failed: %s", strings.TrimSpace(errb.String()))
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
	dest = filepath.Join(confDir, fmt.Sprintf("%s_%s.seed", strings.ToLower(accountName), strings.ToLower(userName)))
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
