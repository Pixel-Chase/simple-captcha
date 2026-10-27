import { useEffect, useState } from 'react';
import type { FormEvent, ReactNode } from 'react';
import { adminApi, formatDateTime, getErrorMessage, isAdminApiError } from '../api';
import type { LoginResponse } from '../api';
import { Icon } from '../components/Icon';
import { useCopy } from '../components/Toast';
import './AccountPage.css';

/** 与服务端一致：至少 10 个字符，不超过 72 字节（bcrypt 上限） */
const MIN_LENGTH = 10;
const MAX_BYTES = 72;

/** 忘记密码时在服务器上执行的命令（README「首次初始化」） */
const RESET_COMMAND = './simple-captcha reset-admin';
const RESET_COMMAND_DOCKER = 'docker compose exec captcha ./simple-captcha reset-admin';

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

type RuleState = 'pending' | 'ok' | 'fail';

/**
 * 新密码规则的实时状态：输入过程中尚未满足的显示为待完成，
 * 已经确定违反的（超出字节数、与当前密码相同）显示为不通过
 */
function passwordRules(current: string, next: string): { label: string; state: RuleState }[] {
  return [
    {
      label: `至少 ${MIN_LENGTH} 个字符`,
      state: [...next].length >= MIN_LENGTH ? 'ok' : 'pending',
    },
    {
      label: `不超过 ${MAX_BYTES} 字节（中文一般每字 3 字节）`,
      state: !next ? 'pending' : byteLength(next) <= MAX_BYTES ? 'ok' : 'fail',
    },
    {
      label: '不能与当前密码相同',
      state: !next || !current ? 'pending' : next === current ? 'fail' : 'ok',
    },
  ];
}

const RULE_ICONS = { pending: 'circle', ok: 'check-circle', fail: 'x-circle' } as const;

/** 会话剩余时长；已过期返回 null */
function formatRemaining(expiresAt: number, now: number): string | null {
  const seconds = expiresAt - Math.floor(now / 1000);
  if (seconds <= 0) {
    return null;
  }
  const minutes = Math.floor(seconds / 60);
  if (minutes < 1) {
    return '不到 1 分钟后过期';
  }
  const days = Math.floor(minutes / 1440);
  const hours = Math.floor((minutes % 1440) / 60);
  if (days > 0) {
    return `还有 ${days} 天 ${hours} 小时后过期`;
  }
  if (hours > 0) {
    return `还有 ${hours} 小时 ${minutes % 60} 分钟后过期`;
  }
  return `还有 ${minutes} 分钟后过期`;
}

/** 当前时间，按 interval 毫秒刷新，用于剩余时长显示 */
function useNow(interval: number) {
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    const timer = setInterval(() => setNow(Date.now()), interval);
    return () => clearInterval(timer);
  }, [interval]);
  return now;
}

interface AccountPageProps {
  token: string;
  username: string;
  /** 当前会话到期时间，Unix 秒 */
  expiresAt: number;
  onChanged: (result: LoginResponse) => void;
  onSessionExpired: () => void;
}

