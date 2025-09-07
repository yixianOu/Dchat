import React, { useState, useEffect } from 'react';
import ChatRoom from './components/ChatRoom';
import KeyManager from './components/KeyManager';
import { 
  setUserInfo, 
  getUser, 
  setKeyPair, 
  addFriendKey, 
  addGroupKey, 
  joinDirect, 
  joinGroup,
  onDecrypted,
  onError
} from './services/dchatAPI';
import { User, DecryptedMessage, ChatSession, Friend, Group } from './types';
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

  // 初始化用户信息和事件监听
  useEffect(() => {
    const initApp = async () => {
      try {
        // 获取当前用户信息
        const currentUser = await getUser();
        setUser(currentUser);
        setNickname(currentUser.nickname);

        // 设置解密消息监听
        await onDecrypted((msg: DecryptedMessage) => {
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

        // 设置错误监听
        await onError((error: string) => {
          console.error('Chat error:', error);
          alert(`聊天错误: ${error}`);
        });

      } catch (error) {
        console.error('初始化应用失败:', error);
      }
    };

    initApp();
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
    const uid = prompt('输入好友 ID:');
    const pubKey = prompt('输入好友公钥 (Base64):');
    if (uid && pubKey) {
      try {
        await addFriendKey(uid, pubKey);
        setFriends(prev => [...prev, { id: uid, nickname: uid, publicKey: pubKey }]);
        alert('好友添加成功');
      } catch (error) {
        console.error('添加好友失败:', error);
        alert('添加好友失败');
      }
    }
  };

  const handleJoinGroup = async () => {
    const gid = prompt('输入群组 ID:');
    const symKey = prompt('输入群组对称密钥 (Base64):');
    if (gid && symKey) {
      try {
        await addGroupKey(gid, symKey);
        await joinGroup(gid);
        setGroups(prev => [...prev, { id: gid, name: `群聊 ${gid}`, symmetricKey: symKey }]);
        
        const newSession: ChatSession = {
          id: gid,
          name: `群聊 ${gid}`,
          isGroup: true
        };
        setSessions(prev => [...prev, newSession]);
        setCurrentSession(newSession);
        alert('加入群组成功');
      } catch (error) {
        console.error('加入群组失败:', error);
        alert('加入群组失败');
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
        const newSession: ChatSession = {
          id: peerID, // 这里使用 peerID，实际的 CID 会在后端计算
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
    return messages.filter(msg => 
      msg.IsGroup ? msg.CID === sessionId : 
      (msg.CID === sessionId || msg.Sender === sessionId)
    );
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
        </div>
        
        <div className="chat-controls">
          <button onClick={handleAddFriend}>添加好友</button>
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
