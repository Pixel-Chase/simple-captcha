import { useCallback, useEffect, useMemo, useState } from 'react';
import type { FormEvent } from 'react';
import { adminApi, getErrorMessage, isAdminApiError } from '../api';
import type { RuntimeSettings } from '../api';

type Group = keyof RuntimeSettings;

/** 表单里数字以字符串保存，允许输入过程中的空值；保存时再转成整数交给服务端校验 */
type FormValue = string | boolean;
type FormState = { [G in Group]: Record<string, FormValue> };

interface FieldBase {
  key: string;
  label: string;
  desc: string;
}

type Field =
  | (FieldBase & { kind: 'number'; unit: string; min: number; max: number })
  | (FieldBase & { kind: 'bool' })
  | (FieldBase & { kind: 'mode' });

interface GroupMeta {
  id: Group;
  title: string;
  desc: string;
  fields: Field[];
}

/** 字段说明取自 README「环境变量参考」原有的描述；取值范围与服务端校验一致，仅用于提示 */
const GROUPS: GroupMeta[] = [
  {
    id: 'captcha',
    title: '验证码',
    desc: '出题与校验的基本参数。',
    fields: [
      {
        key: 'defaultDifficulty',
        kind: 'number',
        label: '基础难度',
        desc: '基础难度等级 1-5，客户端风控状态与环境信号在此基础上加难。',
        unit: '级',
        min: 1,
        max: 5,
      },
      {
        key: 'expireSeconds',
        kind: 'number',
        label: '题目有效期',
        desc: '验证码过期时间。',
        unit: '秒',
        min: 30,
        max: 3600,
      },
      {
        key: 'secondVerifyExpireSeconds',
        kind: 'number',
        label: '二次验证有效期',
        desc: '通过后签发的 second_key 的有效期，业务后端需在此时间内完成 S2S 查询。',
        unit: '秒',
        min: 10,
        max: 3600,
      },
      {
        key: 'maxAttempts',
        kind: 'number',
        label: '最大尝试次数',
        desc: '同一道题最多提交的次数，用尽后作废。',
        unit: '次',
        min: 1,
        max: 20,
      },
      {
        key: 'minSolveTimeMs',
        kind: 'number',
        label: '最短作答时间',
        desc: '出题到提交的最短间隔，低于则判定为脚本。',
        unit: '毫秒',
        min: 0,
        max: 10000,
      },
      {
        key: 'ipEscalatePerMinute',
        kind: 'number',
        label: '单 IP 加难阈值',
        desc: '同一 IP 每分钟出题与提交超过该次数，后续题目加 1 级，超过 2 倍加 2 级；0 关闭。验证码请求不会因 IP 被拒绝。',
        unit: '次/分钟',
        min: 0,
        max: 100000,
      },
      {
        key: 'bindClient',
        kind: 'bool',
        label: '绑定客户端',
        desc: '是否将题目绑定到客户端（IP + UA 哈希），换客户端提交直接作废。',
      },
      {
        key: 'imageQuality',
        kind: 'number',
        label: '主图质量',
        desc: '主图 JPEG 质量 55-100。',
        unit: '',
        min: 55,
        max: 100,
      },
      {
        key: 'maxScale',
        kind: 'number',
        label: '最大出图倍率',
        desc: '允许的最大出图倍率 1-3（对应前端 devicePixelRatio）。',
        unit: '倍',
        min: 1,
        max: 3,
      },
    ],
  },
  {
    id: 'pow',
    title: '工作量证明',
    desc: '提交前在浏览器里计算的 SHA-256 难题，位数随难度提升。',
    fields: [
      { key: 'enabled', kind: 'bool', label: '下发 PoW 挑战', desc: '是否下发工作量证明挑战。' },
      {
        key: 'required',
        kind: 'bool',
        label: '强制要求 PoW',
        desc: '关闭时未提交 PoW 仍可通过，但计入风险分。',
      },
      {
        key: 'difficulty',
        kind: 'number',
        label: '基础难度',
        desc: 'PoW 基础难度（前导零位数）。',
        unit: '位',
        min: 1,
        max: 26,
      },
      {
        key: 'maxDifficulty',
        kind: 'number',
        label: '难度上限',
        desc: 'PoW 难度上限，不能小于基础难度。',
        unit: '位',
        min: 1,
        max: 26,
      },
    ],
  },
  {
    id: 'risk',
    title: '风控评分',
    desc: '行为轨迹与环境信号评分。调整前先在「风控观察」里对照真人与脚本的原因分布。',
    fields: [
      {
        key: 'enabled',
        kind: 'bool',
        label: '启用风控评分',
        desc: '是否启用行为 / 环境风控评分。',
      },
      {
        key: 'mode',
        kind: 'mode',
        label: '运行模式',
        desc: 'enforce 按阈值拦截；log 仅记录分数，用于灰度观察。',
      },
      {
        key: 'escalateThreshold',
        kind: 'number',
        label: '加难阈值',
        desc: '达到该分数后提升该客户端后续难度与 PoW 位数。',
        unit: '分',
        min: 0,
        max: 100,
      },
      {
        key: 'blockThreshold',
        kind: 'number',
        label: '拦截阈值',
        desc: '达到该分数且满足行为证据要求时拒绝本次校验，不能小于加难阈值。',
        unit: '分',
        min: 0,
        max: 100,
      },
      {
        key: 'blockMinBehavior',
        kind: 'number',
        label: '拦截所需最低行为分',
        desc: '触发拦截所需的最低行为分；命中 webdriver 等硬性自动化标记时不要求。',
        unit: '分',
        min: 0,
        max: 100,
      },
      {
        key: 'envScoreCap',
        kind: 'number',
        label: '环境分上限',
        desc: '环境 / 请求头类信号对风险分的贡献上限，0 不限制；仅凭环境信号达不到拦截阈值。',
        unit: '分',
        min: 0,
        max: 100,
      },
      {
        key: 'fpReplayThreshold',
        kind: 'number',
        label: '指纹重放阈值',
        desc: '同一设备指纹在时间窗内对应的不同客户端数超过该值即视为重放加难，0 关闭。',
        unit: '个客户端',
        min: 0,
        max: 1000,
      },
      {
        key: 'fpReplayWindowSeconds',
        kind: 'number',
        label: '指纹重放时间窗',
        desc: '指纹重放计数的时间窗。',
        unit: '秒',
        min: 10,
        max: 86400,
      },
      {
        key: 'stateTtlSeconds',
        kind: 'number',
        label: '客户端状态保留时长',
        desc: '客户端风控状态（近期失败次数、上次风险分）的保留时长。',
        unit: '秒',
        min: 60,
        max: 86400,
      },
      {
        key: 'telemetryRetention',
        kind: 'number',
        label: '风控观察保留条数',
        desc: '「风控观察」保留的最近记录条数（出题评估与校验评分共用），0 表示不记录。',
        unit: '条',
        min: 0,
        max: 1000000,
      },
    ],
  },
];

