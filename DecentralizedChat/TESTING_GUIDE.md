# DChat 桌面客户端完整功能测试指南

## 📋 测试前准备

### 环境要求
- Go 1.21+
- Node.js 18+
- Wails CLI v2.10+
- 至少2台计算机或虚拟机（用于多节点测试）

### 构建应用
```bash
# Linux版本
wails build

# Windows版本 (交叉编译)
wails build -platform windows/amd64

# macOS版本 (交叉编译)
wails build -platform darwin/universal
```

---

## 🔥 阶段1: 单节点基础功能测试

### 1.1 应用启动测试
```bash
# 启动应用
./build/bin/DecentralizedChat

# 或者开发模式
wails dev
```

**预期结果**:
- ✅ 应用正常启动，显示主界面
- ✅ 控制台显示 "DChat application started (minimal mode)"
- ✅ 用户界面显示默认昵称 "Anonymous"
- ✅ 网络状态显示 🟢 在线

### 1.2 用户信息设置测试
**测试步骤**:
1. 点击 "设置" 按钮
2. 修改昵称为 "测试用户1"
3. 点击 "保存"

**预期结果**:
- ✅ 昵称成功更新显示
- ✅ 配置保存到 `~/.dchat/config.json`

### 1.3 密钥管理测试
**测试步骤**:
1. 点击 "密钥管理" 按钮
2. 生成新的密钥对
3. 复制公钥和私钥

**预期结果**:
- ✅ 成功生成Base64编码的密钥对
- ✅ 私钥长度约44字符，公钥长度约44字符
- ✅ 密钥可以正常复制到剪贴板

### 1.4 网络状态监控测试
**测试步骤**:
1. 观察侧边栏网络状态显示
2. 等待30秒查看状态更新

**预期结果**:
- ✅ 显示 🟢 在线状态
- ✅ 节点数显示 1
- ✅ 消息统计正常更新

---

## 🌐 阶段2: 双节点Routes连接测试

### 2.1 准备第二个节点

**在第二台机器上**:
```bash
# 复制应用到第二台机器
scp build/bin/DecentralizedChat user@machine2:/path/to/dchat/

# 或使用Windows版本
copy DecentralizedChat.exe \\machine2\path\
```

### 2.2 配置种子节点连接

**节点1 (种子节点)**:
```bash
# 启动应用，记录IP地址
# 假设节点1 IP: 192.168.1.100
```

**节点2 (连接节点)**:
编辑配置文件 `~/.dchat/config.json`:
```json
{
  "network": {
    "auto_discovery": true,
    "seed_routes": ["nats://192.168.1.100:6222"],
    "local_ip": "192.168.1.101"
  }
}
```

### 2.3 验证Routes连接
**测试步骤**:
1. 先启动节点1
2. 等待5秒后启动节点2
3. 检查两个节点的网络状态

**预期结果**:
- ✅ 节点1显示 "节点: 2"
- ✅ 节点2显示 "节点: 2"
- ✅ 两个节点都显示 🟢 在线状态

---

## 🔐 阶段3: 端到端加密通信测试

### 3.1 用户身份设置
**节点1**:
- 昵称: "Alice"
- 生成密钥对 A (记录公钥)

**节点2**:
- 昵称: "Bob"  
- 生成密钥对 B (记录公钥)

### 3.2 好友密钥交换
**节点1 (Alice)**:
1. 点击 "添加好友"
2. 输入好友ID: "Bob的用户ID"
3. 输入好友公钥: "Bob的公钥"

**节点2 (Bob)**:
1. 点击 "添加好友"
2. 输入好友ID: "Alice的用户ID"
3. 输入好友公钥: "Alice的公钥"

### 3.3 私聊通信测试
**测试步骤**:
1. Alice点击 "开始私聊"
2. 输入Bob的用户ID
3. 发送消息: "Hello Bob!"
4. Bob查看收到的消息
5. Bob回复: "Hi Alice!"

**预期结果**:
- ✅ 消息成功加密传输
- ✅ 对方能正确解密并显示消息
- ✅ 会话列表正确显示对话
- ✅ 消息时间戳正确

---

