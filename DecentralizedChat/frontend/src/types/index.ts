// Chat 相关类型定义
export interface Message {
  id: string;
  username: string;
  content: string;
  timestamp: number;
  room_id: string;
}

export interface User {
  id: string;
  nickname: string;
  avatar?: string;
}

export interface TailscaleStatus {
  connected: boolean;
  ip: string;
}

export interface ChatRoomProps {
  roomName: string;
}

// 事件类型定义
export interface MessageEvent {
  room_id: string;
  id: string;
  username: string;
  content: string;
  timestamp: number;
}

export interface UsersUpdateEvent {
  users: User[];
}

// Wails 事件回调类型
export type EventCallback<T = any> = (data: T) => void;
