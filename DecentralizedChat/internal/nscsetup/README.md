## NATS 去中心化认证三要素：NKEY、JWT、Creds

参考文档（已研读并抽炼，未逐字复制）：https://docs.nats.io/running-a-nats-service/nats_admin/security/jwt

### 1. NKEY 是什么
NKEY 是基于 Ed25519 的公私钥对，经过前缀标注 + Base32 编码 + CRC 校验：
* 公钥前缀：O(Operator)、A(Account)、U(User) —— 标识身份层级。
* 种子(Seed)私钥前缀：SO / SA / SU，S 代表 seed，后缀与公钥类型对应。
* 私钥仅保存在客户端或签发端，服务端永远不接触私钥。

核心作用：
1. 连接挑战签名（服务器下发一次性 nonce，客户端用私钥签名，服务器用公钥验签）。
2. 为上层 JWT 做身份根（每个 JWT 的 subject == 该实体的公钥）。
3. 作为签发链中的签名密钥（Operator / Account 可使用"签名密钥"轮换，而不暴露身份密钥）。

### 2. JWT 是什么
NATS 定制的 JWT 载荷，把"配置 + 权限 + 身份公钥"打包：
* Operator JWT：全域/拓扑级别配置（系统账户 ID、账户 JWT server URL、可信签名键集合等）。
* Account JWT：账户级导入/导出、资源/连接/订阅/负载限制、缺省用户权限、撤销列表等。
* User JWT：用户自身权限（Pub/Sub 允许/禁止、限制、过期时间等），Issuer 指向其 Account。

链条（去中心化信任链）：Operator -> Account -> User。
验证要点：
1. User JWT 的签名用 Account (或其签名键) 验证；Account JWT 用 Operator (或其签名键) 验证；Operator JWT 由其自身身份 NKEY 验证。
2. 每个 JWT 的 `sub` 是其公钥身份；`iss` 是签发者（上一层身份或其签名键）。
3. 服务器无需保存私钥，只需：已信任的 Operator JWT + 获取 Account JWT 的途径(resolver) + 客户端提供的 User JWT 与签名证明。

去中心化意义：各层配置/权限由各自"拥有者"独立演进；服务器仅验证链条与策略，不强绑定集中式配置文件。

### 3. Creds 文件是什么
Creds 是"使用便利"格式：将 User JWT + 对应用户私钥种子（SU...）串接成单一文本（含 BEGIN/END 块）。
作用：方便 CLI / 客户端一次性加载，无需显式编写回调函数来提供 JWT 与签名过程。
安全考虑：因为包含私钥种子，Creds 必须像密码一样保护；一旦泄露可被完全冒充。可以改用回调（UserJWT + NKEY 签名函数）避免把种子落盘，或只在内存持有。

### 4. 三者关系概览
| 概念 | 含义 | 包含私钥? | 主要用途 | 传输/分发风险 |
|------|------|-----------|----------|---------------|
| NKEY 公钥 | 装饰后的 Ed25519 公钥 | 否 | 身份/签名验证 | 可公开 |
| NKEY 种子 | 私钥（SU/SO/SA 前缀） | 是 | 签名、连接挑战 | 必须严格保密 |
| JWT (Operator/Account/User) | 配置 + 公钥身份 + 签名 | 否(只含签名) | 分发策略/权限 | 内容可被查看（可能暴露主题/限制） |
| Creds | User JWT + User 种子 | 是 | 简化客户端认证 | 高敏感；最小化复制 |

### 5. 连接认证流程（User JWT 模式）
1. 客户端读取 creds（或通过代码获得 userJWT 与签名函数）。
2. 建立 TCP 后服务器发送 INFO（包含 nonce 且指明需认证）。
3. 客户端发送 CONNECT：携带 user JWT、签名后的 nonce（使用用户私钥）。
4. 服务器验证：
	* 校验 user JWT 签名（通过其 Account 公钥或签名键）。
	* 解析 user JWT -> 找到 Account 公钥 -> 通过 resolver 获取/缓存最新 Account JWT。
	* 验证 Account JWT 对 Operator JWT 的链条，检查撤销、过期、限制。
	* 使用 user 公钥验签 nonce，确认私钥持有。
