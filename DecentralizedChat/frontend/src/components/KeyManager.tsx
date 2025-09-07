import React, { useState } from 'react';
import { setKeyPair } from '../services/dchatAPI';

interface KeyManagerProps {
  onClose: () => void;
}

const KeyManager: React.FC<KeyManagerProps> = ({ onClose }) => {
  const [privateKey, setPrivateKey] = useState('');
  const [publicKey, setPublicKey] = useState('');
  const [showKeys, setShowKeys] = useState(false);

  const generateKeyPair = () => {
    // 这里应该调用加密库生成密钥对
    // 暂时用随机字符串模拟
    const privKey = btoa(Math.random().toString(36).substring(2, 15) + Math.random().toString(36).substring(2, 15));
    const pubKey = btoa(Math.random().toString(36).substring(2, 15) + Math.random().toString(36).substring(2, 15));
    
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
      await setKeyPair(privateKey, publicKey);
      alert('密钥对设置成功');
      onClose();
    } catch (error) {
      console.error('设置密钥对失败:', error);
      alert('设置密钥对失败');
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