## 👥 阶段4: 群聊功能测试

### 4.1 创建群聊密钥
**测试步骤**:
1. 生成256位AES密钥 (32字节Base64编码)
2. 通过安全渠道分享给所有群成员

**示例密钥生成**:
```bash
# 生成群聊密钥
openssl rand -base64 32
```

### 4.2 群聊配置
**所有节点**:
1. 点击 "加入群组"
2. 输入群组ID: "test-group-001"
3. 输入相同的对称密钥

### 4.3 群聊通信测试
**测试步骤**:
1. Alice发送群消息: "大家好！"
2. Bob回复: "Hello everyone!"
3. 验证所有群成员都能收到消息

**预期结果**:
- ✅ 群消息正确加密传输
- ✅ 所有成员能解密并显示消息
- ✅ 群聊会话正确显示

---

## 🚀 阶段5: 高级功能测试

### 5.1 会话ID一致性测试
**测试步骤**:
1. 在前端开发者工具中查看网络请求
2. 验证CID计算的一致性
3. 检查消息路由的正确性

### 5.2 网络容错测试
**测试步骤**:
1. 断开一个节点的网络连接
2. 恢复连接
3. 验证自动重连功能

**预期结果**:
- ✅ 断开时状态显示 🔴 离线
- ✅ 重连后自动恢复 🟢 在线
- ✅ 历史消息不丢失

### 5.3 密钥持久化测试
**测试步骤**:
1. 添加好友密钥
2. 重启应用
3. 验证密钥是否恢复

**预期结果**:
- ✅ 重启后密钥自动恢复
- ✅ 无需重新添加好友密钥
- ✅ 历史会话正常显示

---

## 🔧 Windows跨平台编译和部署

### 编译Windows版本

```bash
# 在Linux/macOS上交叉编译Windows版本
wails build -platform windows/amd64

# 编译结果
ls build/bin/
# DecentralizedChat.exe
```

### Windows环境配置

**Windows节点配置**:
```json
// %USERPROFILE%\.dchat\config.json
{
  "user": {
    "id": "windows-user-001",
    "nickname": "Windows用户"
  },
  "network": {
    "auto_discovery": true,
    "seed_routes": ["nats://192.168.1.100:6222"],
    "local_ip": "192.168.1.102"
  }
}
```

### 防火墙配置
```powershell
# Windows防火墙规则
netsh advfirewall firewall add rule name="DChat Client" dir=in action=allow protocol=TCP localport=4222
netsh advfirewall firewall add rule name="DChat Cluster" dir=in action=allow protocol=TCP localport=6222
```

---

## 📊 测试结果检查清单

### ✅ 基础功能
- [ ] 应用正常启动
- [ ] 用户设置保存
- [ ] 密钥生成和管理
- [ ] 网络状态监控

### ✅ 网络连接
- [ ] 单节点运行正常
- [ ] 双节点Routes连接成功
- [ ] 网络状态正确显示
- [ ] 自动重连功能

### ✅ 加密通信
- [ ] 私聊消息加密传输
- [ ] 群聊消息加密传输
- [ ] 密钥正确管理
- [ ] 消息正确解密

### ✅ 跨平台兼容
- [ ] Linux版本正常运行
- [ ] Windows版本正常运行
- [ ] 跨平台通信正常
- [ ] 配置文件兼容

---

## 🐛 常见问题排除

### 连接问题
```bash
# 检查端口是否被占用
netstat -tulnp | grep :4222
netstat -tulnp | grep :6222

# 检查防火墙设置
sudo ufw status
```

### 密钥问题
```bash
# 检查密钥存储
ls ~/.dchat/
cat ~/.dchat/config.json
```

### 日志调试
```bash
# 启动应用并查看详细日志
./DecentralizedChat 2>&1 | tee dchat.log
```

---

## 🎯 测试成功标准

**完整功能测试通过标准**:
1. ✅ 所有基础功能正常
2. ✅ 多节点网络连接稳定
3. ✅ 端到端加密通信成功
4. ✅ 跨平台兼容性良好
5. ✅ 网络容错和恢复正常

达到以上标准即可认为应用达到生产部署就绪状态！🎉
