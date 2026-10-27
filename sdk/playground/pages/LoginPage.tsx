import { useId, useState } from 'react';
import type { FormEvent } from 'react';
import { CaptchaError, useCaptcha } from '@aetherlib/g-captcha';
import { ADMIN_ERROR_CODE, adminApi, getErrorMessage, isAdminApiError } from '../api';
import type { LoginResponse } from '../api';
import { Icon } from '../components/Icon';
import type { IconName } from '../components/Icon';
import { useCopy } from '../components/Toast';
import { CaptchaShowcase } from './CaptchaShowcase';
import { GuestBackdrop, TrailMeter } from './GuestBackdrop';
import './GuestPages.css';

type Stage = 'idle' | 'verifying' | 'submitting';

const STAGE_VIEW: Record<Stage, { text: string; icon: IconName }> = {
  idle: { text: '验证并登录', icon: 'lock-open' },
  verifying: { text: '请完成行为验证…', icon: 'shield' },
  submitting: { text: '登录中…', icon: 'spinner' },
};

/** 提交按钮图标的动效：验证中盾牌脉冲，登录中转圈 */
const STAGE_ICON_CLASS: Record<Stage, string | undefined> = {
  idle: undefined,
  verifying: 'login-form__pulse',
  submitting: 'spin',
};

/** 遗失密码时在服务器执行的重置命令（镜像内工作目录为 /app，本地开发用 ./run-dev.sh reset-admin） */
const RESET_COMMAND = './simple-captcha reset-admin';

/** 登录链路涉及的防护机制，说明以 backend 的实际行为为准 */
const FACTS: ReadonlyArray<{ icon: IconName; name: string; desc: string }> = [
  {
    icon: 'key',
    name: '一次性 second_key',
    desc: '验证通过后签发，默认 2 分钟内有效，服务端消费一次即作废；登录只接受管理后台自身签发的 key。',
  },
  {
    icon: 'cpu',
    name: '工作量证明 PoW',
    desc: '题目附带 SHA-256 难题，SDK 在后台求解并随答案提交；真人无感，批量脚本的算力成本随题量线性上升。',
  },
  {
    icon: 'activity',
    name: '行为轨迹风控',
    desc: 'SDK 采集指针轨迹（速度、直线度、停顿等）与环境信号，服务端规则评分：风险偏高时加难，达到拦截阈值时拒绝本次校验。',
  },
  {
    icon: 'lock',
    name: 'bcrypt 密码校验',
    desc: '密码只以 bcrypt 哈希保存；无论用户名对错都会完整比对一次，响应时间不会泄露账号是否存在。',
  },
];

interface LoginPageProps {
  onLogin: (result: LoginResponse) => void;
  /** 服务端尚未初始化（例如执行了 reset-admin）：切到初始化页面 */
  onNotInitialized: () => void;
}

/** 控制台首页（未登录）：产品简介、安全机制与轨迹读数 + 登录卡片 + 题型体验 */
export function LoginPage({ onLogin, onNotInitialized }: LoginPageProps) {
  return (
    <div className="guest guest--login">
      <GuestBackdrop />
      <section className="guest__hero">
        <div className="login-intro">
          <div>
            <span className="login-intro__badge">
              <Icon name="shield-check" size={16} />
              行为验证码管理后台
            </span>
            <h1 className="login-intro__title">
              自托管<span className="login-intro__accent">行为验证码</span>服务
            </h1>
            <p className="login-intro__lead">
              管理员登录后统一管理接入应用、风控策略与背景图。登录本身同样受行为验证码保护，每次尝试都要重新完成一次验证。
            </p>
          </div>

          <div className="login-facts">
            <h2 className="login-facts__title">核心安全机制</h2>
            <ul className="login-facts__grid">
              {FACTS.map((fact) => (
                <li key={fact.name} className="login-fact">
                  <div className="login-fact__head">
                    <span className="login-fact__icon">
                      <Icon name={fact.icon} size={18} />
                    </span>
                    <h3 className="login-fact__name">{fact.name}</h3>
                  </div>
                  <p className="login-fact__desc">{fact.desc}</p>
                </li>
              ))}
            </ul>
          </div>

          <TrailMeter />
        </div>

        <LoginCard onLogin={onLogin} onNotInitialized={onNotInitialized} />
      </section>

      <CaptchaShowcase />
    </div>
  );
}

