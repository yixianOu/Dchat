// 临时模拟函数，等待Wails重新生成
import { TailscaleStatus, Message } from '../types';

export const GetTailscaleStatus = async (): Promise<TailscaleStatus> => {
  // 模拟函数，需要在后端实现后重新生成
  return { connected: false, ip: '' };
};

export const GetConnectedRooms = async (): Promise<string[]> => {
  // 模拟函数，需要在后端实现后重新生成
  return ['general'];
};

export const SendMessage = async (roomName: string, message: string): Promise<void> => {
  // 模拟函数，需要在后端实现后重新生成
  console.log(`Sending message to ${roomName}: ${message}`);
};

export const GetChatHistory = async (roomName: string): Promise<Message[]> => {
  // 模拟函数，需要在后端实现后重新生成
  return [];
};
