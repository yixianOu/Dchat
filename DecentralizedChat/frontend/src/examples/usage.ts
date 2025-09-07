// 去中心化聊天应用使用示例

import { 
  setUserInfo, 
  setKeyPair, 
  addFriendKey, 
  addGroupKey, 
  joinDirect, 
  joinGroup, 
  sendDirect, 
  sendGroup,
  onDecrypted,
  onError 
} from '../services/dchatAPI';
import { DecryptedMessage } from '../types';

// 示例：完整的聊天应用使用流程

async function initializeApp() {
  try {
    // 1. 设置用户信息
    await setUserInfo('Alice');
    
    // 2. 设置密钥对（实际应用中应该生成真实的密钥对）
    const privateKey = 'base64EncodedPrivateKey';
    const publicKey = 'base64EncodedPublicKey';
    await setKeyPair(privateKey, publicKey);
    
    // 3. 设置消息监听
    await onDecrypted((message: DecryptedMessage) => {
      console.log('收到消息:', {
        发送者: message.Sender,
        内容: message.Plain,
        时间: message.Ts,
        类型: message.IsGroup ? '群聊' : '私聊'
      });
    });
    
    // 4. 设置错误监听
    await onError((error: string) => {
      console.error('聊天错误:', error);
    });
    
    console.log('应用初始化完成');
  } catch (error) {
    console.error('初始化失败:', error);
  }
}

async function startDirectChat() {
  try {
    // 1. 添加好友公钥
    const friendId = 'user_12345';
    const friendPublicKey = 'base64EncodedFriendPublicKey';
    await addFriendKey(friendId, friendPublicKey);
    
    // 2. 加入私聊
    await joinDirect(friendId);
    
    // 3. 发送消息
    await sendDirect(friendId, 'Hello, this is a private message!');
    
    console.log('私聊已建立');
  } catch (error) {
    console.error('私聊建立失败:', error);
  }
}

async function joinGroupChat() {
  try {
    // 1. 添加群组对称密钥
    const groupId = 'group_abcdef';
    const groupSymmetricKey = 'base64EncodedGroupSymmetricKey';
    await addGroupKey(groupId, groupSymmetricKey);
    
    // 2. 加入群组
    await joinGroup(groupId);
    
    // 3. 发送群组消息
    await sendGroup(groupId, 'Hello everyone!');
    
    console.log('群聊已加入');
  } catch (error) {
    console.error('群聊加入失败:', error);
  }
}

// 密钥生成示例（实际应该使用真实的加密库）
function generateKeyPair() {
  // 注意：这只是示例，实际应用应使用真实的加密库如 libsodium
  const privateKey = btoa(crypto.getRandomValues(new Uint8Array(32)).join(''));
  const publicKey = btoa(crypto.getRandomValues(new Uint8Array(32)).join(''));
  
  return { privateKey, publicKey };
}

function generateSymmetricKey() {
  // 注意：这只是示例，实际应用应使用真实的加密库
  return btoa(crypto.getRandomValues(new Uint8Array(32)).join(''));
}

// 使用示例
export {
  initializeApp,
  startDirectChat,
  joinGroupChat,
  generateKeyPair,
  generateSymmetricKey
};
