import React, { useState } from 'react';
import { loadNSCKeys } from '../services/dchatAPI';

interface KeyManagerProps {
  onClose: () => void;
}

const KeyManager: React.FC<KeyManagerProps> = ({ onClose }) => {
  const [privateKey, setPrivateKey] = useState('');
  const [publicKey, setPublicKey] = useState('');
  const [showKeys, setShowKeys] = useState(false);

  const generateKeyPair = () => {
    // TODO: 需要集成真正的加密库（如 libsodium.js 或 tweetnacl-js）
    // 当前使用模拟实现，生产环境中应该使用真正的 X25519 密钥对
    console.warn('警告：当前使用模拟密钥生成，生产环境请使用真正的加密库');
    
    // 生成32字节的模拟密钥（实际应该使用 X25519）
    const generateRandomBytes = (length: number) => {
      const array = new Uint8Array(length);
      crypto.getRandomValues(array);
      return btoa(String.fromCharCode(...array));
    };
    
    const privKey = generateRandomBytes(32);
    const pubKey = generateRandomBytes(32);
    
    setPrivateKey(privKey);
    setPublicKey(pubKey);
    setShowKeys(true);
  };

  const saveKeyPair = async () => {
    if (!privateKey || !publicKey) {
      alert('请先生成密钥对');
      return;
    }
    
    try {
      // 使用NSC seed加载密钥（这里应该传入实际的NSC seed）
      await loadNSCKeys(privateKey);  // 假设privateKey是NSC seed
      alert('NSC密钥加载成功');
      onClose();
    } catch (error) {
      console.error('加载NSC密钥失败:', error);
      alert('加载NSC密钥失败');
    }
  };

  const importKeyPair = () => {
    const privKey = prompt('输入私钥 (Base64):');
    const pubKey = prompt('输入公钥 (Base64):');
    
    if (privKey && pubKey) {
      setPrivateKey(privKey);
      setPublicKey(pubKey);
      setShowKeys(true);
    }
  };

  return (
    <div className="key-manager-modal">
      <div className="modal-content">
        <h3>密钥管理</h3>
        
        <div className="key-actions">
          <button onClick={generateKeyPair} className="btn-primary">
            生成新密钥对
          </button>
          <button onClick={importKeyPair} className="btn-secondary">
            导入密钥对
          </button>
        </div>

        {showKeys && (
          <div className="key-display">
            <div className="key-item">
              <label>公钥 (可分享):</label>
              <textarea 
                value={publicKey}
                onChange={(e) => setPublicKey(e.target.value)}
                rows={3}
                readOnly
              />
              <button 
                onClick={() => navigator.clipboard.writeText(publicKey)}
                className="copy-btn"
              >
                复制
              </button>
            </div>
            
            <div className="key-item">
              <label>私钥 (请妥善保管):</label>
              <textarea 
                value={privateKey}
                onChange={(e) => setPrivateKey(e.target.value)}
                rows={3}
                className="private-key"
              />
              <button 
                onClick={() => navigator.clipboard.writeText(privateKey)}
                className="copy-btn"
              >
                复制
              </button>
            </div>
          </div>
        )}

        <div className="modal-actions">
          <button onClick={saveKeyPair} className="btn-primary">
            保存密钥对
          </button>
          <button onClick={onClose} className="btn-secondary">
            取消
          </button>
        </div>
      </div>
    </div>
  );
};

export default KeyManager;
