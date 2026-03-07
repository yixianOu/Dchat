import React, { useState, useEffect } from 'react';
import ChatRoom from './components/ChatRoom';
import KeyManager from './components/KeyManager';
import {
  setUserInfo,
  getUser,
  addFriendKey,
  addFriendNSCKey,
  addGroupKey,
  joinDirect,
  joinGroup,
  createGroup,
  onDecrypted,
  onError,
  getConversationID,  // ✅ 新增功能
  getNetworkStatus    // ✅ 新增功能
} from './services/dchatAPI';
import { User, DecryptedMessage, ChatSession, Friend, Group, NetworkStatus } from './types';
import './App.css';

const App: React.FC = () => {
  const [currentSession, setCurrentSession] = useState<ChatSession | null>(null);
  const [sessions, setSessions] = useState<ChatSession[]>([]);
  const [user, setUser] = useState<User>({ id: '', nickname: '' });
  const [messages, setMessages] = useState<DecryptedMessage[]>([]);
  const [friends, setFriends] = useState<Friend[]>([]);
  const [groups, setGroups] = useState<Group[]>([]);
  const [showSettings, setShowSettings] = useState(false);
  const [showKeyManager, setShowKeyManager] = useState(false);
  const [nickname, setNickname] = useState('');
  const [networkStatus, setNetworkStatus] = useState<NetworkStatus | null>(null); // ✅ 新增网络状态

  // 初始化用户信息和事件监听
  useEffect(() => {
    const initApp = async () => {
      try {
        // 获取当前用户信息
        const currentUser = await getUser();
        setUser(currentUser);
        setNickname(currentUser.nickname);

        // ⭐ 基于事件的消息监听
        const unsubscribeMessages = onDecrypted((msg: DecryptedMessage) => {
          setMessages(prev => [...prev, msg]);
          
          // 更新会话列表
          const sessionId = msg.IsGroup ? msg.CID : msg.CID;
          setSessions(prev => {
            const existing = prev.find(s => s.id === sessionId);
            if (existing) {
              return prev.map(s => 
                s.id === sessionId 
                  ? { ...s, lastMessage: msg.Plain, lastTime: new Date(msg.Ts).getTime() }
                  : s
              );
            } else {
              return [...prev, {
                id: sessionId,
                name: msg.IsGroup ? `群聊 ${sessionId.slice(0, 8)}` : `私聊 ${msg.Sender}`,
                isGroup: msg.IsGroup,
                lastMessage: msg.Plain,
                lastTime: new Date(msg.Ts).getTime()
              }];
            }
          });
        });

        // ⭐ 基于事件的错误监听
        const unsubscribeErrors = onError((errorData: { error: string; timestamp: string }) => {
          console.error('Chat error:', errorData.error);
          // 可以添加更好的错误处理，如错误状态管理
          alert(`聊天错误: ${errorData.error}`);
        });

        // 清理函数
        return () => {
          unsubscribeMessages();
          unsubscribeErrors();
        };

      } catch (error) {
        console.error('初始化应用失败:', error);
      }
    };

    const cleanup = initApp();
    return () => {
      cleanup?.then(fn => fn?.());
    };
  }, []);

  // ✅ 新增：定期检查网络状态
  useEffect(() => {
    const checkNetworkStatus = async () => {
      try {
        const status = await getNetworkStatus();
        setNetworkStatus(status as NetworkStatus);
      } catch (error) {
        console.error('获取网络状态失败:', error);
      }
    };

    // 立即检查一次
    checkNetworkStatus();
    
    // 每30秒检查一次网络状态
    const interval = setInterval(checkNetworkStatus, 30000);
    
    return () => clearInterval(interval);
  }, []);

  const handleSetNickname = async () => {
    try {
      await setUserInfo(nickname);
      const updatedUser = await getUser();
      setUser(updatedUser);
      setShowSettings(false);
    } catch (error) {
      console.error('设置昵称失败:', error);
      alert('设置昵称失败');
    }
  };

  const handleAddFriend = async () => {
    const uid = prompt('输入好友备注名:');
    const nscPubKey = prompt('输入好友的NSC公钥 (U开头的公开身份ID):');
    if (uid && nscPubKey) {
      try {
        // 新的方法：仅用NSC公钥添加好友，自动派生聊天公钥
        await addFriendNSCKey(uid, nscPubKey);
        setFriends(prev => [...prev, { id: uid, nickname: uid, publicKey: nscPubKey }]);
        alert('好友添加成功！无需交换密钥，可直接发送加密消息');
      } catch (error) {
        console.error('添加好友失败:', error);
        alert('添加好友失败，请检查NSC公钥格式是否正确');
      }
    }
  };

  const handleCreateGroup = async () => {
    try {
      const { gid, groupKey } = await createGroup();
      setGroups(prev => [...prev, { id: gid, name: `群聊 ${gid.slice(0, 8)}`, symmetricKey: groupKey }]);

      const newSession: ChatSession = {
        id: gid,
        name: `群聊 ${gid.slice(0, 8)}`,
        isGroup: true
      };
      setSessions(prev => [...prev, newSession]);
      setCurrentSession(newSession);

      // 展示群ID和密钥给用户
      alert(`群创建成功！\n群ID: ${gid}\n群密钥: ${groupKey}\n请将这些信息分享给要加入的好友。`);
    } catch (error) {
      console.error('创建群失败:', error);
      alert('创建群失败');
    }
  };

  const handleJoinGroup = async () => {
    const gid = prompt('输入群组 ID:');
    const symKey = prompt('输入群组对称密钥 (Base64):');
    if (gid && symKey) {
      try {
        // 新的joinGroup已经包含了addGroupKey逻辑，不需要单独调用
        await joinGroup(gid, symKey);
        setGroups(prev => [...prev, { id: gid, name: `群聊 ${gid.slice(0, 8)}`, symmetricKey: symKey }]);

        const newSession: ChatSession = {
          id: gid,
          name: `群聊 ${gid.slice(0, 8)}`,
          isGroup: true
        };
        setSessions(prev => [...prev, newSession]);
        setCurrentSession(newSession);
        alert('加入群组成功');
      } catch (error) {
        console.error('加入群组失败:', error);
        alert('加入群组失败，请检查群ID和密钥是否正确');
      }
    }
  };

  const handleStartDirectChat = async () => {
    const peerID = prompt('输入对方 ID:');
    if (peerID) {
      const friend = friends.find(f => f.id === peerID);
      if (!friend) {
        alert('请先添加该用户为好友');
        return;
      }
      
      try {
        await joinDirect(peerID);
        
        // ✅ 使用新功能：获取真实的会话ID
        const conversationID = await getConversationID(peerID);
        
        const newSession: ChatSession = {
          id: conversationID, // 使用后端计算的真实CID
          name: `私聊 ${friend.nickname}`,
          isGroup: false
        };
        setSessions(prev => [...prev, newSession]);
        setCurrentSession(newSession);
        alert('开始私聊成功');
      } catch (error) {
        console.error('开始私聊失败:', error);
        alert('开始私聊失败');
      }
    }
  };

  const getSessionMessages = (sessionId: string): DecryptedMessage[] => {
    // ✅ 简化：现在直接匹配CID，因为我们使用真实的会话ID
    return messages.filter(msg => msg.CID === sessionId);
  };

  return (
    <div className="app">
      {/* 侧边栏 */}
      <div className="sidebar">
        <div className="app-header">
          <h2>DChat</h2>
          <div className="user-info">
            <span>{user.nickname || '未设置昵称'}</span>
            <button onClick={() => setShowSettings(true)}>设置</button>
          </div>
          
          {/* ✅ 新增：网络状态显示 */}
          {networkStatus && (
            <div className="network-status">
              <div className={`status-indicator ${networkStatus.nats?.connected ? 'online' : 'offline'}`}>
                {networkStatus.nats?.connected ? '🟢 在线' : '🔴 离线'}
              </div>
              <div className="network-info">
                <small>
                  消息: {networkStatus.nats?.stats?.InMsgs || 0}↓ {networkStatus.nats?.stats?.OutMsgs || 0}↑
                </small>
              </div>
            </div>
          )}
        </div>
        
        <div className="chat-controls">
          <button onClick={handleAddFriend}>添加好友</button>
          <button onClick={handleCreateGroup}>创建群聊</button>
          <button onClick={handleJoinGroup}>加入群组</button>
          <button onClick={handleStartDirectChat}>开始私聊</button>
          <button onClick={() => setShowKeyManager(true)}>密钥管理</button>
        </div>
        
        <div className="sessions-list">
          <h4>聊天会话</h4>
          {sessions.map(session => (
            <div 
              key={session.id}
              className={`session-item ${session === currentSession ? 'active' : ''}`}
              onClick={() => setCurrentSession(session)}
            >
              <div className="session-name">
                {session.isGroup ? '🏷️' : '👤'} {session.name}
              </div>
              {session.lastMessage && (
                <div className="last-message">{session.lastMessage.slice(0, 30)}...</div>
              )}
            </div>
          ))}
        </div>
      </div>
      
      {/* 主聊天区域 */}
      <div className="main-content">
        {currentSession ? (
          <ChatRoom 
            roomName={currentSession.name}
            sessionId={currentSession.id}
            isGroup={currentSession.isGroup}
            messages={getSessionMessages(currentSession.id)}
          />
        ) : (
          <div className="no-session">
            <p>请选择一个聊天会话或开始新的对话</p>
          </div>
        )}
      </div>

      {/* 设置弹窗 */}
      {showSettings && (
        <div className="settings-modal">
          <div className="modal-content">
            <h3>用户设置</h3>
            <div className="form-group">
              <label>昵称:</label>
              <input 
                value={nickname}
                onChange={(e) => setNickname(e.target.value)}
                placeholder="输入昵称"
              />
            </div>
            <div className="modal-actions">
              <button onClick={handleSetNickname}>保存</button>
              <button onClick={() => setShowSettings(false)}>取消</button>
            </div>
          </div>
        </div>
      )}

      {/* 密钥管理弹窗 */}
      {showKeyManager && (
        <KeyManager onClose={() => setShowKeyManager(false)} />
      )}
    </div>
  );
};

export default App;
