# Wails 自动绑定生成指南

## 概述

Wails 框架可以自动分析 Go 后端代码，生成对应的 TypeScript 绑定文件，让前端能够直接调用后端方法。

## 自动生成的文件

### 1. 类型定义文件 (`wailsjs/go/main/App.d.ts`)
```typescript
// 自动生成，包含所有导出方法的 TypeScript 声明
export function AddFriendKey(arg1:string,arg2:string):Promise<void>;
export function GetUser():Promise<chat.User>;
// ... 其他方法
```

### 2. 实现文件 (`wailsjs/go/main/App.js`)  
```javascript
// 自动生成，通过 window.go 调用后端方法
export function AddFriendKey(arg1, arg2) {
  return window['go']['main']['App']['AddFriendKey'](arg1, arg2);
}
```

### 3. 模型类型 (`wailsjs/go/models.ts`)
```typescript
// 自动生成，包含 Go 结构体对应的 TypeScript 类型
export namespace chat {
  export class User {
    id: string;
    nickname: string;
    // ...
  }
}
```

## 绑定生成机制

### 1. 何时生成
- 运行 `wails dev` 时自动生成
- 运行 `wails build` 时自动生成  
- Go 代码变更后重新启动开发服务器时更新

### 2. 生成规则
- **结构体方法**：所有导出的 struct 方法都会生成对应的 TypeScript 函数
- **参数类型**：Go 基础类型自动映射到 TypeScript 类型
- **返回类型**：所有方法都包装为 Promise
- **结构体类型**：Go struct 自动生成对应的 TypeScript class

### 3. 类型映射

| Go 类型           | TypeScript 类型          |
| ----------------- | ------------------------ |
| `string`          | `string`                 |
| `int`, `int64`    | `number`                 |
| `bool`            | `boolean`                |
| `[]Type`          | `Type[]`                 |
| `map[string]Type` | `{[key: string]: Type}`  |
| `struct`          | `class`                  |
| `error`           | `Promise<void>` (throws) |

## 使用示例

### 后端 Go 代码
```go
// app.go
type App struct {
    // ...
}

func (a *App) SetUserInfo(nickname string) error {
    // 实现
    return nil
}

func (a *App) GetUser() (chat.User, error) {
    // 实现
    return user, nil
}
```

### 自动生成的 TypeScript 绑定
```typescript
// wailsjs/go/main/App.d.ts
export function SetUserInfo(arg1: string): Promise<void>;
export function GetUser(): Promise<chat.User>;
```

### 前端使用
```typescript
import { SetUserInfo, GetUser } from '../wailsjs/go/main/App';
import { chat } from '../wailsjs/go/models';

// 直接调用
await SetUserInfo('Alice');
const user: chat.User = await GetUser();
```

## 最佳实践

### 1. 直接使用生成的绑定
```typescript
// ✅ 推荐：直接导入使用
import { SetUserInfo } from '../wailsjs/go/main/App';

// ❌ 不推荐：手动包装
const setUserInfo = (nickname: string) => {
  return window.go.main.App.SetUserInfo(nickname);
};
```

### 2. 类型安全
```typescript
// ✅ 使用生成的类型
import { chat } from '../wailsjs/go/models';
const user: chat.User = await GetUser();

// ❌ 使用 any 类型
const user: any = await GetUser();
```

### 3. 错误处理
```typescript
try {
  await SetUserInfo('Alice');
} catch (error) {
  console.error('设置用户信息失败:', error);
}
```

## 开发工作流

### 1. 修改后端代码
```go
// 添加新方法到 App struct
func (a *App) NewMethod(param string) (Result, error) {
    // 实现
}
```

### 2. 重启开发服务器
```bash
# Ctrl+C 停止，然后重新启动
wails dev
```

### 3. 使用新生成的绑定
```typescript
// 新方法自动可用
import { NewMethod } from '../wailsjs/go/main/App';
await NewMethod('parameter');
```

## 调试技巧

### 1. 查看生成的绑定
检查 `wailsjs/go/main/App.d.ts` 确认方法是否正确生成

### 2. 跳过绑定生成（调试用）
```bash
wails dev --skipbindings
```

### 3. 清理重新生成
```bash
# 删除 wailsjs 目录
rm -rf frontend/wailsjs
# 重新运行
wails dev
```

## 注意事项

1. **不要手动编辑**：`wailsjs/` 目录下的文件是自动生成的，不要手动修改
2. **版本控制**：建议将 `wailsjs/` 目录加入 `.gitignore`，因为它可以重新生成
3. **类型同步**：确保前后端类型定义保持同步
4. **错误处理**：Go 方法返回的 error 会转换为 Promise rejection

## 与手动包装的对比

| 方式     | 优点                         | 缺点                 |
| -------- | ---------------------------- | -------------------- |
| 自动绑定 | 类型安全、自动同步、减少维护 | 依赖 Wails 工具链    |
| 手动包装 | 完全控制、可定制             | 维护成本高、容易出错 |

**结论**：推荐使用 Wails 自动生成的绑定，它提供了更好的类型安全性和开发体验。
