// DChatAPI - 使用 Wails 自动生成的绑定
import {
  SetUserInfo,
  GetUser,
  AddFriendKey,
  AddFriendNSCKey,
  AddGroupKey,
  JoinDirect,
  JoinGroup,
  CreateGroup,
  SendDirect,
  SendGroup,
  GetConversationID,  // ✅ 新增功能，已生成
  GetNetworkStatus,   // ✅ 新增功能，已生成
  GetConversation,
  GetMessages,
  MarkAsRead,
  SearchMessages,
  GetAllConversations, // ✅ 新增：获取所有会话列表
  GetUserNSCPublicKey // ✅ 新增：获取当前用户NSC公钥
} from '../../wailsjs/go/main/App';

import { EventsOn } from '../../wailsjs/runtime/runtime';
import { chat } from '../../wailsjs/go/models';
import { DecryptedMessage } from '../types';

// CreateGroup 返回值类型
export interface CreateGroupResult {
  gid: string;
  groupKey: string;
}

// 重新导出 Wails 生成的函数，保持 API 一致性
export const setUserInfo = SetUserInfo;
export const getUser = GetUser;
export const addFriendKey = AddFriendKey;
export const addFriendNSCKey = AddFriendNSCKey as (nscPubKey: string) => Promise<string>; // ✅ 新增：通过NSC公钥添加好友，返回自动派生的好友ID
export const addGroupKey = AddGroupKey;
export const joinDirect = JoinDirect;
export const joinGroup = JoinGroup as (gid: string, groupKey: string) => Promise<void>; // ✅ 新的带密钥的JoinGroup
export const createGroup = CreateGroup as unknown as () => Promise<CreateGroupResult>; // ✅ 新增：创建群聊
export const sendDirect = SendDirect;
export const sendGroup = SendGroup;
export const getConversationID = GetConversationID;
export const getNetworkStatus = GetNetworkStatus;
export const getConversation = GetConversation; // ✅ 新增：获取会话信息
export const getMessages = GetMessages; // ✅ 新增：获取消息历史
export const markAsRead = MarkAsRead; // ✅ 新增：标记已读
export const searchMessages = SearchMessages; // ✅ 新增：搜索消息
export const getAllConversations = GetAllConversations; // ✅ 新增：获取所有会话列表
export const getUserNSCPublicKey = GetUserNSCPublicKey; // ✅ 新增：获取当前用户NSC公钥

// ⭐ 基于事件的监听器，替代回调方式
export const onDecrypted = (callback: (msg: DecryptedMessage) => void): (() => void) => {
  return EventsOn('message:decrypted', callback);
};

export const onError = (callback: (error: { error: string; timestamp: string }) => void): (() => void) => {
  return EventsOn('message:error', callback);
};

// 导出 Wails 生成的类型
export type WailsUser = chat.User;
