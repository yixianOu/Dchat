# Bug 报告: LeafNode GetLocalNATSURL 不支持随机端口

**日期**: 2026-03-04
**Bug ID**: BUG_LEAFNODE_GETLOCALURL_003

## 问题描述

`internal/leafnode/manager.go` 中的 `GetLocalNATSURL()` 函数在 `LocalPort` 配置为 0（随机端口）时，不会返回实际监听的端口，而是返回配置的 0 端口。

## 复现步骤

1. 创建 LeafNodeConfig，设置 LocalPort = 0
2. 调用 manager.Start()
3. 调用 manager.GetLocalNATSURL()
4. 查看返回的 URL，端口是 0 而不是实际监听的端口

## 测试代码

当前代码 (internal/leafnode/manager.go):
```go
func (m *Manager) GetLocalNATSURL() string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	host := m.config.LocalHost
	if host == "" {
		host = "127.0.0.1"
	}

	port := m.config.LocalPort
	// 注意：如果配置端口为0，实际监听端口需要通过其他方式获取
	// 这里暂时直接使用配置的端口  // ❌ 这是问题所在！

	return fmt.Sprintf("nats://%s:%d", host, port)
}
```

## 预期行为

当 `LocalPort` 为 0 时，应该从运行的 server 实例中获取实际监听的端口：

```go
func (m *Manager) GetLocalNATSURL() string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	host := m.config.LocalHost
	if host == "" {
		host = "127.0.0.1"
	}

	port := m.config.LocalPort
	if m.server != nil && port == 0 {
		// 从 server 获取实际监听的端口
		port = m.server.Addr().(*net.TCPAddr).Port
	}

	return fmt.Sprintf("nats://%s:%d", host, port)
}
```

## 实际行为

返回的 URL 端口始终是配置的端口，即使配置为 0（随机端口）时也是如此。

## 影响

1. 当使用随机端口（LocalPort = 0）时，无法获取正确的连接地址
2. NATS 客户端无法连接到正确的端口

## 建议修复

1. 导入 `net` 包
2. 在 `GetLocalNATSURL()` 中，当 server 正在运行且 port 为 0 时，从 `m.server.Addr()` 获取实际端口
