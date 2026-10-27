import { useId, useState } from 'react';
import type { FormEvent, ReactNode } from 'react';
import { ADMIN_ERROR_CODE, adminApi, getErrorMessage, isAdminApiError } from '../api';
import type { LoginResponse } from '../api';
import { Icon } from '../components/Icon';
import type { IconName } from '../components/Icon';
import { CaptchaShowcase } from './CaptchaShowcase';
import { GuestBackdrop } from './GuestBackdrop';
import './GuestPages.css';

/** 与服务端一致：用户名 1-64 个字符；密码至少 10 个字符、不超过 72 字节（bcrypt 上限） */
const USERNAME_MAX = 64;
const PASSWORD_MIN = 10;
const PASSWORD_MAX_BYTES = 72;

const LEAD =
  '服务已启动，还没有管理员账号。填写服务启动日志中的初始化口令，设置管理员用户名与密码；完成后即可进入控制台配置接入应用、允许来源与风控策略。';

const STEPS: ReadonlyArray<{ name: string; sub: ReactNode }> = [
  {
    name: '查看启动日志中的初始化口令',
    sub: <code className="setup-step__cmd">docker logs &lt;容器名&gt;</code>,
  },
  {
    name: '设置管理员用户名与密码',
    sub: (
      <span className="setup-step__sub">只有一个管理员账号，之后可在「账号与安全」里修改密码</span>
    ),
  },
  {
    name: '进入控制台创建接入应用',
    sub: <span className="setup-step__sub">应用 ID 交给前端，应用密钥交给业务后端</span>,
  },
];

/** 两张说明卡，内容以 backend 的实际行为为准（services/admin_credential.go） */
const NOTES: ReadonlyArray<{ icon: IconName; title: string; desc: string }> = [
  {
    icon: 'shield-check',
    title: '一次性初始化口令',
    desc: '只打印在服务启动日志中，每次启动都会更换，以最近一次为准；初始化完成后立即作废，库中只存哈希。',
  },
  {
    icon: 'server',
    title: '本地单实例凭据',
    desc: '账号保存在本地 SQLite（data/captcha.db），密码只存 bcrypt 哈希，不依赖外部认证服务。',
  },
];

const byteLength = (value: string) => new TextEncoder().encode(value).length;

const passwordValid = (password: string) =>
  [...password].length >= PASSWORD_MIN && byteLength(password) <= PASSWORD_MAX_BYTES;

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

/** 控制台首页（未初始化）：流程指引与说明 + 初始化卡片 + 紧凑版题型体验 */
export function SetupPage({ onSetup, onAlreadyInitialized }: SetupPageProps) {
  return (
    <div className="guest guest--setup">
      <GuestBackdrop />
      <section className="guest__hero">
        <div className="setup-intro">
          <div className="setup-intro__head">
            <span className="setup-intro__eyebrow">
              <Icon name="terminal" size={16} />
              管理后台初始化
            </span>
            <h1 className="setup-intro__title">服务首次启动 · 初始化管理员账号</h1>
            <p className="setup-intro__lead">{LEAD}</p>
          </div>

          <div className="setup-steps">
            <h2 className="setup-steps__title">初始化流程指引</h2>
            <ol className="setup-steps__list">
              {STEPS.map((step, index) => (
                <li key={step.name} className="setup-step">
                  <span className={`setup-step__num setup-step__num--${index + 1}`}>
                    {index + 1}
                  </span>
                  <div className="setup-step__body">
                    <span className="setup-step__name">{step.name}</span>
                    {step.sub}
                  </div>
                </li>
              ))}
            </ol>
          </div>

          <div className="setup-notes">
            {NOTES.map((note) => (
              <div key={note.title} className="setup-note">
                <span className="setup-note__icon">
                  <Icon name={note.icon} size={18} />
                </span>
                <h3 className="setup-note__title">{note.title}</h3>
                <p className="setup-note__desc">{note.desc}</p>
              </div>
            ))}
          </div>
        </div>

        <SetupForm onSetup={onSetup} onAlreadyInitialized={onAlreadyInitialized} />
      </section>

      <CaptchaShowcase variant="compact" />
    </div>
  );
}

