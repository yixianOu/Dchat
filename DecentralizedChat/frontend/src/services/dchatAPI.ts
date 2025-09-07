// DChatAPI - 使用 Wails 自动生成的绑定
import { 
  SetUserInfo,
  GetUser,
  AddFriendKey,
  AddGroupKey,
  LoadNSCKeys,
  GenerateSSLCertificate,  // ✅ 新增的证书生成功能
  GetAllDerivedKeys,       // ✅ 新增的密钥获取功能
  JoinDirect,
  JoinGroup,
  SendDirect,
  SendGroup,
  GetConversationID,  // ✅ 新增功能，已生成
  GetNetworkStatus    // ✅ 新增功能，已生成
} from '../../wailsjs/go/main/App';

import { EventsOn } from '../../wailsjs/runtime/runtime';
import { chat } from '../../wailsjs/go/models';
import { DecryptedMessage } from '../types';

// 重新导出 Wails 生成的函数，保持 API 一致性
export const setUserInfo = SetUserInfo;
export const getUser = GetUser;
export const addFriendKey = AddFriendKey;
export const addGroupKey = AddGroupKey;
export const joinDirect = JoinDirect;
export const joinGroup = JoinGroup;
export const sendDirect = SendDirect;
export const sendGroup = SendGroup;
export const getConversationID = GetConversationID;  // ✅ 新增功能
export const getNetworkStatus = GetNetworkStatus;    // ✅ 新增功能
export const loadNSCKeys = LoadNSCKeys;              // ✅ 新的密钥加载方式
export const generateSSLCertificate = GenerateSSLCertificate;  // ✅ 证书生成
export const getAllDerivedKeys = GetAllDerivedKeys;  // ✅ 密钥获取

// ⭐ 基于事件的监听器，替代回调方式
export const onDecrypted = (callback: (msg: DecryptedMessage) => void): (() => void) => {
  return EventsOn('message:decrypted', callback);
};

export const onError = (callback: (error: { error: string; timestamp: string }) => void): (() => void) => {
  return EventsOn('message:error', callback);
};

// 导出 Wails 生成的类型
export type WailsUser = chat.User;
