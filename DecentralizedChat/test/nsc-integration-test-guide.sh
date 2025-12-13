#!/bin/bash

# DChat NSC密钥集成完整测试指南
# 这个脚本展示如何使用NSC生成的密钥进行完整的聊天测试

set -e

echo "🚀 DChat NSC密钥集成测试指南"
echo "================================"

# 创建测试目录
mkdir -p test-nsc-setup
cd test-nsc-setup

echo ""
echo "📋 测试流程说明:"
echo "1. ✅ NSC自动生成用户身份密钥 (认证 + 聊天加密)"
echo "2. ✅ 应用启动时自动加载NSC密钥"
echo "3. ✅ 前端通过NSC公钥添加好友"
echo "4. ✅ 直接使用NSC派生的密钥进行聊天"

echo ""
echo "🔧 关键改进："
echo "- 不再需要手动生成聊天密钥"
echo "- NSC Ed25519密钥自动转换为X25519聊天密钥"
echo "- 密钥管理完全统一，减少复杂度"

echo ""
echo "📁 测试准备："

# 创建两个独立的应用实例配置
echo "创建 Alice 和 Bob 的独立配置..."

# Alice 配置 (端口 4222/6222)
mkdir -p alice/.dchat
cat > alice/.dchat/config.json << 'EOF'
{
  "user": {
    "id": "alice",
    "nickname": "Alice"
  },
  "network": {
    "auto_discovery": true,
    "seed_nodes": [],
    "local_ip": "127.0.0.1"
  },
  "server": {
    "client_port": 4222,
    "cluster_port": 6222,
    "cluster_name": "dchat-cluster",
    "host": "127.0.0.1"
  },
  "nsc": {
    "operator": "dchat-operator-alice",
    "account": "SYS",
    "user": "alice-user"
  }
}
EOF

# Bob 配置 (端口 4223/6223)
mkdir -p bob/.dchat
cat > bob/.dchat/config.json << 'EOF'
{
  "user": {
    "id": "bob", 
    "nickname": "Bob"
  },
  "network": {
    "auto_discovery": true,
    "seed_nodes": ["nats://127.0.0.1:4222"],
    "local_ip": "127.0.0.1"
  },
  "server": {
    "client_port": 4223,
    "cluster_port": 6223,
    "cluster_name": "dchat-cluster",
    "host": "127.0.0.1",
    "routes": ["nats://127.0.0.1:6222"]
  },
  "nsc": {
    "operator": "dchat-operator-bob",
    "account": "SYS", 
    "user": "bob-user"
  }
}
EOF

echo ""
echo "🔑 NSC密钥工作流程："
echo ""
echo "第1步: 启动Alice实例"
echo "  - NSC自动生成: operator -> account -> user"
echo "  - 自动导出: alice-user.seed (聊天私钥)"
echo "  - 自动生成: alice-user.creds (NATS认证)"
echo ""
echo "第2步: 启动Bob实例" 
echo "  - NSC自动生成: operator -> account -> user"
echo "  - 自动导出: bob-user.seed (聊天私钥)"
echo "  - 自动生成: bob-user.creds (NATS认证)"
echo ""
echo "第3步: 交换NSC公钥"
echo "  - Alice添加Bob的NSC公钥: AddFriendNSCKey(\"bob\", \"U...\")"
echo "  - Bob添加Alice的NSC公钥: AddFriendNSCKey(\"alice\", \"U...\")"
echo ""
echo "第4步: 开始聊天"
echo "  - 应用自动使用NSC派生的X25519密钥对"
echo "  - 端到端加密通信，密钥管理完全透明"

echo ""
echo "🖥️ 桌面客户端测试步骤："
echo ""
echo "1. 构建应用:"
echo "   cd /path/to/DecentralizedChat"
echo "   wails build"
echo ""
echo "2. 启动Alice实例:"
echo "   HOME=\$(pwd)/alice ./build/bin/DecentralizedChat"
echo ""
echo "3. 在Alice界面中:"
echo "   - 设置昵称: Alice" 
echo "   - 记录NSC公钥 (将在日志中显示)"
echo ""
echo "4. 启动Bob实例:"
echo "   HOME=\$(pwd)/bob ./build/bin/DecentralizedChat"
echo ""
echo "5. 在Bob界面中:"
echo "   - 设置昵称: Bob"
echo "   - 记录NSC公钥"
echo ""
echo "6. 建立好友关系:"
echo "   - Alice: 添加好友 -> 输入Bob的NSC公钥"
echo "   - Bob: 添加好友 -> 输入Alice的NSC公钥"
echo ""
echo "7. 开始私聊:"
echo "   - Alice: 开始私聊 -> 输入 'bob'"
echo "   - 发送消息测试端到端加密"

echo ""
echo "🔍 密钥验证:"
echo ""
echo "检查NSC生成的文件:"
echo "- alice/.dchat/: 配置和NSC密钥文件"
echo "- bob/.dchat/: 配置和NSC密钥文件"
echo ""
echo "验证聊天密钥派生:"
echo "- 两个用户的X25519密钥从各自的NSC seed确定性派生"
echo "- 公钥通过NSC公钥字符串的SHA256派生"
echo "- 私钥通过NSC seed的SHA256派生"

echo ""
echo "🌐 Windows版本测试:"
echo ""
echo "1. 构建Windows版本:"
echo "   wails build -platform windows/amd64"
echo ""
echo "2. 复制到Windows机器:"
echo "   scp build/bin/DecentralizedChat.exe user@windows-machine:/"
echo ""
echo "3. 在不同机器上运行:"
echo "   - 修改配置文件中的IP地址为实际IP"
echo "   - 确保防火墙开放4222/4223和6222/6223端口"
echo "   - 按照上述步骤进行测试"

echo ""
echo "✅ 测试配置已准备完成！"
echo ""
echo "📝 主要优势:"
echo "1. 统一密钥管理: NSC身份 = 聊天加密"
echo "2. 零手动密钥生成: 全部自动化"
echo "3. 确定性派生: 相同NSC seed始终生成相同聊天密钥"
echo "4. 简化用户体验: 只需交换NSC公钥即可"
echo ""
echo "🚀 现在可以开始测试了！"

# 返回原目录
cd ..

echo ""
echo "测试环境已在 test-nsc-setup/ 目录中准备好"
echo "请按照上述步骤进行完整功能测试"