5. 全部通过则连接建立，随后权限与限制来自 user JWT + account 限额。

### 6. Resolver 在去中心化中的角色
服务器不预装所有 Account JWT，而通过 resolver 获取：
* mem-resolver：静态少量账户；配置修改需重载。
* url-resolver：HTTP 拉取，新增无需重启。
* nats-resolver：推荐；通过 NATS 网络 gossip，同步与删除（需要持久目录）。

### 7. 签名键 (Signing Keys) 与轮换
* 身份 NKEY（identity）最关键；建议离线保存，生产中使用"签名键"进行常规签发。
* 删除或增加签名键 -> 对应层 JWT 更新并推送；下层 JWT 重新签发或保持有效（取决于层级）。
* 用户层没有签名键（由账户直接签发）。

### 8. 撤销与更新
* Account JWT 维护用户撤销列表与签名键集合；更新后通过 push/resolver 广播。
* User JWT 被重新签发（更高 iat）可覆盖之前限制；或通过 revocation 使旧 JWT 失效。

### 9. 本项目中的实现映射
代码位置：`internal/nscsetup/setup.go`
流程概述：
1. `EnsureSysAccountSetup` 首次运行：
	* 生成 / 编辑本地 Operator（含 system account，签名键要求）。
	* 创建 system account (默认 `SYS`) 下的默认用户 (默认 `sys`)。
	* 生成 nats-resolver 配置片段写入本地（`<account>_resolver.conf`）。
2. 解析并记录：
	* NKEY / Store 目录（`nsc env`）。
	* 查找并保存用户 creds 路径；按需导出用户种子（可进一步最小化保留策略）。
3. 将路径与名称持久化到 `config.Config`（仅保留用户层必要工件，已去除账户/操作员私钥追踪）。

核心函数与作用：
* `initNSCOperatorAndSys`：幂等创建 Operator + System Account 签名键要求。
* `generateResolverConfig`：生成 nats-resolver 需要的初始账户配置文件。
* `collectUserArtifacts`：描述用户 (`nsc describe user --json`) 提取 `sub` (即用户公钥)；收集 creds / 导出种子。
* `exportSeed`（限制 user）：使用 `nsc export keys --users` 生成临时种子文件并保存。

### 10. 安全实践建议
* 若客户端仅需连接，不再开发签发逻辑，考虑不落盘用户种子，只分发 creds（已包含种子则同样要保护）。
* 更高安全：避免 creds 文件（含私钥）散布，改用回调 `UserJWT + 签名函数`，将种子保存在安全模块或内存中。
* 对私钥目录做好离线备份和访问控制；身份 NKEY 与签名键分离，降低轮换成本。
* 监控并适时推送账户 JWT 更新（权限、撤销列表、签名键变更）。

### 11. 为什么 `sub` 可以直接视为公钥
根据官方文档：JWT subject (`sub`) 字段即该实体（Operator / Account / User）的公钥 NKEY，JWT 自身只包含签名与声明，不含私钥；签名验证链通过 `iss`（issuer）与上一层公钥建立。故在本项目中用 `describe user --json` 结果中的 `sub` 作为用户公钥来源是正确且规范的实现。

### 12. 简要对照表
| 层级 | JWT 主要内容 | `sub` | `iss` | 常见变更 | 客户端是否发送 |
|------|--------------|-------|-------|---------|----------------|
| Operator | 系统账户 ID、账户 JWT server、全局策略、签名键 | Operator 公钥 | Operator 公钥(自签) | 添加/移除签名键 | 否 |
| Account | 导入/导出、限额、撤销、默认权限、签名键 | Account 公钥 | Operator 身份或签名键 | 权限/限额/撤销/签名键 | 否（服务器自行获取） |
| User | 用户权限、限制、过期、发行账户引用 | User 公钥 | Account 身份或签名键 | 权限变更 / 重新签发 | 是（随连接） |

---
如果后续希望进一步"只保留 creds 不保留 seed"或实现签名回调方案，可在 `collectUserArtifacts` 中条件化 `exportSeed` 调用，或引入配置开关（TODO 方向）。
