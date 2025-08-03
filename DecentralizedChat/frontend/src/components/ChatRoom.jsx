import React, { useState, useEffect, useRef } from 'react';
import { SendMessage, GetChatHistory } from '../../wailsjs/go/main/App';
import { EventsOn } from '../../wailsjs/runtime/runtime';

const ChatRoom = ({ roomName }) => {
  const [messages, setMessages] = useState([]);
  const [newMessage, setNewMessage] = useState('');
  const [onlineUsers, setOnlineUsers] = useState([]);
  const messagesContainerRef = useRef(null);

  const sendMessage = async () => {
    if (!newMessage.trim()) return;
    
    try {
      // 调用Go后端方法
      await SendMessage(roomName, newMessage);
      setNewMessage('');
    } catch (error) {
      console.error('发送消息失败:', error);
    }
  };

  const loadMessages = async () => {
    try {
      const history = await GetChatHistory(roomName);
      setMessages(history || []);
      setTimeout(scrollToBottom, 0);
    } catch (error) {
      console.error('加载消息历史失败:', error);
    }
  };

  const scrollToBottom = () => {
    const container = messagesContainerRef.current;
    if (container) {
      container.scrollTop = container.scrollHeight;
    }
  };

  const formatTime = (timestamp) => {
    return new Date(timestamp).toLocaleTimeString();
  };

  const handleKeyPress = (e) => {
    if (e.key === 'Enter') {
      sendMessage();
    }
  };

  useEffect(() => {
    loadMessages();
    
    // 订阅消息更新
    const unsubscribe = EventsOn('new-message', (msg) => {
      if (msg.room_id === roomName) {
        setMessages(prev => [...prev, msg]);
        setTimeout(scrollToBottom, 0);
      }
    });

    // 订阅在线用户更新
    const unsubscribeUsers = EventsOn('users-update', (users) => {
      setOnlineUsers(users);
    });

    // 清理订阅
    return () => {
      if (unsubscribe) unsubscribe();
      if (unsubscribeUsers) unsubscribeUsers();
    };
  }, [roomName]);

  useEffect(() => {
    scrollToBottom();
  }, [messages]);

  return (
    <div className="chat-room">
      {/* 聊天室头部 */}
      <div className="room-header">
        <h3>#{roomName}</h3>
        <div className="online-users">
          {onlineUsers.map(user => (
            <span key={user.id} className="user-badge">
              {user.nickname}
            </span>
          ))}
        </div>
      </div>
      
      {/* 消息列表 */}
      <div className="messages" ref={messagesContainerRef}>
        {messages.map(msg => (
          <div key={msg.id} className="message">
            <div className="message-header">
              <span className="username">{msg.username}</span>
              <span className="timestamp">{formatTime(msg.timestamp)}</span>
            </div>
            <div className="message-content">{msg.content}</div>
          </div>
        ))}
      </div>
      
      {/* 输入框 */}
      <div className="input-area">
        <input 
          value={newMessage}
          onChange={(e) => setNewMessage(e.target.value)}
          onKeyPress={handleKeyPress}
          placeholder="输入消息..."
          className="message-input"
        />
        <button onClick={sendMessage} className="send-button">
          发送
        </button>
      </div>
    </div>
  );
};

export default ChatRoom;