interface LoginError {
  /** 用户主动关掉验证弹窗：用中性样式提示，不当作错误 */
  cancelled: boolean;
  text: string;
}

function LoginCard({ onLogin, onNotInitialized }: LoginPageProps) {
  const { verify, holder } = useCaptcha();
  const copy = useCopy();
  const usernameId = useId();
  const passwordId = useId();
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [stage, setStage] = useState<Stage>('idle');
  const [error, setError] = useState<LoginError | null>(null);

  const busy = stage !== 'idle';
  const view = STAGE_VIEW[stage];

  const handleSubmit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (busy) {
      return;
    }

    const name = username.trim();
    if (!name || !password) {
      setError({ cancelled: false, text: '请输入用户名和密码' });
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
      setError({
        cancelled,
        text: cancelled ? '你已取消本次验证' : getErrorMessage(err, '验证未完成'),
      });
      setStage('idle');
      return;
    }

    setStage('submitting');
    try {
      onLogin(await adminApi.login({ username: name, password, captchaKey: secondKey }));
    } catch (err) {
      if (isAdminApiError(err) && err.code === ADMIN_ERROR_CODE.notInitialized) {
        onNotInitialized();
        return;
      }
      setError({ cancelled: false, text: getErrorMessage(err) });
      setStage('idle');
    }
  };

  return (
    <>
      {/* 弹窗里的按钮不能落在 form 内，否则会触发提交 */}
      <div className="captcha-holder">{holder}</div>

      <div className="login-card">
        <div className="login-card__head">
          <div className="login-card__title-row">
            <h2 className="login-card__title">管理员登录</h2>
            <span className="login-card__chip">
              <Icon name="shield" size={13} />
              行为验证防护
            </span>
          </div>
          <p className="login-card__desc">提交前先完成一次行为验证，每次尝试都需要重新验证。</p>
        </div>

        <form className="login-form" onSubmit={handleSubmit}>
          <div className="field guest-field">
            <label className="field__label" htmlFor={usernameId}>
              用户名
            </label>
            <div className="guest-field__control">
              <input
                id={usernameId}
                className="field__input"
                value={username}
                onChange={(e) => setUsername(e.target.value)}
                placeholder="请输入管理员用户名"
                autoComplete="username"
                disabled={busy}
              />
              <Icon name="user" size={18} className="guest-field__icon" />
            </div>
          </div>

          <div className="field guest-field">
            <label className="field__label" htmlFor={passwordId}>
              密码
            </label>
            <div className="guest-field__control">
              <input
                id={passwordId}
                className="field__input"
                type="password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                placeholder="请输入密码"
                autoComplete="current-password"
                disabled={busy}
              />
              <Icon name="lock" size={18} className="guest-field__icon" />
            </div>
          </div>

          <button
            type="submit"
            className="btn btn--primary btn--block login-form__submit"
            disabled={busy}
          >
            <Icon name={view.icon} size={18} className={STAGE_ICON_CLASS[stage]} />
            {view.text}
          </button>

          {error && (
            <div className={error.cancelled ? 'alert' : 'alert alert--error'} role="alert">
              <Icon name={error.cancelled ? 'x-circle' : 'error'} size={16} />
              <span>{error.text}</span>
            </div>
          )}
        </form>

        <div className="login-recover">
          <div className="login-recover__title">
            <Icon name="terminal" size={16} />
            账号初始化与恢复
          </div>
          <p className="login-recover__text">
            用户名与密码在首次初始化时设置；遗失时在服务器上（容器部署时进入容器）执行下面的命令，再用输出的新口令重新初始化。
          </p>
          <div className="login-recover__cmd">
            <code>{RESET_COMMAND}</code>
            <button
              type="button"
              className="login-recover__copy"
              onClick={() => copy(RESET_COMMAND, '已复制命令')}
              aria-label="复制命令"
              title="复制命令"
            >
              <Icon name="copy" size={14} />
            </button>
          </div>
        </div>
      </div>
    </>
  );
}
