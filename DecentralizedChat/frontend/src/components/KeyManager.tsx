import React, { useState, useEffect } from 'react';
import { getUserNSCPublicKey } from '../services/dchatAPI';

interface KeyManagerProps {
  onClose: () => void;
}

const KeyManager: React.FC<KeyManagerProps> = ({ onClose }) => {
  const [userPubKey, setUserPubKey] = useState('');

  // 组件加载时获取当前用户公钥
  useEffect(() => {
    const loadPubKey = async () => {
      try {
        const pubKey = await getUserNSCPublicKey();
        setUserPubKey(pubKey);
      } catch (err) {
        console.warn('当前未配置NSC密钥:', err);
      }
    };
    loadPubKey();
  }, []);

  return (
    <div className="key-manager-modal">
      <div className="modal-content">
        <h3>NSC 密钥管理</h3>

        {/* 显示当前用户公钥 */}
        <div className="key-item">
          <label>我的NSC公钥 (直接分享给好友添加，无需额外UserID):</label>
          <textarea
            value={userPubKey || '未配置NSC密钥'}
            rows={2}
            readOnly
            style={{ fontFamily: 'monospace', fontSize: '12px' }}
          />
          {userPubKey && (
            <button
              onClick={() => {
                navigator.clipboard.writeText(userPubKey);
                alert('公钥已复制到剪贴板');
              }}
              className="copy-btn"
              style={{ marginTop: '4px' }}
            >
              复制公钥
            </button>
          )}
        </div>


        <div className="modal-actions">
          <button onClick={onClose} className="btn-secondary">
            关闭
          </button>
        </div>
      </div>
    </div>
  );
};

export default KeyManager;
