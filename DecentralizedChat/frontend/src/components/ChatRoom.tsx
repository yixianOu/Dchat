import React, { useState, useEffect, useRef } from 'react';
import { SendMessage, GetChatHistory } from '../services/mockWails';
import { EventsOn } from '../../wailsjs/runtime/runtime';
import { Message, User, ChatRoomProps, MessageEvent } from '../types';

const ChatRoom: React.FC<ChatRoomProps> = ({ roomName }) => {
  const [messages, setMessages] = useState<Message[]>([]);
  const [newMessage, setNewMessage] = useState<string>('');
  const [onlineUsers, setOnlineUsers] = useState<User[]>([]);
  const messagesContainerRef = useRef<HTMLDivElement>(null);

  const sendMessage = async (): Promise<void> => {
    if (!newMessage.trim()) return;
    
    try {
      // 调用Go后端方法
      await SendMessage(roomName, newMessage);
      setNewMessage('');
    } catch (error) {
      console.error('发送消息失败:', error);
    }
  };

  const loadMessages = async (): Promise<void> => {
    try {
      const history: Message[] = await GetChatHistory(roomName);
      setMessages(history || []);
      setTimeout(scrollToBottom, 0);
    } catch (error) {
      console.error('加载消息历史失败:', error);
    }
  };

  const scrollToBottom = (): void => {
    const container = messagesContainerRef.current;
    if (container) {
      container.scrollTop = container.scrollHeight;
    }
  };

  const formatTime = (timestamp: number): string => {
    return new Date(timestamp).toLocaleTimeString();
  };

  const handleKeyPress = (e: React.KeyboardEvent<HTMLInputElement>): void => {
    if (e.key === 'Enter') {
      sendMessage();
    }
  };

  useEffect(() => {
    loadMessages();
    
    // 订阅消息更新
    EventsOn('new-message', (msg: MessageEvent) => {
      if (msg.room_id === roomName) {
        setMessages((prev: Message[]) => [...prev, msg]);
        setTimeout(scrollToBottom, 0);
      }
    });

    // 订阅在线用户更新
    EventsOn('users-update', (users: User[]) => {
      setOnlineUsers(users);
    });

    // 清理订阅在组件卸载时处理
    return () => {
      // Wails EventsOn 不返回unsubscribe函数，由Wails自动处理
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
