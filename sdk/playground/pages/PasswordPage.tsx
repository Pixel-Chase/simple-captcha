import { useState } from 'react';
import type { FormEvent } from 'react';
import { adminApi, getErrorMessage, isAdminApiError } from '../api';
import type { LoginResponse } from '../api';

/** 与服务端一致：至少 10 个字符，不超过 72 字节（bcrypt 上限） */
const MIN_LENGTH = 10;
const MAX_BYTES = 72;

const byteLength = (value: string) => new TextEncoder().encode(value).length;

function validate(current: string, next: string, confirm: string): string | null {
  if (!current || !next) {
    return '请填写当前密码和新密码';
  }
  if ([...next].length < MIN_LENGTH) {
    return `新密码至少需要 ${MIN_LENGTH} 个字符`;
  }
  if (byteLength(next) > MAX_BYTES) {
    return `新密码不能超过 ${MAX_BYTES} 字节`;
  }
  if (next !== confirm) {
    return '两次输入的新密码不一致';
  }
  if (next === current) {
    return '新密码不能与当前密码相同';
  }
  return null;
}

interface PasswordPageProps {
  token: string;
  username: string;
  onChanged: (result: LoginResponse) => void;
  onSessionExpired: () => void;
}

export function PasswordPage({ token, username, onChanged, onSessionExpired }: PasswordPageProps) {
  const [current, setCurrent] = useState('');
  const [next, setNext] = useState('');
  const [confirm, setConfirm] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [done, setDone] = useState(false);

  const handleSubmit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (busy) {
      return;
    }

    const reason = validate(current, next, confirm);
    if (reason) {
      setError(reason);
      return;
    }

    setBusy(true);
    setError(null);
    setDone(false);
    try {
      const result = await adminApi.changePassword(token, {
        currentPassword: current,
        newPassword: next,
      });
      setCurrent('');
      setNext('');
      setConfirm('');
      setDone(true);
      onChanged(result);
    } catch (err) {
      if (isAdminApiError(err) && err.status === 401) {
        onSessionExpired();
        return;
      }
      setError(getErrorMessage(err, '修改密码失败'));
    } finally {
      setBusy(false);
    }
  };

  return (
    <section className="password">
      <div className="password__intro">
        <p className="eyebrow">账号安全</p>
        <h1>修改管理员密码</h1>
        <p className="password__lead">
          修改后，其他浏览器或设备上的登录会全部退出，当前页面自动换用新会话。
        </p>
        <p className="password__note">
          忘记密码时，在后端执行 <code>./simple-captcha reset-admin</code>
          ：管理员账号被删除，用启动日志中的新口令在初始化页面重新设置用户名与密码，接入应用不受影响。
        </p>
      </div>

      <form className="card password__form" onSubmit={handleSubmit}>
        <h2>{username}</h2>

        {/* 供密码管理器识别账号 */}
        <input type="hidden" autoComplete="username" value={username} readOnly />

        <label className="field">
          <span className="field__label">当前密码</span>
          <input
            className="field__input"
            type="password"
            value={current}
            onChange={(e) => setCurrent(e.target.value)}
            autoComplete="current-password"
            disabled={busy}
          />
        </label>

        <label className="field">
          <span className="field__label">新密码</span>
          <input
            className="field__input"
            type="password"
            value={next}
            onChange={(e) => setNext(e.target.value)}
            autoComplete="new-password"
            disabled={busy}
          />
          <span className="field__hint">至少 {MIN_LENGTH} 个字符</span>
        </label>

        <label className="field">
          <span className="field__label">确认新密码</span>
          <input
            className="field__input"
            type="password"
            value={confirm}
            onChange={(e) => setConfirm(e.target.value)}
            autoComplete="new-password"
            disabled={busy}
          />
        </label>

        {error && (
          <div className="alert alert--error" role="alert">
            {error}
          </div>
        )}
        {done && (
          <div className="alert alert--success" role="status">
            密码已修改，其他会话已全部退出。
          </div>
        )}

        <button type="submit" className="btn btn--primary btn--block" disabled={busy}>
          {busy ? '保存中…' : '保存新密码'}
        </button>
      </form>
    </section>
  );
}
