# 自建授权NATS Hub指南

本指南用于部署一个需要JWT授权的公网Hub，只有持有有效凭证的LeafNode才能连接，保障公网Hub的安全。

---

## 一、前置条件
1. 本地已经完成Dchat的初始化运行，生成了NSC配置文件
2. 有一台公网服务器，开放端口：
   - 4222：NATS客户端端口（可选，仅当需要直接连接Hub时开放）
   - 7422：LeafNode连接端口（必须开放）

---

## 二、从本地复制需要的文件
本地配置文件默认路径：`~/.dchat/`

### 需要复制到服务器的文件（2个）
| 本地路径 | 说明 | 敏感级别 |
|---------|------|----------|
| `~/.dchat/simple_resolver.conf` | Resolver配置文件，包含Operator JWT（公钥信息）和认证规则 | 公开 |
| `~/.dchat/accounts/` 目录 | 所有账户的JWT文件 | 公开 |

### ❌ 绝对不要复制到服务器的文件
- `~/.dchat/operator.nk`：Operator私钥，是最高权限密钥，必须本地离线保存
- `~/.dchat/account.nk`：账户私钥，本地保存即可
- `~/.dchat/user.nk` / `user.seed` / `user.creds`：用户个人凭证，属于用户隐私

---

## 三、服务器配置步骤
### 1. 上传文件到服务器
将上述2个文件上传到服务器的 `/etc/nats/` 目录（可自定义路径）：
```bash
# 服务器上创建目录
mkdir -p /etc/nats/accounts
```
上传后目录结构：
```
/etc/nats/
├── simple_resolver.conf
├── accounts/
│   └── ACxxxxxxxxxxxxxxxxxxxxxxxxxxxx.jwt  # 你的账户JWT文件
```

### 2. 修正resolver配置路径
上传`simple_resolver.conf`到服务器后，修改其中的`dir`路径为服务器上accounts目录的实际路径：
```yaml
# 修改simple_resolver.conf
resolver {
  type: full
  dir: "/etc/nats/accounts"  # 改为服务器上accounts目录的绝对路径，或者用相对路径"./accounts"
}
```

### 3. 编写Hub配置文件
在服务器上创建 `/etc/nats/hub.conf`：
```yaml
# Hub基本配置
port: 4222
host: 0.0.0.0

# LeafNode连接配置（必须配置）
leafnodes: {
  host: 0.0.0.0
  port: 7422
  # 可选：限制仅指定账户可以连接，增强安全性
  # allowed_accounts: ["ACxxxxxxxxxxxxxxxxxxxxxxxxxxxx"]
}

# 加载认证配置（必须）
include simple_resolver.conf

# 可选：开启日志
debug: false
trace: false
logtime: true
```

### 3. 启动Hub
#### 方式1：直接启动
```bash
cd /etc/nats && nats-server -c hub.conf
```

#### 方式2：systemd后台运行
创建 `/etc/systemd/system/nats-hub.service`：
```ini
[Unit]
Description=NATS Hub Server
After=network.target

[Service]
Type=simple
ExecStart=/usr/local/bin/nats-server -c /etc/nats/hub.conf
Restart=always
RestartSec=5
User=root

[Install]
WantedBy=multi-user.target
```
启动并设置开机自启：
```bash
systemctl daemon-reload
systemctl enable --now nats-hub
```

---

## 四、更新账户信息

将本地`~/.dchat/accounts/`下新生成的JWT文件上传到服务器的`/etc/nats/accounts/`目录即可，无需重启Hub，会自动加载。

---

## 五、客户端配置
用户需要修改Dchat配置中的Hub地址为你的自建Hub：
1. 打开 `~/.dchat/config.json`
2. 修改 `leafnode.hub_urls` 为：
   ```json
   "hub_urls": ["nats://<你的Hub公网IP>:7422"]
   ```
3. 重启Dchat即可自动连接到自建Hub。

---

## 六、验证部署是否成功

如果不填`CredsFile`字段，LeafNode连接Hub时会直接报错：`authorization violation`，说明授权机制生效。

---

## 安全建议
1. Operator私钥(`operator.nk`)必须离线安全保存，不要泄露，一旦泄露整个Hub的认证体系将失效
2. 定期备份服务器上的`accounts/`目录，避免账户数据丢失
3. 生产环境建议开启TLS加密LeafNode连接，参考NATS官方TLS配置文档