function toForm(settings: RuntimeSettings): FormState {
  const form = {} as FormState;
  for (const group of GROUPS) {
    const values = settings[group.id] as unknown as Record<string, number | boolean | string>;
    form[group.id] = {};
    for (const field of group.fields) {
      const value = values[field.key];
      form[group.id][field.key] = typeof value === 'boolean' ? value : String(value);
    }
  }
  return form;
}

/** 表单转回接口结构；数字字段必须是整数，否则返回提示文案 */
function fromForm(form: FormState): RuntimeSettings | string {
  const out: Record<string, Record<string, number | boolean | string>> = {};
  for (const group of GROUPS) {
    out[group.id] = {};
    for (const field of group.fields) {
      const value = form[group.id][field.key];
      if (field.kind === 'number') {
        const text = String(value).trim();
        if (!/^-?\d+$/.test(text)) {
          return `「${group.title} · ${field.label}」请输入整数`;
        }
        out[group.id][field.key] = Number(text);
      } else {
        out[group.id][field.key] = value;
      }
    }
  }
  return out as unknown as RuntimeSettings;
}

const formatDefault = (field: Field, value: number | boolean | string) => {
  if (field.kind === 'bool') {
    return value ? '开启' : '关闭';
  }
  if (field.kind === 'number' && field.unit) {
    return `${value} ${field.unit}`;
  }
  return String(value);
};

interface SettingsPageProps {
  token: string;
  onSessionExpired: () => void;
}