export function AccountPage({
  token,
  username,
  expiresAt,
  onChanged,
  onSessionExpired,
}: AccountPageProps) {
  const copy = useCopy();
  const now = useNow(60_000);
  const [current, setCurrent] = useState('');
  const [next, setNext] = useState('');
  const [confirm, setConfirm] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [done, setDone] = useState(false);

  const rules = passwordRules(current, next);
  // 确认框输入到与新密码等长后仍不一致才提示，避免边输边报错
  const mismatch = !!confirm && confirm !== next && [...confirm].length >= [...next].length;
  const remaining = formatRemaining(expiresAt, now);

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

  const handleClear = () => {
    setCurrent('');
    setNext('');
    setConfirm('');
    setError(null);
  };

  return (
    <div className="page account">
      <header className="page-head account__head">
        <div>
          <div className="page-head__title">
            <h1 className="page-title">账号与安全</h1>
            <span className="chip chip--primary">单管理员</span>
          </div>
          <p className="page-desc">修改控制台唯一管理员的登录密码，查看当前会话的有效期。</p>
        </div>
      </header>

      {done && (
        <div className="account-banner" role="status">
          <span className="account-banner__icon">
            <Icon name="check-circle" size={18} />
          </span>
          <div className="account-banner__body">
            <p className="account-banner__title">密码已修改，其他会话已全部退出</p>
            <p className="account-banner__desc">
              当前页面已换用新签发的会话（有效期重新计算），其他浏览器或设备需用新密码重新登录。
            </p>
          </div>
          <button
            type="button"
            className="btn btn--ghost btn--icon"
            aria-label="关闭提示"
            onClick={() => setDone(false)}
          >
            <Icon name="close" size={18} />
          </button>
        </div>
      )}

      <div className="account__grid">
        <div className="account__col">
          <section className="account-card" aria-labelledby="account-password-title">
            <div className="account-card__head">
              <div>
                <h2 className="account-card__title" id="account-password-title">
                  修改登录密码
                </h2>
                <p className="account-card__desc">更新唯一管理员的登录密码，请妥善保存。</p>
              </div>
              <span className="account-card__badge">
                <Icon name="key" size={20} />
              </span>
            </div>

            <form className="account-form" onSubmit={handleSubmit} noValidate>
              {/* 供密码管理器识别账号 */}
              <input type="hidden" autoComplete="username" value={username} readOnly />

              <PasswordField
                id="account-current"
                label="当前密码"
                value={current}
                onChange={setCurrent}
                autoComplete="current-password"
                placeholder="输入当前使用的密码"
                disabled={busy}
              />

              <PasswordField
                id="account-next"
                label="新密码"
                value={next}
                onChange={setNext}
                autoComplete="new-password"
                placeholder={`至少 ${MIN_LENGTH} 个字符`}
                disabled={busy}
                describedBy="account-rules"
              >
                <div className="account-rules" id="account-rules">
                  <span className="account-rules__title">密码规则（实时校验）</span>
                  <ul className="account-rules__list">
                    {rules.map((rule) => (
                      <li
                        key={rule.label}
                        className={`account-rules__item account-rules__item--${rule.state}`}
                      >
                        <Icon name={RULE_ICONS[rule.state]} size={14} />
                        <span>{rule.label}</span>
                      </li>
                    ))}
                  </ul>
                </div>
              </PasswordField>

              <PasswordField
                id="account-confirm"
                label="确认新密码"
                value={confirm}
                onChange={setConfirm}
                autoComplete="new-password"
                placeholder="再次输入新密码"
                disabled={busy}
                invalid={mismatch}
                describedBy={mismatch ? 'account-confirm-error' : undefined}
              >
                {mismatch && (
                  <p className="account-form__inline-error" id="account-confirm-error">
                    <Icon name="error" size={14} />
                    两次输入的新密码不一致
                  </p>
                )}
              </PasswordField>

              {error && (
                <div className="alert alert--error" role="alert">
                  <Icon name="error" size={16} />
                  <span>{error}</span>
                </div>
              )}

              <div className="account-form__actions">
                <button type="submit" className="btn btn--primary" disabled={busy}>
                  {busy && <Icon name="spinner" size={16} className="spin" />}
                  {busy ? '保存中…' : '保存新密码'}
                </button>
                <button
                  type="button"
                  className="btn account-form__clear"
                  onClick={handleClear}
                  disabled={busy || (!current && !next && !confirm && !error)}
                >
                  重置输入
                </button>
              </div>
            </form>
          </section>

          <div className="account-note">
            <Icon name="monitor" size={22} />
            <div>
              <h3 className="account-note__title">会话影响说明</h3>
              <p className="account-note__text">
                修改后，其他浏览器或设备上的登录会全部退出，当前页面自动换用新会话。
              </p>
            </div>
          </div>
        </div>

        <div className="account__col">
          <section
            className="account-card account-card--side"
            aria-labelledby="account-session-title"
          >
            <div className="account-card__head account-card__head--compact">
              <h2 className="account-card__subtitle" id="account-session-title">
                管理员与会话状态
              </h2>
            </div>
            <dl className="account-session">
              <div className="account-session__item">
                <dt className="account-session__label">当前管理员账号</dt>
                <dd className="account-session__user">
                  <Icon name="user" size={18} />
                  <span>{username}</span>
                </dd>
              </div>
              <div className="account-session__item">
                <dt className="account-session__label">当前会话到期时间</dt>
                <dd className="account-session__time">
                  {formatDateTime(new Date(expiresAt * 1000).toISOString())}
                </dd>
                <dd
                  className={`account-session__remain${remaining ? '' : ' account-session__remain--expired'}`}
                >
                  <Icon name="clock" size={14} />
                  {remaining ?? '会话已过期，下次操作时需要重新登录'}
                </dd>
              </div>
              <div className="account-session__item">
                <dt className="account-session__label">会话说明</dt>
                <dd>
                  <ul className="account-session__facts">
                    <li>
                      有效期由部署参数 <code>ADMIN_SESSION_TTL</code> 决定（默认 2
                      小时），从登录起计算，期间的操作不会顺延。
                    </li>
                    <li>
                      会话令牌保存在当前标签页的
                      sessionStorage：刷新页面仍保持登录，关闭标签页后需要重新登录。
                    </li>
                    <li>退出登录会立即作废当前会话。</li>
                  </ul>
                </dd>
              </div>
            </dl>
          </section>

          <section className="account-recovery" aria-labelledby="account-recovery-title">
            <div className="account-recovery__head">
              <Icon name="terminal" size={18} />
              <h2 className="account-recovery__title" id="account-recovery-title">
                忘记密码
              </h2>
            </div>
            <p className="account-recovery__lead">在服务器上执行：</p>
            <CommandLine
              command={RESET_COMMAND}
              onCopy={() => void copy(RESET_COMMAND, '已复制命令')}
            />
            <p className="account-recovery__alt">容器部署时在容器内执行：</p>
            <CommandLine
              command={RESET_COMMAND_DOCKER}
              onCopy={() => void copy(RESET_COMMAND_DOCKER, '已复制命令')}
            />
            <ul className="account-recovery__notes">
              <li>
                <Icon name="info" size={14} />
                <span>
                  服务无需重启。命令会打印新的初始化口令（之后若重启服务，以最近一次启动日志中的口令为准），在控制台初始化页面用它重新设置用户名与密码。
                </span>
              </li>
              <li>
                <Icon name="shield-check" size={14} className="account-recovery__ok" />
                <span>
                  只删除管理员账号与全部会话，接入应用、策略配置、背景图等其他数据都会保留。
                </span>
              </li>
            </ul>
          </section>
        </div>
      </div>
    </div>
  );
}

