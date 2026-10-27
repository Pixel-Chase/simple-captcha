import { useState } from 'react';
import type { FormEvent } from 'react';
import { ADMIN_ERROR_CODE, adminApi, getErrorMessage, isAdminApiError } from '../api';
import type { LoginResponse } from '../api';
import { CaptchaShowcase } from './CaptchaShowcase';

/** 与服务端一致：用户名 1-64 个字符；密码至少 10 个字符、不超过 72 字节（bcrypt 上限） */
const USERNAME_MAX = 64;
const PASSWORD_MIN = 10;
const PASSWORD_MAX_BYTES = 72;

const LEAD =
  '服务已启动，还没有管理员账号。填写启动日志里的初始化口令，设置管理员用户名与密码，之后接入应用、允许来源与风控策略都在控制台里配置。';

const STEPS = ['查看启动日志中的初始化口令', '设置管理员用户名与密码', '进入控制台创建接入应用'];

const byteLength = (value: string) => new TextEncoder().encode(value).length;

function validate(code: string, username: string, password: string, confirm: string) {
  if (!code.trim()) {
    return '请填写初始化口令';
  }
  if (!username) {
    return '请填写用户名';
  }
  if ([...username].length > USERNAME_MAX) {
    return `用户名不能超过 ${USERNAME_MAX} 个字符`;
  }
  if ([...password].length < PASSWORD_MIN) {
    return `密码至少需要 ${PASSWORD_MIN} 个字符`;
  }
  if (byteLength(password) > PASSWORD_MAX_BYTES) {
    return `密码不能超过 ${PASSWORD_MAX_BYTES} 字节`;
  }
  if (password !== confirm) {
    return '两次输入的密码不一致';
  }
  return null;
}

interface SetupPageProps {
  onSetup: (result: LoginResponse) => void;
  /** 服务端已完成初始化（例如另一个浏览器先完成了）：切回登录页 */
  onAlreadyInitialized: () => void;
}

/** 控制台首页（未初始化）：说明 + 初始化卡片 + 题型体验，版式与登录页一致 */
export function SetupPage({ onSetup, onAlreadyInitialized }: SetupPageProps) {
  return (
    <div className="home">
      <section className="home__hero">
        <div className="home__intro">
          <span className="home__badge">
            <SparkIcon />
            Simple Captcha · Setup
          </span>
          <h1>初始化管理后台</h1>
          <p className="home__lead">{LEAD}</p>
          <ol className="home__facts home__facts--steps">
            {STEPS.map((step) => (
              <li key={step} className="home__fact">
                {step}
              </li>
            ))}
          </ol>
        </div>

        <SetupForm onSetup={onSetup} onAlreadyInitialized={onAlreadyInitialized} />
      </section>

      <CaptchaShowcase />
    </div>
  );
}

function SetupForm({ onSetup, onAlreadyInitialized }: SetupPageProps) {
  const [code, setCode] = useState('');
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [confirm, setConfirm] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const handleSubmit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (busy) {
      return;
    }

    const name = username.trim();
    const reason = validate(code, name, password, confirm);
    if (reason) {
      setError(reason);
      return;
    }

    setBusy(true);
    setError(null);
    try {
      onSetup(await adminApi.setup({ setupCode: code.trim(), username: name, password }));
    } catch (err) {
      if (isAdminApiError(err) && err.code === ADMIN_ERROR_CODE.alreadyInitialized) {
        onAlreadyInitialized();
        return;
      }
      setError(getErrorMessage(err, '初始化失败'));
      setBusy(false);
    }
  };

  return (
    <form className="login-card" onSubmit={handleSubmit}>
      <div className="login-card__head">
        <span className="login-card__icon" aria-hidden="true">
          <KeyIcon />
        </span>
        <div>
          <h2>首次初始化</h2>
          <p className="login-card__desc">完成后直接进入控制台</p>
        </div>
      </div>

      <label className="field">
        <span className="field__label">初始化口令</span>
        <input
          className="field__input field__input--code"
          value={code}
          onChange={(e) => setCode(e.target.value.toUpperCase())}
          placeholder="XXXX-XXXX-XXXX"
          autoComplete="off"
          spellCheck={false}
          disabled={busy}
        />
        <span className="field__hint">
          见服务启动日志（例如 <code>docker logs</code>）；遗失时重启服务即可获得新口令
        </span>
      </label>

      <label className="field">
        <span className="field__label">管理员用户名</span>
        <input
          className="field__input"
          value={username}
          onChange={(e) => setUsername(e.target.value)}
          autoComplete="username"
          disabled={busy}
        />
      </label>

      <label className="field">
        <span className="field__label">密码</span>
        <input
          className="field__input"
          type="password"
          value={password}
          onChange={(e) => setPassword(e.target.value)}
          autoComplete="new-password"
          disabled={busy}
        />
        <span className="field__hint">至少 {PASSWORD_MIN} 个字符</span>
      </label>

      <label className="field">
        <span className="field__label">确认密码</span>
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

      <button type="submit" className="btn btn--block login-card__submit" disabled={busy}>
        {busy ? '初始化中…' : '完成初始化'}
      </button>
    </form>
  );
}

function SparkIcon() {
  return (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" aria-hidden="true">
      <path
        d="M12 3v4M12 17v4M3 12h4M17 12h4M5.6 5.6l2.8 2.8M15.6 15.6l2.8 2.8M5.6 18.4l2.8-2.8M15.6 8.4l2.8-2.8"
        strokeLinecap="round"
      />
    </svg>
  );
}

function KeyIcon() {
  return (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" aria-hidden="true">
      <circle cx="8" cy="15" r="4" />
      <path d="m10.8 12.2 8.2-8.2M16 7l2.5 2.5M14 9l2 2" strokeLinecap="round" />
    </svg>
  );
}
