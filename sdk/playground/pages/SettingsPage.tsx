import { useCallback, useEffect, useMemo, useState } from 'react';
import type { FormEvent, ReactNode } from 'react';
import { adminApi, getErrorMessage, isAdminApiError } from '../api';
import type { RuntimeSettings } from '../api';
import { Icon } from '../components/Icon';
import { useToast } from '../components/Toast';
import type { ConsoleSection } from '../console/ConsoleView';
import './SettingsPage.css';

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
  /** stepper：取值只有几档的字段用 −/+ 步进器加等级方块代替输入框 */
  | (FieldBase & { kind: 'number'; unit: string; min: number; max: number; stepper?: boolean })
  | (FieldBase & { kind: 'bool' })
  | (FieldBase & { kind: 'mode' });

interface GroupMeta {
  id: Group;
  title: string;
  /** 分组标题后括号里的简介 */
  desc: string;
  fields: Field[];
}

/** 字段说明取自 README「环境变量参考」原有的描述；取值范围与服务端校验一致，仅用于提示 */
const GROUPS: GroupMeta[] = [
  {
    id: 'captcha',
    title: '验证码',
    desc: '出题与校验的基本参数',
    fields: [
      {
        key: 'defaultDifficulty',
        kind: 'number',
        label: '基础难度',
        desc: '基础难度等级 1-5，客户端风控状态与环境信号在此基础上加难。',
        unit: '级',
        min: 1,
        max: 5,
        stepper: true,
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
    desc: '提交前在浏览器里计算的 SHA-256 难题，位数随难度提升',
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
    desc: '行为轨迹与环境信号评分',
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

const GROUP_NUMERALS = ['一', '二', '三'];

const MODE_OPTIONS = [
  { value: 'enforce', hint: '按阈值拦截' },
  { value: 'log', hint: '只记录分数' },
] as const;

/** 服务端的两条关联校验（backend/config/runtime.go）：上限一侧不能小于下限一侧 */
const RELATIONS = [
  {
    group: 'pow',
    title: '工作量证明',
    upper: { key: 'maxDifficulty', label: '难度上限' },
    lower: { key: 'difficulty', label: '基础难度' },
    unit: '位',
  },
  {
    group: 'risk',
    title: '风控评分',
    upper: { key: 'blockThreshold', label: '拦截阈值' },
    lower: { key: 'escalateThreshold', label: '加难阈值' },
    unit: '分',
  },
] as const;

const INTEGER = /^-?\d+$/;

/** 字段在接口里的路径，与服务端校验错误里的字段名一致（如 pow.maxDifficulty） */
const fieldPath = (group: Group, field: Field) => `${group}.${field.key}`;

/** 字段路径 → 界面名称，用来把服务端错误里的字段名换成「分组 · 名称」 */
const FIELD_NAMES = new Map(
  GROUPS.flatMap((group) =>
    group.fields.map((field) => [fieldPath(group.id, field), `${group.title} · ${field.label}`]),
  ),
);

/** 不同分组里重名的字段（两个「基础难度」），变更摘要里带上分组名区分 */
const DUPLICATE_LABELS = (() => {
  const seen = new Set<string>();
  const duplicates = new Set<string>();
  for (const field of GROUPS.flatMap((group) => group.fields)) {
    if (seen.has(field.label)) {
      duplicates.add(field.label);
    }
    seen.add(field.label);
  }
  return duplicates;
})();

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
        if (!INTEGER.test(text)) {
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

/** 比较用的规范值：数字字段去掉空白与前导零（"03" 与 "3" 视为相同），其余原样 */
function normalize(field: Field, value: FormValue): string {
  if (field.kind !== 'number') {
    return String(value);
  }
  const text = String(value).trim();
  return INTEGER.test(text) ? String(Number(text)) : text;
}

/** 数字字段不是整数或超出取值范围：只用于标红提示，最终以服务端校验为准 */
function isOutOfRange(field: Field, value: FormValue): boolean {
  if (field.kind !== 'number') {
    return false;
  }
  const text = String(value).trim();
  if (!INTEGER.test(text)) {
    return true;
  }
  const number = Number(text);
  return number < field.min || number > field.max;
}

function formatValue(field: Field, value: FormValue): string {
  if (field.kind === 'bool') {
    return value ? '开启' : '关闭';
  }
  const text = String(value).trim();
  if (!text) {
    return '（空）';
  }
  return field.kind === 'number' && field.unit ? `${text} ${field.unit}` : text;
}

function formatRange(field: Field): string {
  if (field.kind === 'bool') {
    return '开启 / 关闭';
  }
  if (field.kind === 'mode') {
    return 'enforce / log';
  }
  return `${field.min} - ${field.max}${field.unit ? ` ${field.unit}` : ''}`;
}

interface Change {
  path: string;
  label: string;
  from: string;
  to: string;
}

/** 逐项比较两份表单，返回有差异的字段（顺序与页面一致） */
function diffForms(form: FormState, base: FormState): Change[] {
  const changes: Change[] = [];
  for (const group of GROUPS) {
    for (const field of group.fields) {
      const value = form[group.id][field.key];
      const before = base[group.id][field.key];
      if (normalize(field, value) !== normalize(field, before)) {
        changes.push({
          path: fieldPath(group.id, field),
          label: DUPLICATE_LABELS.has(field.label)
            ? `${group.title} · ${field.label}`
            : field.label,
          from: formatValue(field, before),
          to: formatValue(field, value),
        });
      }
    }
  }
  return changes;
}

/**
 * 服务端校验错误每行一个字段、以字段路径开头（如「pow.maxDifficulty 不能小于 pow.difficulty（14）」）：
 * 拆成多行、路径换成界面名称，并取出出错的字段用于标记对应行
 */
function parseSaveError(message: string): { lines: string[]; paths: Set<string> } {
  const paths = new Set<string>();
  const lines = message
    .split('\n')
    .map((line) => line.trim())
    .filter(Boolean)
    .map((line) => {
      const head = /^[a-z]+\.[A-Za-z]+/.exec(line)?.[0];
      if (head && FIELD_NAMES.has(head)) {
        paths.add(head);
      }
      return line.replace(/\s?\b[a-z]+\.[A-Za-z]+\b\s?/g, (match) => {
        const name = FIELD_NAMES.get(match.trim());
        return name ? `「${name}」` : match;
      });
    });
  return { lines, paths };
}

const readInt = (value: FormValue): number | null => {
  const text = String(value).trim();
  return INTEGER.test(text) ? Number(text) : null;
};

const formatRuleValue = (value: FormValue, unit: string) => {
  const number = readInt(value);
  return number === null ? '未填写' : `${number} ${unit}`;
};

interface SettingsPageProps {
  token: string;
  onSessionExpired: () => void;
  hrefFor: (section: ConsoleSection) => string;
}

export function SettingsPage({ token, onSessionExpired, hrefFor }: SettingsPageProps) {
  const toast = useToast();
  const [saved, setSaved] = useState<RuntimeSettings | null>(null);
  const [defaults, setDefaults] = useState<RuntimeSettings | null>(null);
  const [form, setForm] = useState<FormState | null>(null);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [saveError, setSaveError] = useState<string | null>(null);

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
    setLoadError(null);
    try {
      const data = await adminApi.getSettings(token);
      setSaved(data.settings);
      setDefaults(data.defaults);
      setForm(toForm(data.settings));
    } catch (err) {
      const message = handleApiError(err, '加载策略配置失败');
      if (message) {
        setLoadError(message);
      }
    } finally {
      setLoading(false);
    }
  }, [token, handleApiError]);

  useEffect(() => {
    void load();
  }, [load]);

  const savedForm = useMemo(() => (saved ? toForm(saved) : null), [saved]);
  const defaultForm = useMemo(() => (defaults ? toForm(defaults) : null), [defaults]);

  const changes = useMemo(
    () => (form && savedForm ? diffForms(form, savedForm) : []),
    [form, savedForm],
  );
  const dirty = changes.length > 0;
  const atDefaults = useMemo(
    () => !!form && !!defaultForm && diffForms(form, defaultForm).length === 0,
    [form, defaultForm],
  );
  const saveErrorView = useMemo(() => (saveError ? parseSaveError(saveError) : null), [saveError]);

  const update = (group: Group, key: string, value: FormValue) => {
    setSaveError(null);
    setForm((prev) => (prev ? { ...prev, [group]: { ...prev[group], [key]: value } } : prev));
  };

  const handleReset = () => {
    if (defaultForm) {
      setSaveError(null);
      setForm(defaultForm);
      toast('已填入默认值，保存后生效', 'info');
    }
  };

  const handleDiscard = () => {
    if (savedForm) {
      setSaveError(null);
      setForm(savedForm);
    }
  };

  const handleSubmit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!form || !dirty || saving) {
      return;
    }
    const next = fromForm(form);
    if (typeof next === 'string') {
      setSaveError(next);
      return;
    }
    setSaving(true);
    setSaveError(null);
    try {
      const result = await adminApi.updateSettings(token, next);
      setSaved(result.settings);
      setForm(toForm(result.settings));
      toast('已保存并生效');
    } catch (err) {
      const message = handleApiError(err, '保存失败');
      if (message) {
        setSaveError(message);
      }
    } finally {
      setSaving(false);
    }
  };

  if (!form || !savedForm || !defaultForm) {
    return (
      <div className="page settings">
        <SettingsHead />
        {loading ? (
          <div className="page-status">
            <Icon name="spinner" className="spin" /> 正在加载策略配置…
          </div>
        ) : (
          <div className="panel settings__failed">
            <div className="alert alert--error" role="alert">
              <Icon name="error" size={16} />
              <span>{loadError ?? '加载策略配置失败'}</span>
            </div>
            <button type="button" className="btn" onClick={() => void load()}>
              <Icon name="refresh" size={16} />
              重新加载
            </button>
          </div>
        )}
      </div>
    );
  }

  return (
    <form className="page settings" onSubmit={handleSubmit} noValidate>
      <SettingsHead dirtyCount={changes.length}>
        <button
          type="button"
          className="btn"
          onClick={handleReset}
          disabled={saving || atDefaults}
          title={atDefaults ? '当前已全部是默认值' : undefined}
        >
          <Icon name="refresh" size={16} />
          恢复全部默认值
        </button>
        <button type="submit" className="btn btn--primary" disabled={!dirty || saving}>
          <Icon name="check" size={16} />
          保存并生效
        </button>
      </SettingsHead>

      <div className="settings__hints">
        <div className="settings-hint">
          <span className="settings-hint__icon">
            <Icon name="shield" size={16} />
          </span>
          <div className="settings-hint__body">
            <p className="settings-hint__title">参数关联校验规则</p>
            <ul className="settings-hint__list">
              {RELATIONS.map((rule) => {
                const values = form[rule.group];
                const upper = readInt(values[rule.upper.key]);
                const lower = readInt(values[rule.lower.key]);
                const broken = upper !== null && lower !== null && upper < lower;
                return (
                  <li
                    key={rule.group}
                    className={broken ? 'settings-hint__rule--broken' : undefined}
                  >
                    {rule.title}：<strong>{rule.upper.label}</strong>（当前{' '}
                    {formatRuleValue(values[rule.upper.key], rule.unit)}）不能小于{' '}
                    <strong>{rule.lower.label}</strong>（当前{' '}
                    {formatRuleValue(values[rule.lower.key], rule.unit)}）
                    {broken && <span className="settings-hint__flag">· 当前不满足</span>}
                  </li>
                );
              })}
            </ul>
            <p className="settings-hint__foot">不满足时服务端拒绝保存。</p>
          </div>
        </div>

        <div className="settings-hint settings-hint--link">
          <div className="settings-hint__body">
            <p className="settings-hint__title">
              <Icon name="activity" size={16} className="settings-hint__accent" />
              对照实际风险分布
            </p>
            <p className="settings-hint__text">
              调整前建议先去「风控观察」对照真人与脚本的原因分布。
            </p>
          </div>
          <a className="btn btn--sm settings-hint__go" href={hrefFor('risk')}>
            去观察
            <Icon name="arrow-right" size={14} />
          </a>
        </div>
      </div>

      {/* 保存期间整体禁用，避免提交中途改动 */}
      <fieldset className="settings__groups" disabled={saving}>
        {GROUPS.map((group, index) => (
          <section
            key={group.id}
            className="settings-group"
            aria-labelledby={`settings-${group.id}`}
          >
            <header className="settings-group__head">
              <span className={`settings-group__dot settings-group__dot--${group.id}`} />
              <h2 className="settings-group__title" id={`settings-${group.id}`}>
                {GROUP_NUMERALS[index]}、{group.title}（{group.desc}）
              </h2>
              <span className="settings-group__count">共 {group.fields.length} 项参数</span>
            </header>
            <div className="settings-group__rows">
              {group.fields.map((field) => {
                const path = fieldPath(group.id, field);
                const value = form[group.id][field.key];
                return (
                  <SettingRow
                    key={field.key}
                    id={`setting-${group.id}-${field.key}`}
                    field={field}
                    value={value}
                    defaultValue={defaultForm[group.id][field.key]}
                    changed={
                      normalize(field, value) !== normalize(field, savedForm[group.id][field.key])
                    }
                    serverError={!!saveErrorView?.paths.has(path)}
                    onChange={(next) => update(group.id, field.key, next)}
                  />
                );
              })}
            </div>
          </section>
        ))}
      </fieldset>

      {dirty && (
        <section className="settings-bar" aria-label="未保存的修改">
          {saveErrorView && (
            <div className="alert alert--error settings-bar__error" role="alert">
              <Icon name="error" size={16} />
              <div>
                <p className="settings-bar__error-title">保存失败，已保存的配置未改变</p>
                <ul className="settings-bar__error-list">
                  {saveErrorView.lines.map((line) => (
                    <li key={line}>{line}</li>
                  ))}
                </ul>
              </div>
            </div>
          )}
          <div className="settings-bar__main">
            <div className="settings-bar__summary">
              <span className="settings-bar__icon">
                <Icon name="warning" size={16} />
              </span>
              <div className="settings-bar__text">
                <p className="settings-bar__title">存在 {changes.length} 项未保存修改</p>
                <ul className="settings-bar__changes">
                  {changes.map((change) => (
                    <li key={change.path}>
                      {change.label}：{change.from} →{' '}
                      <span className="settings-bar__to">{change.to}</span>
                    </li>
                  ))}
                </ul>
              </div>
            </div>
            <div className="settings-bar__actions">
              <button
                type="button"
                className="btn btn--sm"
                onClick={handleDiscard}
                disabled={saving}
              >
                放弃未保存修改
              </button>
              <button type="submit" className="btn btn--primary" disabled={saving}>
                <Icon
                  name={saving ? 'spinner' : 'check'}
                  size={16}
                  className={saving ? 'spin' : undefined}
                />
                {saving ? '保存中…' : '保存并立即生效'}
              </button>
            </div>
          </div>
        </section>
      )}
    </form>
  );
}

interface SettingsHeadProps {
  /** 有修改时显示「有未保存的修改」；加载前不传 */
  dirtyCount?: number;
  children?: ReactNode;
}

function SettingsHead({ dirtyCount = 0, children }: SettingsHeadProps) {
  return (
    <header className="page-head settings__head">
      <div>
        <div className="page-head__title">
          <h1 className="page-title">策略配置</h1>
          {dirtyCount > 0 && (
            <span className="chip chip--warn">
              <span className="chip__dot settings__pulse" />
              有未保存的修改（{dirtyCount} 项）
            </span>
          )}
          <span className="settings__note">保存后下一次出题即生效，无需重启</span>
        </div>
        <p className="page-desc">
          配置验证码出题、工作量证明难题与风控评分参数。只保存当前值，不记录修改历史。
        </p>
      </div>
      {children && <div className="page-head__actions">{children}</div>}
    </header>
  );
}

interface SettingRowProps {
  id: string;
  field: Field;
  value: FormValue;
  defaultValue: FormValue;
  /** 与已保存的值不同 */
  changed: boolean;
  /** 服务端校验错误里点名了该字段 */
  serverError: boolean;
  onChange: (value: FormValue) => void;
}

function SettingRow({
  id,
  field,
  value,
  defaultValue,
  changed,
  serverError,
  onChange,
}: SettingRowProps) {
  const isDefault = normalize(field, value) === normalize(field, defaultValue);
  const labelId = `${id}-label`;
  const className = [
    'settings-row',
    changed && 'settings-row--dirty',
    serverError && 'settings-row--error',
  ]
    .filter(Boolean)
    .join(' ');

  return (
    <div className={className}>
      <div className="settings-row__text">
        <div className="settings-row__title">
          {/* 运行模式是一组单选，用 aria-labelledby 关联；其余字段用 label 关联到控件 */}
          {field.kind === 'mode' ? (
            <span className="settings-row__label" id={labelId}>
              {field.label}
            </span>
          ) : (
            <label className="settings-row__label" id={labelId} htmlFor={id}>
              {field.label}
            </label>
          )}
          {changed && <span className="chip chip--warn">已修改未保存</span>}
          {serverError && <span className="chip chip--danger">未通过校验</span>}
        </div>
        <p className="settings-row__desc">{field.desc}</p>
        <p className="settings-row__meta">
          <span>
            默认值：<strong>{formatValue(field, defaultValue)}</strong>
          </span>
          <span aria-hidden="true">·</span>
          <span>
            取值范围：<strong>{formatRange(field)}</strong>
          </span>
        </p>
      </div>

      <div className="settings-row__control">
        {field.kind === 'number' && field.stepper && (
          <LevelStepper id={id} field={field} value={value} onChange={onChange} />
        )}
        {field.kind === 'number' && !field.stepper && (
          <span
            className={`settings-num${isOutOfRange(field, value) ? ' settings-num--invalid' : ''}`}
          >
            <input
              id={id}
              className="settings-num__input"
              type="number"
              inputMode="numeric"
              min={field.min}
              max={field.max}
              step={1}
              value={String(value)}
              aria-invalid={isOutOfRange(field, value) || undefined}
              onChange={(e) => onChange(e.target.value)}
            />
            {field.unit && <span className="settings-num__unit">{field.unit}</span>}
          </span>
        )}
        {field.kind === 'bool' && (
          <label className="settings-switch">
            <input
              id={id}
              className="settings-switch__input"
              type="checkbox"
              role="switch"
              aria-checked={value === true}
              checked={value === true}
              onChange={(e) => onChange(e.target.checked)}
            />
            <span className="settings-switch__track" aria-hidden="true">
              <span className="settings-switch__thumb" />
            </span>
            <span className="settings-switch__text">{value === true ? '开启' : '关闭'}</span>
          </label>
        )}
        {field.kind === 'mode' && (
          <div className="settings-seg" role="radiogroup" aria-labelledby={labelId}>
            {MODE_OPTIONS.map((option) => {
              const checked = value === option.value;
              return (
                <label
                  key={option.value}
                  className={`settings-seg__opt${checked ? ' settings-seg__opt--active' : ''}`}
                >
                  <input
                    className="settings-seg__input"
                    type="radio"
                    name={id}
                    value={option.value}
                    checked={checked}
                    onChange={() => onChange(option.value)}
                  />
                  {option.value}（{option.hint}）
                </label>
              );
            })}
          </div>
        )}
        <span className="settings-row__default">
          {isDefault && <span className="chip chip--good">默认</span>}
        </span>
      </div>
    </div>
  );
}

interface LevelStepperProps {
  id: string;
  field: Extract<Field, { kind: 'number' }>;
  value: FormValue;
  onChange: (value: FormValue) => void;
}

/** 等级步进器：−/+ 逐级调整，右侧方块可直接点选等级 */
function LevelStepper({ id, field, value, onChange }: LevelStepperProps) {
  const parsed = readInt(value);
  const level = parsed === null ? field.min : Math.min(field.max, Math.max(field.min, parsed));
  const levels = Array.from({ length: field.max - field.min + 1 }, (_, i) => field.min + i);

  return (
    <>
      <span className="settings-stepper">
        <button
          type="button"
          className="settings-stepper__btn"
          aria-label="降低一级"
          disabled={level <= field.min}
          onClick={() => onChange(String(level - 1))}
        >
          <MinusIcon />
        </button>
        <output id={id} className="settings-stepper__value" aria-live="polite">
          <strong>{level}</strong> {field.unit}
        </output>
        <button
          type="button"
          className="settings-stepper__btn"
          aria-label="提高一级"
          disabled={level >= field.max}
          onClick={() => onChange(String(level + 1))}
        >
          <Icon name="plus" size={14} />
        </button>
      </span>
      <span className="settings-levels">
        {levels.map((n) => (
          <button
            key={n}
            type="button"
            className={`settings-level${n < level ? ' settings-level--on' : ''}${n === level ? ' settings-level--current' : ''}`}
            aria-label={`设为 ${n} ${field.unit}`}
            aria-pressed={n === level}
            onClick={() => onChange(String(n))}
          >
            {n}
          </button>
        ))}
      </span>
    </>
  );
}

/** 共享图标里没有减号，步进器就地画一个，线宽与 Icon 一致 */
function MinusIcon() {
  return (
    <svg
      className="icon"
      viewBox="0 0 24 24"
      width={14}
      height={14}
      fill="none"
      stroke="currentColor"
      strokeWidth="1.8"
      strokeLinecap="round"
      aria-hidden="true"
    >
      <path d="M5 12h14" />
    </svg>
  );
}
