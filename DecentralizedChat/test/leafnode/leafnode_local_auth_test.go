package e2e

import (
	"net"
	"testing"
	"time"

	"DecentralizedChat/internal/config"
	"DecentralizedChat/internal/leafnode"
	"DecentralizedChat/internal/nscsetup"

	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLeafNodeLocalAuthentication(t *testing.T) {
	// 1. 重写配置路径到临时目录
	tmpDir := t.TempDir()
	origGetConfigPath := config.GetConfigPath
	defer func() { config.GetConfigPath = origGetConfigPath }()
	config.GetConfigPath = func() (string, error) {
		return tmpDir + "/config.json", nil
	}

	// 2. 初始化配置和NSC设置
	cfg, err := config.LoadConfig()
	require.NoError(t, err)
	cfg.User.Nickname = "test_user"
	cfg.LeafNode.LocalHost = "127.0.0.1" // 强制绑定本地回环地址
	cfg.LeafNode.LocalPort = 0            // 随机端口
	err = nscsetup.EnsureSimpleSetup(cfg)
	require.NoError(t, err)

	// 验证并同步配置
	err = cfg.ValidateAndSetDefaults()
	require.NoError(t, err)

	// 3. 保存配置
	err = config.SaveConfig(cfg)
	require.NoError(t, err)

	// 4. 验证配置
	assert.Equal(t, "127.0.0.1", cfg.LeafNode.LocalHost)
	assert.NotEmpty(t, cfg.LeafNode.CredsFile)
	assert.FileExists(t, cfg.LeafNode.CredsFile)

	// 5. 启动LeafNode
	manager := leafnode.NewManager(&cfg.LeafNode)
	err = manager.Start()
	require.NoError(t, err)
	defer manager.Stop()
	assert.True(t, manager.IsRunning())

	// 获取本地NATS地址
	localURL := manager.GetLocalNATSURL()
	assert.NotEmpty(t, localURL)
	t.Logf("LeafNode listening on: %s", localURL)

	// 解析地址，验证只监听本地回环
	host, port, err := net.SplitHostPort(localURL[len("nats://"):])
	require.NoError(t, err)
	assert.Equal(t, "127.0.0.1", host)
	t.Logf("Listening on port: %s", port)

	// 6. 测试1：本地连接成功（因为只监听本地，不需要额外认证）
	t.Run("LocalConnectionSucceeds", func(t *testing.T) {
		nc, err := nats.Connect(localURL, nats.Timeout(2*time.Second))
		require.NoError(t, err)
		defer nc.Close()
		assert.True(t, nc.IsConnected())

		// 测试消息收发
		subject := "test.local.auth"
		testMsg := []byte("Hello Local Auth!")

		// 订阅
		sub, err := nc.SubscribeSync(subject)
		require.NoError(t, err)
		defer sub.Unsubscribe()

		// 发布消息
		err = nc.Publish(subject, testMsg)
		require.NoError(t, err)
		err = nc.Flush()
		require.NoError(t, err)

		// 接收消息
		msg, err := sub.NextMsg(2 * time.Second)
		require.NoError(t, err)
		assert.Equal(t, testMsg, msg.Data)
	})

	// 7. 测试2：请求-响应模式正常工作
	t.Run("RequestResponseWorks", func(t *testing.T) {
		nc, err := nats.Connect(localURL, nats.Timeout(2*time.Second))
		require.NoError(t, err)
		defer nc.Close()

		subject := "test.local.request"
		responseMsg := []byte("Response from local server!")

		// 注册响应者
		_, err = nc.Subscribe(subject, func(msg *nats.Msg) {
			_ = msg.Respond(responseMsg)
		})
		require.NoError(t, err)
		err = nc.Flush()
		require.NoError(t, err)

		// 发送请求
		resp, err := nc.Request(subject, []byte("Request data"), 2*time.Second)
		require.NoError(t, err)
		assert.Equal(t, responseMsg, resp.Data)
	})

	// 8. 测试3：Hub连接认证正常（使用CredsFile）
	t.Run("HubConnectionUsesCredentials", func(t *testing.T) {
		// 验证配置中CredsFile已经正确设置
		assert.NotEmpty(t, cfg.LeafNode.CredsFile)
		// 验证LeafNode的配置包含凭证
		assert.Equal(t, cfg.LeafNode.CredsFile, manager.GetConfig().CredsFile)
		t.Logf("LeafNode will connect to Hub using credentials: %s", cfg.LeafNode.CredsFile)
	})
}

// 为了测试，我们给Manager添加一个测试用的方法，或者直接通过GetConfig验证
// 这里我们直接验证配置即可，不需要实际启动Hub
