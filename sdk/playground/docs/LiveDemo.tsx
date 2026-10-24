import { useState } from 'react';
import { useCaptcha } from '@aetherlib/g-captcha';
import type { CaptchaType } from '@aetherlib/g-captcha';
import { API_BASE, getErrorMessage } from '../api';
import { CAPTCHA_TYPE_OPTIONS, captchaTypeLabel } from '../captcha-options';
import { DEMO_ID } from './content';

interface DemoResult {
  secondKey: string;
  type: CaptchaType;
  at: Date;
}

const SNIPPET = `const { verify, holder } = useCaptcha();
const { second_key } = await verify({ type: 'auto' });
// 把 second_key 随业务表单提交，由业务后端做 S2S 校验`;

/** 文档页顶部的在线体验：走一遍真实的取题 → 验证 → 拿到 second_key 流程 */
export function LiveDemo() {
  const { verify, holder } = useCaptcha();
  const [type, setType] = useState<CaptchaType>('auto');
  const [busy, setBusy] = useState(false);
  const [result, setResult] = useState<DemoResult | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [copied, setCopied] = useState(false);

  const curl = result
    ? `curl -H "Authorization: Bearer <API_KEY>" \\\n  ${API_BASE}/api/v1/captchas/states/${result.secondKey}`
    : '';

  const run = async () => {
    if (busy) {
      return;
    }
    setBusy(true);
    setError(null);
    setCopied(false);
    try {
      const data = await verify({ type });
      setResult({ secondKey: data.second_key, type, at: new Date() });
    } catch (err) {
      setError(getErrorMessage(err, '验证未完成'));
    } finally {
      setBusy(false);
    }
  };

  const copy = async () => {
    try {
      await navigator.clipboard.writeText(curl);
      setCopied(true);
    } catch {
      setCopied(false);
    }
  };

  return (
    <section id={DEMO_ID} className="demo card">
      <div className="captcha-holder">{holder}</div>

      <div className="demo__intro">
        <p className="eyebrow">在线体验</p>
        <h2>先完整走一遍流程</h2>
        <p>
          选择题型后点击按钮，SDK 会向 <code>{API_BASE}</code> 取题并弹出验证。通过后得到一次性的{' '}
          <code>second_key</code>，业务后端再用它向验证码服务做二次确认，前端不需要也不应该持有{' '}
          <code>API_KEY</code>。
        </p>
        <pre className="demo__snippet">
          <code>{SNIPPET}</code>
        </pre>
      </div>

      <div className="demo__panel">
        <label className="field">
          <span className="field__label">验证方式</span>
          <select
            className="field__input"
            value={type}
            onChange={(e) => setType(e.target.value as CaptchaType)}
            disabled={busy}
          >
            {CAPTCHA_TYPE_OPTIONS.map((item) => (
              <option key={item.value} value={item.value}>
                {item.label}
              </option>
            ))}
          </select>
        </label>

        <button type="button" className="btn btn--primary btn--block" onClick={run} disabled={busy}>
          {busy ? '请完成行为验证…' : '开始验证'}
        </button>

        {error && (
          <div className="alert alert--error" role="alert">
            {error}
          </div>
        )}

        {result && (
          <div className="demo__result">
            <div className="demo__result-head">
              <span className="badge badge--good">验证通过</span>
              <span className="muted">
                {result.at.toLocaleTimeString('zh-CN', { hour12: false })} ·{' '}
                {captchaTypeLabel(result.type)}
              </span>
            </div>

            <div className="field__label">second_key</div>
            <code className="demo__key">{result.secondKey}</code>

            <div className="field__label">业务后端二次校验</div>
            <pre className="demo__snippet">
              <code>{curl}</code>
            </pre>
            <div className="demo__actions">
              <button type="button" className="btn btn--ghost btn--sm" onClick={copy}>
                {copied ? '已复制' : '复制命令'}
              </button>
              <span className="muted">
                second_key 只能消费一次；开发环境的 API_KEY 见 backend/.env.development
              </span>
            </div>
          </div>
        )}
      </div>
    </section>
  );
}
