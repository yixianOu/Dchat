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
import { User, DecryptedMessage, ChatSession, Group } from './types';
import './App.css';

// 统一转换存储消息格式，修复时间戳解析问题
const convertStorageMessages = (historyMessages: any[]): DecryptedMessage[] => {
  return historyMessages.map((msg: any) => {
    // 解析Go格式时间戳，兼容带m=后缀的格式
    let timestamp = msg.timestamp;
    if (typeof timestamp === 'string' && timestamp.includes(' m=')) {
      timestamp = timestamp.split(' m=')[0];
    }
    const date = new Date(timestamp);
    return {
      SenderNickname: msg.sender_nickname,
      CID: msg.conversation_id,
      Sender: msg.sender_nickname || msg.sender_id,
      Ts: isNaN(date.getTime()) ? new Date().toISOString() : date.toISOString(),
      Plain: msg.content,
      IsGroup: msg.is_group,
      Subject: ''
    };
  });
};

const App: React.FC = () => {
  const [currentSession, setCurrentSession] = useState<ChatSession | null>(null);
  const [sessions, setSessions] = useState<ChatSession[]>([]);
  const [user, setUser] = useState<User>({ id: '', nickname: '' });
  const [messages, setMessages] = useState<DecryptedMessage[]>([]);
  const [groups, setGroups] = useState<Group[]>([]);
  const [showSettings, setShowSettings] = useState(false);
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

        // ✅ 加载所有历史会话
        try {
          const convs = await getAllConversations();
          const loadedSessions = convs.map(conv => {
            // 解析时间戳，兼容异常格式
            let lastTime = Date.now();
            if (conv.last_message_at) {
              let ts = String(conv.last_message_at);
              if (ts.includes(' m=')) {
                ts = ts.split(' m=')[0];
              }
              const date = new Date(ts);
              if (!isNaN(date.getTime())) {
                lastTime = date.getTime();
              }
            }

            return {
              id: conv.id,
              name: conv.type === 'group' ? `群聊 ${conv.id.slice(0, 8)}` : `私聊 ${conv.id.slice(0, 8)}`,
              isGroup: conv.type === 'group',
              lastTime: lastTime
            };
          });
          // 按最后消息时间倒序排列
          loadedSessions.sort((a, b) => b.lastTime - a.lastTime);
          setSessions(loadedSessions);
          console.log(`加载了 ${loadedSessions.length} 个历史会话`);
        } catch (err) {
          console.warn('加载历史会话失败:', err);
        }
      } catch (error) {
        console.error('初始化应用失败:', error);
      }
    };

    initApp();
  }, []);

  // 消息监听单独处理，依赖currentSession确保能拿到最新值
  useEffect(() => {
    console.log('🔄 重新注册消息回调，当前会话:', currentSession?.id);

    // ⭐ 基于事件的消息监听：直接去重添加，保证实时性
    const unsubscribeMessages = onDecrypted((msg: DecryptedMessage) => {
      console.log('📩 收到新消息:', msg);
      console.log('📍 当前会话ID:', currentSession?.id, '消息会话ID:', msg.CID);

      // 1. 先修复时间格式
      // 优先使用消息携带的发送者昵称
      // @ts-ignore
      processedMsg.Sender = msg.RawWire?.Nickname || msg.Sender;
      const processedMsg = {...msg};
      // 解析时间戳，兼容各种格式
      try {
        let ts = msg.Ts;
        if (typeof ts === 'string' && ts.includes(' m=')) {
          ts = ts.split(' m=')[0];
        }
        const date = new Date(ts);
        if (isNaN(date.getTime())) {
          // 时间无效则用当前时间
          processedMsg.Ts = new Date().toISOString();
        }
      } catch (e) {
        processedMsg.Ts = new Date().toISOString();
      }

      // 2. 严格去重：会话ID+发送者+内容完全一致就认为是重复
      setMessages(prev => {
        const isDuplicate = prev.some(m =>
          m.CID === processedMsg.CID &&
          m.Sender === processedMsg.Sender &&
          m.Plain === processedMsg.Plain
        );

        if (isDuplicate) {
          console.log('ℹ️ 检测到重复消息，忽略');
          return prev;
        }
        return [...prev, processedMsg];
      });

      // 2. 更新会话列表
      const sessionId = msg.IsGroup ? msg.CID : msg.CID;
      setSessions(prev => {
        const existing = prev.find(s => s.id === sessionId);
        if (existing) {
          return prev.map(s =>
            s.id === sessionId
              ? { ...s, lastMessage: msg.Plain, lastTime: new Date().getTime() }
              : s
          );
        } else {
          return [...prev, {
            id: sessionId,
            name: msg.IsGroup ? `群聊 ${sessionId.slice(0, 8)}` : `私聊 ${sessionId.slice(0, 8)}`,
            isGroup: msg.IsGroup,
            lastMessage: processedMsg.Plain,
            lastTime: new Date().getTime()
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
  }, [currentSession]); // 加入currentSession依赖，确保能拿到最新的会话值


  // ✅ 切换会话时加载本地历史消息
  useEffect(() => {
    const loadSessionHistory = async () => {
      if (!currentSession) return;

      try {
        const historyMessages = await getMessages(currentSession.id, 50, null as any);
        const converted = convertStorageMessages(historyMessages);
        setMessages(converted);
      } catch (error) {
        console.error('加载历史消息失败:', error);
      }
    };

    loadSessionHistory();
  }, [currentSession?.id]);


  const handleAddFriend = async () => {
    const nscPubKey = prompt('输入好友的NSC公钥 (U开头的公开身份ID):');
    if (!nscPubKey) return;

    try {
      // 1. 添加好友NSC公钥，自动派生聊天公钥和好友ID
      const friendID = await addFriendNSCKey(nscPubKey);

      // 2. 自动加入私聊会话，不需要用户手动点击"开始私聊"
      const conversationID = await getConversationID(friendID);
      const newSession: ChatSession = {
        id: conversationID,
        name: `私聊 ${friendID.slice(0, 8)}`,
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
      alert('添加好友失败，请检查NSC公钥格式是否正确');
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
          </div>
          
        </div>
        
        <div className="chat-controls">
          <button onClick={handleAddFriend}>添加好友</button>
          <button onClick={handleCreateGroup}>创建群聊</button>
          <button onClick={handleJoinGroup}>加入群组</button>
          <button onClick={() => setShowSettings(true)}>设置</button>
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

      {/* 设置弹窗（原密钥管理，已整合用户ID、昵称、公钥管理） */}
      {showSettings && (
        <KeyManager onClose={() => setShowSettings(false)} />
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
