// DChatAPI - 对应后端 app.go 的接口
import { User, DecryptedMessage } from '../types';

// 导入 Wails 运行时函数
declare global {
  interface Window {
    go: {
      main: {
        App: {
          SetUserInfo: (nickname: string) => Promise<void>;
          GetUser: () => Promise<User>;
          SetKeyPair: (privB64: string, pubB64: string) => Promise<void>;
          AddFriendKey: (uid: string, pubB64: string) => Promise<void>;
          AddGroupKey: (gid: string, symB64: string) => Promise<void>;
          JoinDirect: (peerID: string) => Promise<void>;
          JoinGroup: (gid: string) => Promise<void>;
          SendDirect: (peerID: string, content: string) => Promise<void>;
          SendGroup: (gid: string, content: string) => Promise<void>;
          OnDecrypted: (callback: (msg: DecryptedMessage) => void) => Promise<void>;
          OnError: (callback: (error: string) => void) => Promise<void>;
        };
      };
    };
  }
}

// 用户管理
export const setUserInfo = (nickname: string): Promise<void> => {
  return window.go?.main?.App?.SetUserInfo(nickname) || Promise.resolve();
};

export const getUser = (): Promise<User> => {
  return window.go?.main?.App?.GetUser() || Promise.resolve({ id: '', nickname: '' });
};

export const setKeyPair = (privB64: string, pubB64: string): Promise<void> => {
  return window.go?.main?.App?.SetKeyPair(privB64, pubB64) || Promise.resolve();
};

// 密钥管理
export const addFriendKey = (uid: string, pubB64: string): Promise<void> => {
  return window.go?.main?.App?.AddFriendKey(uid, pubB64) || Promise.resolve();
};

export const addGroupKey = (gid: string, symB64: string): Promise<void> => {
  return window.go?.main?.App?.AddGroupKey(gid, symB64) || Promise.resolve();
};

// 聊天功能
export const joinDirect = (peerID: string): Promise<void> => {
  return window.go?.main?.App?.JoinDirect(peerID) || Promise.resolve();
};

export const joinGroup = (gid: string): Promise<void> => {
  return window.go?.main?.App?.JoinGroup(gid) || Promise.resolve();
};

export const sendDirect = (peerID: string, content: string): Promise<void> => {
  return window.go?.main?.App?.SendDirect(peerID, content) || Promise.resolve();
};

export const sendGroup = (gid: string, content: string): Promise<void> => {
  return window.go?.main?.App?.SendGroup(gid, content) || Promise.resolve();
};

// 事件监听
export const onDecrypted = (callback: (msg: DecryptedMessage) => void): Promise<void> => {
  return window.go?.main?.App?.OnDecrypted(callback) || Promise.resolve();
};

export const onError = (callback: (error: string) => void): Promise<void> => {
  return window.go?.main?.App?.OnError(callback) || Promise.resolve();
};
