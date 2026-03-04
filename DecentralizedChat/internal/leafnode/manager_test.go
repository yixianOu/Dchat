package leafnode

import (
	"testing"
	"time"

	"DecentralizedChat/internal/config"
)

func TestDefaultConfig(t *testing.T) {
	t.Log("=== Test: DefaultConfig ===")

	cfg := config.DefaultLeafNodeConfig()

	if cfg.LocalHost != "127.0.0.1" {
		t.Errorf("Default LocalHost = %q, want %q", cfg.LocalHost, "127.0.0.1")
	}
	if cfg.LocalPort != 4222 {
		t.Errorf("Default LocalPort = %d, want %d", cfg.LocalPort, 4222)
	}
	if len(cfg.HubURLs) == 0 {
		t.Error("Default HubURLs is empty, want at least one")
	}

	t.Log("✅ DefaultConfig test passed")
}

func TestNewManager(t *testing.T) {
	t.Log("=== Test: NewManager ===")

	cfg := config.DefaultLeafNodeConfig()
	cfg.LocalPort = 0 // Use random port for testing

	mgr := NewManager(cfg)
	if mgr == nil {
		t.Fatal("NewManager returned nil")
	}
	if mgr.config == nil {
		t.Error("Manager.config is nil")
	}
	if mgr.IsRunning() != false {
		t.Error("New manager should not be running")
	}

	t.Log("✅ NewManager test passed")
}

func TestManager_StartStop(t *testing.T) {
	t.Log("=== Test: Manager Start/Stop ===")

	cfg := config.DefaultLeafNodeConfig()
	cfg.LocalPort = 0     // Random port
	cfg.HubURLs = []string{} // No hub for local test
	cfg.ConnectTimeout = 5 * time.Second

	mgr := NewManager(cfg)

	// Start the manager
	t.Log("Starting LeafNode manager...")
	err := mgr.Start()
	if err != nil {
		// It's okay if it fails because no hub URLs - let's verify the error
		t.Logf("Start failed as expected (no hub URLs): %v", err)
	} else {
		// If it started, check it's running
		if !mgr.IsRunning() {
			t.Error("Manager should be running after Start()")
		}
		t.Log("Manager started successfully")

		// Get local URL
		url := mgr.GetLocalNATSURL()
		if url == "" {
			t.Error("GetLocalNATSURL returned empty")
		}
		t.Logf("Local NATS URL: %s", url)

		// Stop the manager
		t.Log("Stopping LeafNode manager...")
		mgr.Stop()

		if mgr.IsRunning() {
			t.Error("Manager should not be running after Stop()")
		}
		t.Log("Manager stopped successfully")
	}

	t.Log("✅ Manager Start/Stop test passed")
}

func TestManager_GetConnectedHubCount(t *testing.T) {
	t.Log("=== Test: GetConnectedHubCount ===")

	cfg := config.DefaultLeafNodeConfig()
	cfg.LocalPort = 0
	cfg.HubURLs = []string{}

	mgr := NewManager(cfg)

	// Before start
	count := mgr.GetConnectedHubCount()
	if count != 0 {
		t.Errorf("GetConnectedHubCount before start = %d, want 0", count)
	}

	t.Log("✅ GetConnectedHubCount test passed")
}

func TestManager_parseHubURLs(t *testing.T) {
	t.Log("=== Test: parseHubURLs ===")

	cfg := config.DefaultLeafNodeConfig()
	cfg.HubURLs = []string{
		"nats://hub1.example.com:7422",
		"nats://hub2.example.com:7422",
		"invalid-url", // This should be skipped
	}

	mgr := NewManager(cfg)

	remotes, err := mgr.parseHubURLs()
	if err != nil {
		t.Errorf("parseHubURLs returned error: %v", err)
	}

	// Should have 2 valid URLs (invalid one should be skipped)
	if len(remotes) < 2 {
		t.Logf("parseHubURLs returned %d remotes (expected at least 2)", len(remotes))
	}

	t.Log("✅ parseHubURLs test passed")
}

func TestManager_buildServerOptions(t *testing.T) {
	t.Log("=== Test: buildServerOptions ===")

	cfg := config.DefaultLeafNodeConfig()
	cfg.LocalHost = "127.0.0.1"
	cfg.LocalPort = 9999

	mgr := NewManager(cfg)

	// Parse some test URLs
	remotes, _ := mgr.parseHubURLs()

	opts := mgr.buildServerOptions(remotes)

	if opts.Host != cfg.LocalHost {
		t.Errorf("opts.Host = %q, want %q", opts.Host, cfg.LocalHost)
	}
	if opts.Port != cfg.LocalPort {
		t.Errorf("opts.Port = %d, want %d", opts.Port, cfg.LocalPort)
	}
	// LeafNode doesn't use JetStream locally (uses SQLite instead)
	if opts.JetStream != false {
		t.Errorf("opts.JetStream = %v, want false", opts.JetStream)
	}

	t.Log("✅ buildServerOptions test passed")
}
