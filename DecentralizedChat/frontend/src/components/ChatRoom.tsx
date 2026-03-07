import React, { useState, useEffect, useRef } from 'react';
import { sendDirect, sendGroup, getMessages } from '../services/dchatAPI';
import { DecryptedMessage, ChatRoomProps } from '../types';

const ChatRoom: React.FC<ChatRoomProps> = ({ roomName, sessionId, isGroup = false, messages, onMessagesUpdate }) => {
  const [newMessage, setNewMessage] = useState<string>('');
  const messagesContainerRef = useRef<HTMLDivElement>(null);

  const sendMessage = async (): Promise<void> => {
    if (!newMessage.trim()) return;
    
    try {
      if (isGroup) {
        await sendGroup(sessionId, newMessage);
      } else {
        await sendDirect(sessionId, newMessage);
      }
      setNewMessage('');

      // Send successful, fetch latest message to update display
      try {
        const latest = await getMessages(sessionId, 1, null as any);
        if (latest && latest.length > 0 && onMessagesUpdate) {
          const converted = {
            CID: latest[0].conversation_id,
            Sender: latest[0].sender_nickname || latest[0].sender_id,
            Ts: String(latest[0].timestamp),
            Plain: latest[0].content,
            IsGroup: latest[0].is_group,
            Subject: ''
          };
          onMessagesUpdate(converted);
        }
      } catch (fetchError) {
        console.warn('Failed to fetch latest message:', fetchError);
      }
    } catch (error) {
      console.error('发送消息失败:', error);
      alert('发送消息失败');
    }
  };

  const scrollToBottom = (): void => {
    const container = messagesContainerRef.current;
    if (container) {
      container.scrollTop = container.scrollHeight;
    }
  };

  const formatTime = (timeStr: string): string => {
    return new Date(timeStr).toLocaleTimeString();
  };

  const handleKeyPress = (e: React.KeyboardEvent<HTMLInputElement>): void => {
    if (e.key === 'Enter') {
      sendMessage();
    }
  };

  useEffect(() => {
    scrollToBottom();
  }, [messages]);

  return (
    <div className="chat-room">
      {/* 聊天室头部 */}
      <div className="room-header">
        <h3>{roomName}</h3>
        <div className="room-info">
          <span className="room-type">
            {isGroup ? '群聊' : '私聊'} | ID: {sessionId.slice(0, 8)}...
          </span>
        </div>
      </div>
      
      {/* 消息列表 */}
      <div className="messages" ref={messagesContainerRef}>
        {messages.map((msg, index) => (
          <div key={`${msg.CID}-${msg.Ts}-${index}`} className="message">
            <div className="message-header">
              <span className="username">{msg.Sender}</span>
              <span className="timestamp">{formatTime(msg.Ts)}</span>
            </div>
            <div className="message-content">{msg.Plain}</div>
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