function SetupForm({ onSetup, onAlreadyInitialized }: SetupPageProps) {
  const ids = {
    code: useId(),
    codeHint: useId(),
    username: useId(),
    password: useId(),
    passwordHint: useId(),
    confirm: useId(),
  };
  const [code, setCode] = useState('');
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [confirm, setConfirm] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const passwordOk = passwordValid(password);
  const confirmOk = confirm !== '' && confirm === password;

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
    <div className="setup-card">
      <div className="setup-card__head">
        <div>
          <h2 className="setup-card__title">创建管理员账号</h2>
          <p className="setup-card__desc">全部为必填项，初始化成功后直接进入控制台</p>
        </div>
        <Icon name="shield-lock" size={22} />
      </div>

      <form className="setup-form" onSubmit={handleSubmit}>
        <div className="field guest-field">
          <div className="guest-field__row">
            <label className="field__label" htmlFor={ids.code}>
              初始化口令
              <span className="guest-field__required" aria-hidden="true">
                *
              </span>
            </label>
            <span className="guest-field__aside">格式 XXXX-XXXX-XXXX，自动转大写</span>
          </div>
          <div className="guest-field__control">
            <input
              id={ids.code}
              className="field__input setup-form__code"
              value={code}
              onChange={(e) => setCode(e.target.value.toUpperCase())}
              placeholder="XXXX-XXXX-XXXX"
              autoComplete="off"
              autoCapitalize="characters"
              spellCheck={false}
              aria-describedby={ids.codeHint}
              disabled={busy}
            />
            <Icon name="key" size={18} className="guest-field__icon" />
          </div>
          <span id={ids.codeHint} className="field__hint">
            见服务启动日志（例如 docker logs）；遗失时重启服务即可获得新口令
          </span>
        </div>

        <div className="field guest-field">
          <div className="guest-field__row">
            <label className="field__label" htmlFor={ids.username}>
              管理员用户名
              <span className="guest-field__required" aria-hidden="true">
                *
              </span>
            </label>
            <span className="guest-field__aside">最长 {USERNAME_MAX} 个字符</span>
          </div>
          <div className="guest-field__control">
            <input
              id={ids.username}
              className="field__input"
              value={username}
              onChange={(e) => setUsername(e.target.value)}
              placeholder="例如 admin"
              autoComplete="username"
              disabled={busy}
            />
            <Icon name="user" size={18} className="guest-field__icon" />
          </div>
        </div>

        <div className="field guest-field">
          <label className="field__label" htmlFor={ids.password}>
            密码
            <span className="guest-field__required" aria-hidden="true">
              *
            </span>
          </label>
          <div className="guest-field__control">
            <input
              id={ids.password}
              className="field__input"
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              autoComplete="new-password"
              aria-describedby={ids.passwordHint}
              disabled={busy}
            />
            <Icon name="lock" size={18} className="guest-field__icon" />
          </div>
          <span
            id={ids.passwordHint}
            className={passwordOk ? 'setup-form__rule setup-form__rule--ok' : 'setup-form__rule'}
          >
            <span className="setup-form__rule-dot" aria-hidden="true" />
            至少 {PASSWORD_MIN} 个字符，不超过 {PASSWORD_MAX_BYTES} 字节
          </span>
        </div>

        <div className="field guest-field">
          <label className="field__label" htmlFor={ids.confirm}>
            确认密码
            <span className="guest-field__required" aria-hidden="true">
              *
            </span>
          </label>
          <div className="guest-field__control">
            <input
              id={ids.confirm}
              className="field__input"
              type="password"
              value={confirm}
              onChange={(e) => setConfirm(e.target.value)}
              autoComplete="new-password"
              disabled={busy}
            />
            <Icon
              name="check-circle"
              size={18}
              className={
                confirmOk ? 'guest-field__icon guest-field__icon--ok' : 'guest-field__icon'
              }
            />
          </div>
        </div>

        <button
          type="submit"
          className="btn btn--primary btn--lg btn--block setup-form__submit"
          disabled={busy}
        >
          {busy ? (
            <>
              <Icon name="spinner" size={18} className="spin" />
              初始化中…
            </>
          ) : (
            <>
              完成初始化并进入控制台
              <Icon name="arrow-right" size={18} />
            </>
          )}
        </button>

        {error && (
          <div className="alert alert--error" role="alert">
            <Icon name="error" size={16} />
            <span>{error}</span>
          </div>
        )}
      </form>
    </div>
  );
}
