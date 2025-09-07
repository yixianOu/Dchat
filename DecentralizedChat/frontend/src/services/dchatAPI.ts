// DChatAPI - 使用 Wails 自动生成的绑定
import { 
  SetUserInfo,
  GetUser,
  SetKeyPair,
  AddFriendKey,
  AddGroupKey,
  JoinDirect,
  JoinGroup,
  SendDirect,
  SendGroup,
  OnDecrypted,
  OnError
} from '../../wailsjs/go/main/App';

import { chat } from '../../wailsjs/go/models';
import { DecryptedMessage } from '../types';

// 重新导出 Wails 生成的函数，保持 API 一致性
export const setUserInfo = SetUserInfo;
export const getUser = GetUser;
export const setKeyPair = SetKeyPair;
export const addFriendKey = AddFriendKey;
export const addGroupKey = AddGroupKey;
export const joinDirect = JoinDirect;
export const joinGroup = JoinGroup;
export const sendDirect = SendDirect;
export const sendGroup = SendGroup;

// 事件监听器需要类型包装
export const onDecrypted = (callback: (msg: DecryptedMessage) => void): Promise<void> => {
  return OnDecrypted(callback);
};

export const onError = (callback: (error: string) => void): Promise<void> => {
  return OnError(callback);
};

// 导出 Wails 生成的类型
export type WailsUser = chat.User;
