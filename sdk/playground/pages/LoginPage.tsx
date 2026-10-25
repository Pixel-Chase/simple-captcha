import { useState } from 'react';
import type { FormEvent } from 'react';
import { CaptchaError, useCaptcha } from '@aetherlib/g-captcha';
import { adminApi, getErrorMessage } from '../api';
import type { LoginResponse } from '../api';
import { CaptchaShowcase } from './CaptchaShowcase';

type Stage = 'idle' | 'verifying' | 'submitting';

const STAGE_TEXT: Record<Stage, string> = {
  idle: '验证并登录',
  verifying: '请完成行为验证…',
  submitting: '登录中…',
};

const FACTS = ['一次性 second_key', '工作量证明 PoW', '行为轨迹风控', 'bcrypt 密码校验'];

interface LoginPageProps {
  onLogin: (result: LoginResponse) => void;
}

/** 控制台首页（未登录）：介绍 + 登录卡片 + 题型体验 */
export function LoginPage({ onLogin }: LoginPageProps) {
  return (
    <div className="home">
      <section className="home__hero">
        <div className="home__intro">
          <span className="home__badge">
            <ShieldIcon />
            Simple Captcha · Console
          </span>
          <h1>行为验证码管理后台</h1>
          <p className="home__lead">
            管理验证码背景图，为业务后端签发 S2S
            应用密钥。登录同样受行为验证码保护：每次尝试都要重新完成一次验证。
          </p>
          <ul className="home__facts">
            {FACTS.map((fact) => (
              <li key={fact} className="home__fact">
                {fact}
              </li>
            ))}
          </ul>
        </div>

        <LoginForm onLogin={onLogin} />
      </section>

      <CaptchaShowcase />
    </div>
  );
}

function LoginForm({ onLogin }: LoginPageProps) {
  const { verify, holder } = useCaptcha();
  const [username, setUsername] = useState('admin');
  const [password, setPassword] = useState('');
  const [stage, setStage] = useState<Stage>('idle');
  const [error, setError] = useState<string | null>(null);

  const busy = stage !== 'idle';

  const handleSubmit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (busy) {
      return;
    }

    const name = username.trim();
    if (!name || !password) {
      setError('请输入用户名和密码');
      return;
    }

    setError(null);
    setStage('verifying');

    // 行为验证成功后拿到一次性的 second_key，随登录表单一起提交，由服务端消费
    let secondKey: string;
    try {
      const result = await verify({ type: 'auto' });
      secondKey = result.second_key;
    } catch (err) {
      const cancelled = err instanceof CaptchaError && err.code === 'CANCELLED';
      setError(cancelled ? '你已取消本次验证' : getErrorMessage(err, '验证未完成'));
      setStage('idle');
      return;
    }

    setStage('submitting');
    try {
      onLogin(await adminApi.login({ username: name, password, captchaKey: secondKey }));
    } catch (err) {
      setError(getErrorMessage(err));
      setStage('idle');
    }
  };

  return (
    <>
      {/* 弹窗里的按钮不能落在 form 内，否则会触发提交 */}
      <div className="captcha-holder">{holder}</div>

      <form className="login-card" onSubmit={handleSubmit}>
        <div className="login-card__head">
          <span className="login-card__icon" aria-hidden="true">
            <LockIcon />
          </span>
          <div>
            <h2>登录控制台</h2>
            <p className="login-card__desc">提交前会先弹出一次行为验证</p>
          </div>
        </div>

        <label className="field">
          <span className="field__label">用户名</span>
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
            autoComplete="current-password"
            disabled={busy}
          />
        </label>

        {error && (
          <div className="alert alert--error" role="alert">
            {error}
          </div>
        )}

        <button type="submit" className="btn btn--block login-card__submit" disabled={busy}>
          {STAGE_TEXT[stage]}
        </button>

        <p className="login-card__hint">
          初始密码在服务首次启动时打印于日志，首次登录后需修改；遗失可执行{' '}
          <code>reset-admin-password</code>。
        </p>
      </form>
    </>
  );
}

function ShieldIcon() {
  return (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" aria-hidden="true">
      <path d="M12 3 4.5 6v5.5c0 4.6 3.2 8.4 7.5 9.5 4.3-1.1 7.5-4.9 7.5-9.5V6L12 3Z" />
      <path d="m8.8 12 2.2 2.2 4.3-4.4" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  );
}

function LockIcon() {
  return (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" aria-hidden="true">
      <rect x="5" y="10.5" width="14" height="10" rx="2.5" />
      <path d="M8.5 10.5V8a3.5 3.5 0 0 1 7 0v2.5" strokeLinecap="round" />
      <path d="M12 14.5v2" strokeLinecap="round" />
    </svg>
  );
}
