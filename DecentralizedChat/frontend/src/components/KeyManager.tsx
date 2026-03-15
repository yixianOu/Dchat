import React, { useState, useEffect } from 'react';
import { getUserNSCPublicKey, getUser, setUserInfo } from '../services/dchatAPI';
import { User } from '../types';

interface KeyManagerProps {
  onClose: () => void;
}

const KeyManager: React.FC<KeyManagerProps> = ({ onClose }) => {
  const [userPubKey, setUserPubKey] = useState('');
  const [user, setUser] = useState<User>({ id: '', nickname: '' });
  const [nickname, setNickname] = useState('');
  const [isSaving, setIsSaving] = useState(false);

  // 组件加载时获取当前用户公钥和用户信息
  useEffect(() => {
    const loadData = async () => {
      try {
        const pubKey = await getUserNSCPublicKey();
        setUserPubKey(pubKey);

        const currentUser = await getUser();
        setUser(currentUser);
        setNickname(currentUser.nickname);
      } catch (err) {
        console.warn('加载用户信息失败:', err);
      }
    };
    loadData();
  }, []);

  const handleSaveNickname = async () => {
    if (!nickname.trim()) {
      alert('昵称不能为空');
      return;
    }

    setIsSaving(true);
    try {
      await setUserInfo(nickname);
      const updatedUser = await getUser();
      setUser(updatedUser);
      alert('昵称保存成功');
    } catch (error) {
      console.error('保存昵称失败:', error);
      alert('保存昵称失败');
    } finally {
      setIsSaving(false);
    }
  };

  return (
    <div className="key-manager-modal">
      <div className="modal-content">
        <h3>设置</h3>

        {/* 用户ID */}
        <div className="key-item">
          <label>你的用户ID (可复制):</label>
          <div style={{ display: 'flex', gap: '8px', marginTop: '4px' }}>
            <input
              value={user.id}
              readOnly
              style={{ flex: 1, fontFamily: 'monospace', fontSize: '12px' }}
            />
            <button
              onClick={async () => {
                await navigator.clipboard.writeText(user.id);
                alert('用户ID已复制到剪贴板');
              }}
              className="copy-btn"
              style={{ padding: '4px 8px' }}
            >
              复制
            </button>
          </div>
        </div>

        {/* 昵称配置 */}
        <div className="key-item">
          <label>昵称:</label>
          <input
            value={nickname}
            onChange={(e) => setNickname(e.target.value)}
            placeholder="输入昵称"
            style={{ width: '100%', marginTop: '4px' }}
          />
          <button
            onClick={handleSaveNickname}
            className="btn-primary"
            disabled={isSaving}
            style={{ marginTop: '8px' }}
          >
            {isSaving ? '保存中...' : '保存昵称'}
          </button>
        </div>

        <div style={{ margin: '20px 0', borderTop: '1px solid #eee', paddingTop: '20px' }}>
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
