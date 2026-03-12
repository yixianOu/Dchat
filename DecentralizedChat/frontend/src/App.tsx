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
  getNetworkStatus,    // ✅ 新增功能
  getMessages,         // ✅ 新增：获取历史消息
  getAllConversations // ✅ 新增：获取所有会话列表
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
  // 可复制信息弹窗状态
  const [showCopyModal, setShowCopyModal] = useState(false);
  const [copyModalTitle, setCopyModalTitle] = useState('');
  const [copyModalItems, setCopyModalItems] = useState<Array<{label: string, value: string}>>([]);

  // 初始化用户信息和事件监听
  useEffect(() => {
    const initApp = async () => {
      try {
        // 获取当前用户信息
        const currentUser = await getUser();
        setUser(currentUser);
        setNickname(currentUser.nickname);

        // ✅ 加载所有历史会话
        try {
          const convs = await getAllConversations();
          const loadedSessions = convs.map(conv => ({
            id: conv.id,
            name: conv.type === 'group' ? `群聊 ${conv.id.slice(0, 8)}` : `私聊 ${conv.id.slice(0, 8)}`,
            isGroup: conv.type === 'group',
            lastTime: new Date(conv.last_message_at as unknown as string).getTime()
          }));
          // 按最后消息时间倒序排列
          loadedSessions.sort((a, b) => b.lastTime - a.lastTime);
          setSessions(loadedSessions);
          console.log(`加载了 ${loadedSessions.length} 个历史会话`);
        } catch (err) {
          console.warn('加载历史会话失败:', err);
        }

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

  // ✅ 切换会话时加载本地历史消息
  useEffect(() => {
    const loadSessionHistory = async () => {
      if (!currentSession) return;

      try {
        const historyMessages = await getMessages(currentSession.id, 50, null as any);
        const converted = historyMessages.map((msg: any) => ({
          CID: msg.conversation_id,
          Sender: msg.sender_nickname || msg.sender_id,
          Ts: String(msg.timestamp),
          Plain: msg.content,
          IsGroup: msg.is_group,
          Subject: ''
        }));
        setMessages(converted);
      } catch (error) {
        console.error('加载历史消息失败:', error);
      }
    };

    loadSessionHistory();
  }, [currentSession?.id]);

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
    const userID = prompt('输入好友UserID:');
    if (!userID) return;

    const nscPubKey = prompt('输入好友的NSC公钥 (U开头的公开身份ID):');
    if (!nscPubKey) return;

    const remark = prompt('输入好友备注名(可选):', userID);

    try {
      // 1. 添加好友NSC公钥，自动派生聊天公钥
      await addFriendNSCKey(userID, nscPubKey);
      setFriends(prev => [...prev, { id: userID, nickname: remark || userID, publicKey: nscPubKey }]);

      // 2. 自动加入私聊会话，不需要用户手动点击"开始私聊"
      const conversationID = await getConversationID(userID);
      const newSession: ChatSession = {
        id: conversationID,
        name: `私聊 ${remark || userID}`,
        isGroup: false
      };

      // 3. 添加到会话列表并自动切换到该会话
      setSessions(prev => {
        if (!prev.find(s => s.id === conversationID)) {
          return [...prev, newSession];
        }
        return prev;
      });
      setCurrentSession(newSession);

      alert('好友添加成功！已自动打开聊天会话');
    } catch (error) {
      console.error('添加好友失败:', error);
      alert('添加好友失败，请检查UserID和NSC公钥格式是否正确');
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

      // 展示群ID和密钥给用户，支持复制
      setCopyModalTitle('群创建成功');
      setCopyModalItems([
        { label: '群ID', value: gid },
        { label: '群密钥', value: groupKey },
      ]);
      setShowCopyModal(true);
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
    // 如果已经有好友列表，让用户选择，否则手动输入
    if (friends.length === 0) {
      alert('您还没有添加任何好友，请先添加好友');
      return;
    }

    // 显示好友列表让用户选择
    const friendOptions = friends.map((f, i) => `${i+1}. ${f.nickname} (${f.id})`).join('\n');
    const selection = prompt(`选择要聊天的好友:\n${friendOptions}\n输入序号:`);
    if (!selection) return;

    const index = parseInt(selection) - 1;
    if (index < 0 || index >= friends.length) {
      alert('无效的序号');
      return;
    }

    const friend = friends[index];
    try {
      await joinDirect(friend.id);

      // 使用后端计算的真实会话ID
      const conversationID = await getConversationID(friend.id);

      const newSession: ChatSession = {
        id: conversationID,
        name: `私聊 ${friend.nickname}`,
        isGroup: false
      };

      // 避免重复添加会话
      setSessions(prev => {
        if (!prev.find(s => s.id === conversationID)) {
          return [...prev, newSession];
        }
        return prev;
      });
      setCurrentSession(newSession);
    } catch (error) {
      console.error('开始私聊失败:', error);
      alert('开始私聊失败');
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
            onMessagesUpdate={(newMsg) => setMessages(prev => [...prev, newMsg])}
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
              <label>你的用户ID (可复制):</label>
              <div style={{ display: 'flex', gap: '8px', marginTop: '4px' }}>
                <input
                  value={user.id}
                  readOnly
                  style={{ flex: 1, fontFamily: 'monospace', fontSize: '12px' }}
                />
                <button
                  onClick={async () => {
                    await navigator.clipboard.writeText(user.id);
                    const btn = event?.target as HTMLButtonElement;
                    const originalText = btn.textContent;
                    btn.textContent = '已复制!';
                    setTimeout(() => {
                      btn.textContent = originalText;
                    }, 2000);
                  }}
                  className="copy-btn"
                  style={{ padding: '4px 8px' }}
                >
                  复制
                </button>
              </div>
            </div>
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

      {/* 可复制信息弹窗 */}
      {showCopyModal && (
        <div className="key-manager-modal">
          <div className="modal-content">
            <h3>{copyModalTitle}</h3>
            <div className="key-display">
              {copyModalItems.map((item, index) => (
                <div key={index} className="key-item">
                  <label>{item.label}:</label>
                  <textarea
                    value={item.value}
                    rows={2}
                    readOnly
                    style={{ width: '100%', fontFamily: 'monospace', fontSize: '12px' }}
                  />
                  <button
                    onClick={async () => {
                      await navigator.clipboard.writeText(item.value);
                      const btn = event?.target as HTMLButtonElement;
                      const originalText = btn.textContent;
                      btn.textContent = '已复制!';
                      setTimeout(() => {
                        btn.textContent = originalText;
                      }, 2000);
                    }}
                    className="copy-btn"
                    style={{ marginTop: '4px' }}
                  >
                    复制
                  </button>
                </div>
              ))}
            </div>
            <p style={{ fontSize: '12px', color: '#666', margin: '10px 0' }}>
              请妥善保管以上信息，分享给需要加入的用户。
            </p>
            <div className="modal-actions">
              <button
                onClick={() => setShowCopyModal(false)}
                className="btn-primary"
              >
                我知道了
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
};

export default App;