interface PasswordFieldProps {
  id: string;
  label: string;
  value: string;
  onChange: (value: string) => void;
  autoComplete: 'current-password' | 'new-password';
  placeholder: string;
  disabled: boolean;
  invalid?: boolean;
  describedBy?: string;
  /** 输入框下方的规则清单或错误提示 */
  children?: ReactNode;
}

/** 带显示 / 隐藏切换的密码输入框 */
function PasswordField({
  id,
  label,
  value,
  onChange,
  autoComplete,
  placeholder,
  disabled,
  invalid,
  describedBy,
  children,
}: PasswordFieldProps) {
  const [visible, setVisible] = useState(false);

  return (
    <div className="field">
      <label className="field__label" htmlFor={id}>
        {label}
        <span className="account-form__required" aria-hidden="true">
          *
        </span>
      </label>
      <div className="account-pw">
        <input
          id={id}
          className="field__input"
          type={visible ? 'text' : 'password'}
          value={value}
          onChange={(e) => onChange(e.target.value)}
          autoComplete={autoComplete}
          placeholder={placeholder}
          disabled={disabled}
          aria-required="true"
          aria-invalid={invalid || undefined}
          aria-describedby={describedBy}
        />
        <button
          type="button"
          className="account-pw__toggle"
          aria-label={visible ? `隐藏${label}` : `显示${label}`}
          aria-pressed={visible}
          disabled={disabled}
          onClick={() => setVisible((shown) => !shown)}
        >
          <Icon name={visible ? 'eye-off' : 'eye'} size={18} />
        </button>
      </div>
      {children}
    </div>
  );
}

/** 深色卡片里的一行命令，右侧复制 */
function CommandLine({ command, onCopy }: { command: string; onCopy: () => void }) {
  return (
    <div className="account-recovery__cmd">
      <code>{command}</code>
      <button
        type="button"
        className="account-recovery__copy"
        aria-label="复制命令"
        title="复制命令"
        onClick={onCopy}
      >
        <Icon name="copy" size={16} />
      </button>
    </div>
  );
}
