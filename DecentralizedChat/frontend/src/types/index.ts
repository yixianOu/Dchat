// 导入 Wails 生成的类型
import { chat } from '../../wailsjs/go/models';

// Chat 相关类型定义
export interface Message {
  id: string;
  username: string;
  content: string;
  timestamp: number;
  isGroup: boolean;
  chatId: string; // cid for direct, gid for group
}

// 使用 Wails 生成的 User 类型
export type User = chat.User;

export interface DecryptedMessage {
  CID: string;      // conversation id or group id
  Sender: string;   // sender user id
  Ts: string;       // timestamp (ISO string)
  Plain: string;    // decrypted content
  IsGroup: boolean; // is group message
  Subject: string;  // NATS subject
  // 注意：后端还有 RawWire 字段，前端暂不需要
}

export interface ChatRoomProps {
  roomName: string;
  sessionId: string;
  isGroup?: boolean;
  messages: DecryptedMessage[];
}

// 聊天会话信息
export interface ChatSession {
  id: string;
  name: string;
  isGroup: boolean;
  lastMessage?: string;
  lastTime?: number;
}

// 好友信息
export interface Friend {
  id: string;
  nickname: string;
  publicKey: string;
}

// 群组信息
export interface Group {
  id: string;
  name: string;
  symmetricKey: string;
}

// Wails 事件回调类型
export type EventCallback<T = any> = (data: T) => void;
