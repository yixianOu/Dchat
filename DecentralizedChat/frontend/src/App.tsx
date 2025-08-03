import React, { useState, useEffect } from 'react';
import ChatRoom from './components/ChatRoom';
import { GetTailscaleStatus, GetConnectedRooms } from './services/mockWails';
import { TailscaleStatus } from './types';
import './App.css';

const App: React.FC = () => {
  const [currentRoom, setCurrentRoom] = useState<string>('general');
  const [rooms, setRooms] = useState<string[]>(['general']);
  const [networkStatus, setNetworkStatus] = useState<string>('connecting');
  const [tailscaleIP, setTailscaleIP] = useState<string>('');

  useEffect(() => {
    // 检查网络状态
    const checkNetworkStatus = async (): Promise<void> => {
      try {
        const status: TailscaleStatus = await GetTailscaleStatus();
        setNetworkStatus(status.connected ? 'connected' : 'disconnected');
        setTailscaleIP(status.ip);
      } catch (error) {
        console.error('获取网络状态失败:', error);
        setNetworkStatus('error');
      }
    };

    // 加载聊天室列表
    const loadRooms = async (): Promise<void> => {
      try {
        const connectedRooms: string[] = await GetConnectedRooms();
        setRooms(connectedRooms);
      } catch (error) {
        console.error('加载聊天室失败:', error);
      }
    };

    checkNetworkStatus();
    loadRooms();

    // 定期检查网络状态
    const interval = setInterval(checkNetworkStatus, 30000);
    return () => clearInterval(interval);
  }, []);

  const joinRoom = (roomName: string): void => {
    if (!rooms.includes(roomName)) {
      setRooms(prev => [...prev, roomName]);
    }
    setCurrentRoom(roomName);
  };

  const getStatusColor = (): string => {
    switch (networkStatus) {
      case 'connected': return '#4CAF50';
      case 'connecting': return '#FF9800';
      case 'disconnected': return '#F44336';
      default: return '#9E9E9E';
    }
  };

  return (
    <div className="app">
      {/* 侧边栏 */}
      <div className="sidebar">
        <div className="app-header">
          <h2>DChat</h2>
          <div className="network-status">
            <div 
              className="status-indicator"
              style={{ backgroundColor: getStatusColor() }}
            />
            <span className="status-text">
              {networkStatus === 'connected' ? `已连接 (${tailscaleIP})` : networkStatus}
            </span>
          </div>
        </div>
        
        <div className="rooms-list">
          <h4>聊天室</h4>
          {rooms.map(room => (
            <div 
              key={room}
              className={`room-item ${room === currentRoom ? 'active' : ''}`}
              onClick={() => setCurrentRoom(room)}
            >
              #{room}
            </div>
          ))}
          
          <button 
            className="join-room-btn"
            onClick={() => {
              const roomName = prompt('输入聊天室名称:');
              if (roomName) joinRoom(roomName);
            }}
          >
            + 加入聊天室
          </button>
        </div>
      </div>
      
      {/* 主聊天区域 */}
      <div className="main-content">
        <ChatRoom roomName={currentRoom} />
      </div>
    </div>
  );
};

export default App;
