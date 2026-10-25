import { useState } from 'react';
import type { FormEvent } from 'react';
import { useCaptcha } from '@aetherlib/g-captcha';
import type { CaptchaType } from '@aetherlib/g-captcha';
import { API_PREFIX, adminApi, getErrorMessage } from '../api';
import type { LoginResponse } from '../api';
import { CAPTCHA_TYPE_OPTIONS } from '../captcha-options';

type Stage = 'idle' | 'verifying' | 'submitting';

const STAGE_TEXT: Record<Stage, string> = {
  idle: '登录',
  verifying: '请完成行为验证…',
  submitting: '登录中…',
};

interface LoginPageProps {
  onLogin: (result: LoginResponse) => void;
}

export function LoginPage({ onLogin }: LoginPageProps) {
  const { verify, holder } = useCaptcha();
  const [username, setUsername] = useState('admin');
  const [password, setPassword] = useState('');
  const [captchaType, setCaptchaType] = useState<CaptchaType>('auto');
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
      const result = await verify({ type: captchaType });
      secondKey = result.second_key;
    } catch (err) {
      setError(getErrorMessage(err, '验证未完成'));
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
    <section className="login">
      <div className="captcha-holder">{holder}</div>

      <div className="login__intro">
        <p className="eyebrow">演示</p>
        <h1>用行为验证码保护登录</h1>
        <p>
          点击"登录"会先弹出行为验证。验证通过后 SDK 返回一次性的 <code>second_key</code>
          ，服务端在比对账号密码之前先消费它：每一次登录尝试都必须重新完成一次验证，
          密码暴力猜解因此变得昂贵。
        </p>
        <ol className="steps">
          <li>
            <span className="steps__index">1</span>
            <span>
              前端调用 <code>useCaptcha().verify()</code>，用户完成点选 / 滑块 / 旋转
            </span>
          </li>
          <li>
            <span className="steps__index">2</span>
            <span>
              携 <code>second_key</code> 提交 <code>POST {API_PREFIX}/admin/login</code>
            </span>
          </li>
          <li>
            <span className="steps__index">3</span>
            <span>服务端消费 second_key → 校验 bcrypt 密码 → 签发 Bearer 会话令牌</span>
          </li>
        </ol>
      </div>

      <form className="card login__form" onSubmit={handleSubmit}>
        <h2>登录控制台</h2>

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

        <label className="field">
          <span className="field__label">验证方式</span>
          <select
            className="field__input"
            value={captchaType}
            onChange={(e) => setCaptchaType(e.target.value as CaptchaType)}
            disabled={busy}
          >
            {CAPTCHA_TYPE_OPTIONS.map((item) => (
              <option key={item.value} value={item.value}>
                {item.label}
              </option>
            ))}
          </select>
        </label>

        {error && (
          <div className="alert alert--error" role="alert">
            {error}
          </div>
        )}

        <button type="submit" className="btn btn--primary btn--block" disabled={busy}>
          {STAGE_TEXT[stage]}
        </button>

        <p className="login__hint">
          用户名默认 <code>admin</code>（<code>ADMIN_USERNAME</code>
          ）。初始密码在后端首次启动时随机生成并打印在服务控制台，首次登录后需要修改；遗失可执行{' '}
          <code>reset-admin-password</code>。
        </p>
      </form>
    </section>
  );
}