export function SettingsPage({ token, onSessionExpired }: SettingsPageProps) {
  const [saved, setSaved] = useState<RuntimeSettings | null>(null);
  const [defaults, setDefaults] = useState<RuntimeSettings | null>(null);
  const [form, setForm] = useState<FormState | null>(null);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);

  const handleApiError = useCallback(
    (err: unknown, fallback: string): string | null => {
      if (isAdminApiError(err) && err.status === 401) {
        onSessionExpired();
        return null;
      }
      return getErrorMessage(err, fallback);
    },
    [onSessionExpired],
  );

  const load = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const data = await adminApi.getSettings(token);
      setSaved(data.settings);
      setDefaults(data.defaults);
      setForm(toForm(data.settings));
    } catch (err) {
      const message = handleApiError(err, '加载策略配置失败');
      if (message) {
        setError(message);
      }
    } finally {
      setLoading(false);
    }
  }, [token, handleApiError]);

  useEffect(() => {
    void load();
  }, [load]);

  const dirty = useMemo(
    () => !!form && !!saved && JSON.stringify(form) !== JSON.stringify(toForm(saved)),
    [form, saved],
  );

  const update = (group: Group, key: string, value: FormValue) => {
    setNotice(null);
    setForm((prev) => (prev ? { ...prev, [group]: { ...prev[group], [key]: value } } : prev));
  };

  const handleReset = () => {
    if (defaults) {
      setNotice(null);
      setError(null);
      setForm(toForm(defaults));
    }
  };

  const handleSubmit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!form || !dirty || saving) {
      return;
    }
    const next = fromForm(form);
    if (typeof next === 'string') {
      setError(next);
      return;
    }
    setSaving(true);
    setError(null);
    setNotice(null);
    try {
      const result = await adminApi.updateSettings(token, next);
      setSaved(result.settings);
      setForm(toForm(result.settings));
      setNotice('已保存并生效');
    } catch (err) {
      const message = handleApiError(err, '保存失败');
      if (message) {
        setError(message);
      }
    } finally {
      setSaving(false);
    }
  };

  return (
    <section className="settings">
      <div className="images__head">
        <div>
          <p className="eyebrow">Strategy</p>
          <h1>策略配置</h1>
          <p className="muted">
            验证码、工作量证明与风控评分的参数。保存后下一次出题即生效，无需重启；只保存当前值，不记录修改历史。
          </p>
        </div>
      </div>

      {loading && !form && <div className="page-status">正在加载…</div>}

      {form && defaults && (
        <form className="settings__form" onSubmit={handleSubmit} noValidate>
          {GROUPS.map((group) => (
            <fieldset key={group.id} className="card settings__group" disabled={saving}>
              <legend className="settings__legend">{group.title}</legend>
              <p className="muted settings__group-desc">{group.desc}</p>
              <div className="settings__rows">
                {group.fields.map((field) => {
                  const id = `setting-${group.id}-${field.key}`;
                  const value = form[group.id][field.key];
                  const defaultValue = (
                    defaults[group.id] as unknown as Record<string, number | boolean | string>
                  )[field.key];
                  return (
                    <div key={field.key} className="settings__row">
                      <div className="settings__text">
                        <label className="settings__label" htmlFor={id}>
                          {field.label}
                        </label>
                        <p className="settings__desc">{field.desc}</p>
                      </div>
                      <div className="settings__control">
                        {field.kind === 'number' && (
                          <span className="settings__number">
                            <input
                              id={id}
                              className="field__input settings__input"
                              type="number"
                              inputMode="numeric"
                              min={field.min}
                              max={field.max}
                              step={1}
                              value={String(value)}
                              onChange={(e) => update(group.id, field.key, e.target.value)}
                            />
                            {field.unit && <span className="settings__unit">{field.unit}</span>}
                          </span>
                        )}
                        {field.kind === 'bool' && (
                          <label className="settings__switch">
                            <input
                              id={id}
                              type="checkbox"
                              checked={value === true}
                              onChange={(e) => update(group.id, field.key, e.target.checked)}
                            />
                            <span>{value === true ? '开启' : '关闭'}</span>
                          </label>
                        )}
                        {field.kind === 'mode' && (
                          <select
                            id={id}
                            className="field__input settings__input"
                            value={String(value)}
                            onChange={(e) => update(group.id, field.key, e.target.value)}
                          >
                            <option value="enforce">enforce · 拦截</option>
                            <option value="log">log · 只记录</option>
                          </select>
                        )}
                        <span className="settings__default">
                          默认 {formatDefault(field, defaultValue)}
                          {field.kind === 'number' && ` · 范围 ${field.min}～${field.max}`}
                        </span>
                      </div>
                    </div>
                  );
                })}
              </div>
            </fieldset>
          ))}

          <div className="card settings__actions">
            <div className="settings__status">
              {error && (
                <div className="alert alert--error" role="alert">
                  {error}
                </div>
              )}
              {notice && !error && (
                <div className="alert alert--success" role="status">
                  {notice}
                </div>
              )}
              {!error && !notice && (
                <span className="muted">{dirty ? '有未保存的修改' : '当前没有修改'}</span>
              )}
            </div>
            <button
              type="button"
              className="btn btn--ghost"
              onClick={handleReset}
              disabled={saving}
            >
              恢复全部默认值
            </button>
            <button type="submit" className="btn btn--primary" disabled={!dirty || saving}>
              {saving ? '保存中…' : '保存'}
            </button>
          </div>
        </form>
      )}

      {!form && error && (
        <div className="alert alert--error" role="alert">
          {error}
        </div>
      )}
    </section>
  );
}
