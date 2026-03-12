import React, { useState, useEffect } from 'react';
import { loadNSCKeys, getUserNSCPublicKey } from '../services/dchatAPI';

interface KeyManagerProps {
  onClose: () => void;
}

const KeyManager: React.FC<KeyManagerProps> = ({ onClose }) => {
  const [seed, setSeed] = useState('');
  const [userPubKey, setUserPubKey] = useState('');
  const [isLoading, setIsLoading] = useState(false);

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

  const importNSCSeed = async () => {
    if (!seed.trim()) {
      alert('请输入NSC Seed');
      return;
    }

    // 简单格式校验
    if (!seed.trim().startsWith('SU')) {
      alert('NSC Seed格式错误，必须以"SU"开头');
      return;
    }

    setIsLoading(true);
    try {
      await loadNSCKeys(seed.trim());
      // 导入成功后重新获取公钥
      const pubKey = await getUserNSCPublicKey();
      setUserPubKey(pubKey);
      alert('NSC密钥导入成功！');
      setSeed('');
    } catch (error) {
      console.error('导入NSC密钥失败:', error);
      alert('导入失败，请检查Seed格式是否正确');
    } finally {
      setIsLoading(false);
    }
  };

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

        <div style={{ margin: '20px 0', borderTop: '1px solid #eee', paddingTop: '20px' }}>
          <h4>导入NSC Seed</h4>
          <div className="form-group">
            <label>NSC Seed (SU开头的私钥):</label>
            <textarea
              value={seed}
              onChange={(e) => setSeed(e.target.value)}
              rows={3}
              placeholder="请输入以SU开头的NSC Seed"
              style={{ width: '100%', fontFamily: 'monospace', fontSize: '12px' }}
            />
          </div>
          <button
            onClick={importNSCSeed}
            className="btn-primary"
            disabled={isLoading}
            style={{ marginTop: '10px' }}
          >
            {isLoading ? '导入中...' : '导入NSC Seed'}
          </button>
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
