# 群聊功能使用指南

## 功能特性
✅ **完全去中心化**：不需要任何中心服务，和私聊逻辑一致
✅ **极简操作**：创建群只需点击按钮，加入群只需输入GID和密钥
✅ **端到端加密**：256位AES-GCM加密，只有群成员能解密
✅ **自动处理**：密钥生成、存储、订阅群主题全自动化

## 前端API使用

### 1. 创建群聊
```typescript
import { createGroup } from './services/dchatAPI';

// 点击创建群聊按钮
async function onCreateGroup() {
  try {
    const { gid, groupKey } = await createGroup();
    // gid是群ID，groupKey是群密钥
    // 展示给用户，让用户分享给要加入的人
    alert(`群创建成功！\n群ID: ${gid}\n群密钥: ${groupKey}`);
  } catch (err) {
    console.error('创建失败:', err);
  }
}
```

### 2. 加入群聊
```typescript
import { joinGroup } from './services/dchatAPI';

// 用户输入群ID和密钥后调用
async function onJoinGroup(gid: string, groupKey: string) {
  try {
    await joinGroup(gid, groupKey);
    alert('加入群成功！');
  } catch (err) {
    console.error('加入失败:', err);
    alert('加入失败，请检查群ID和密钥是否正确');
  }
}
```

### 3. 发送群消息
和私聊完全一致，自动加密：
```typescript
import { sendGroup } from './services/dchatAPI';

await sendGroup(gid, '这是群消息');
```

## UI操作流程

### 创建群
1. 点击侧边栏"创建群聊"按钮
2. 自动生成群ID和256位AES密钥
3. 将群ID和密钥分享给要加入的好友（可通过任意渠道）
4. 自动加入群，可直接发送消息

### 加入群
1. 点击侧边栏"加入群组"按钮
2. 输入好友分享的群ID和群密钥
3. 自动验证并加入群，可直接接收和发送消息

## 安全说明
- 群密钥是256位真随机数，暴力破解完全不可能
- 群密钥只有群成员知道，传输过程中需要用户自行保证安全（建议线下/加密渠道分享）
- 退群只需本地删除群密钥即可，不需要通知任何人
- 群消息持久化到本地SQLite，离线可查看历史记录
