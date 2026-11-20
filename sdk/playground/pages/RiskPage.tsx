import { useCallback, useEffect, useMemo, useState } from 'react';
import type { ReactNode } from 'react';
import { adminApi, formatDateTime, getErrorMessage, isAdminApiError } from '../api';
import type { RiskTelemetryItem } from '../api';
import { Icon } from '../components/Icon';
import type { IconName } from '../components/Icon';
import { Modal } from '../components/Modal';
import { useCopy } from '../components/Toast';
import type { ConsoleSection } from '../console/ConsoleView';
import { buildScoreHistogram, matchesRiskQuery } from '../utils/analytics';
import type { ScoreHistogramBucket } from '../utils/analytics';
import './RiskPage.css';

interface RiskPageProps {
  token: string;
  onSessionExpired: () => void;
  hrefFor: (section: ConsoleSection) => string;
}

const LIMIT_OPTIONS = [50, 100, 200] as const;
type LimitOption = (typeof LIMIT_OPTIONS)[number];

/** 每页行数：筛选与分页都只在已加载的记录内进行 */
const PAGE_SIZE = 20;
/** 表格里每行直接展示的原因数，其余在右侧抽屉里查看 */
const MAX_INLINE_REASONS = 4;
/** 没有值的字段统一显示的占位 */
const EMPTY = '—';
const SKELETON_ROWS = ['r1', 'r2', 'r3', 'r4', 'r5', 'r6'] as const;

type Kind = 'issue' | 'verify';
type StageFilter = 'all' | Kind;
type ResultFilter = 'all' | 'pass' | 'fail';

interface Filters {
  stage: StageFilter;
  result: ResultFilter;
  /** 应用 ID；ALL 表示全部 */
  app: string;
  type: string;
  query: string;
  /** 点击分数直方图桶按 0-9 .. 90-100 过滤；null 表示不限分数段 */
  scoreBucket: number | null;
}

const ALL = '';
const DEFAULT_FILTERS: Filters = {
  stage: 'all',
  result: 'all',
  app: ALL,
  type: ALL,
  query: '',
  scoreBucket: null,
};

const STAGE_OPTIONS: ReadonlyArray<{ value: StageFilter; label: string }> = [
  { value: 'all', label: '全部阶段' },
  { value: 'issue', label: '出题评估' },
  { value: 'verify', label: '校验评分' },
];

const RESULT_OPTIONS: ReadonlyArray<{ value: ResultFilter; label: string }> = [
  { value: 'all', label: '全部结果' },
  { value: 'pass', label: '通过' },
  { value: 'fail', label: '失败' },
];

/** 已在三组字段里展示过的键；其余键（mode、features、webdriver 等）列在「其他字段」 */
const LISTED_KEYS = new Set([
  'kind',
  'time',
  'app',
  'type',
  'key',
  'client',
  'ip',
  'ua',
  'tls',
  'h2',
  'fp',
  'consistent',
  'score',
  'behaviorScore',
  'envScore',
  'level',
  'base',
  'boost',
  'success',
  'reasons',
  'anomalies',
]);

/** 已知额外字段的说明（对照 backend/services/risk_engine.go 的写入逻辑），未知字段只显示原始键名 */
const EXTRA_LABELS: Record<string, string> = {
  mode: '风控运行模式',
  hardMarker: '命中自动化硬标记',
  requestReasons: '请求头核验原因',
  features: '轨迹特征',
  pointerType: '指针类型',
  untrusted: '脚本合成事件数',
  webdriver: 'navigator.webdriver',
  automation: '自动化框架痕迹',
};

const parseTime = (value?: string) => {
  if (!value) {
    return null;
  }
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? null : date;
};

/** 表格里只显示时分秒，完整时间放在 title 里 */
const formatClock = (date: Date) => date.toLocaleTimeString('zh-CN', { hour12: false });

const formatTime = (value?: string) => {
  if (!value) {
    return EMPTY;
  }
  try {
    return formatDateTime(value);
  } catch {
    return value;
  }
};

const isNumber = (value: unknown): value is number =>
  typeof value === 'number' && Number.isFinite(value);

/** 轨迹特征里的小数保留三位，整数原样显示；复制 JSON 时仍是原始值 */
const formatNumber = (value: number) =>
  Number.isInteger(value) ? String(value) : String(Math.round(value * 1000) / 1000);

const isPlainObject = (value: unknown): value is Record<string, unknown> =>
  typeof value === 'object' && value !== null && !Array.isArray(value);

const stringList = (value: unknown): string[] =>
  Array.isArray(value) ? value.filter((entry): entry is string => typeof entry === 'string') : [];

/**
 * 难度构成：默认难度 + 环境加难。客户端历史与出口 IP 带来的加难没有单独字段，
 * 用最终难度的差值补齐（明细见原因）；最终难度被封顶时差值为负
 */
function difficultyText(item: RiskTelemetryItem): string | null {
  if (!isNumber(item.level) || !isNumber(item.base)) {
    return null;
  }
  const boost = isNumber(item.boost) ? item.boost : 0;
  const parts = [`默认 ${item.base}`, `环境加难 ${boost}`];
  const other = item.level - item.base - boost;
  if (other > 0) {
    parts.push(`历史 / IP 加难 ${other}`);
  }
  return other < 0 ? `${parts.join(' + ')}，已到上限` : parts.join(' + ');
}

/** consistent：请求头齐全且与自报环境一致；没有请求侧信号时也为 false */
const consistencyText = (value: unknown) =>
  value === true ? '请求头一致' : value === false ? '请求头不一致或缺失' : null;

const resultText = (value: unknown) => (value === true ? '通过' : value === false ? '失败' : null);

interface AppLabel {
  name: string | null;
  id: string;
  deleted: boolean;
}

/** 应用名称映射；记录里的应用已不在列表中说明已被删除，只能显示 ID */
function appLabelOf(app: string, appNames: Record<string, string>): AppLabel {
  const name = appNames[app];
  return { name: name ?? null, id: app, deleted: name === undefined };
}

const appOptionLabel = (key: string, appNames: Record<string, string>) => {
  const name = appNames[key];
  return name ? `${name}（${key}）` : `${key}（已删除）`;
};

/** 页码条：首末页、当前页前后各一页，靠近两端时多露出几页，其余折叠成省略号 */
function pageItems(current: number, total: number): Array<number | 'gap-start' | 'gap-end'> {
  if (total <= 7) {
    return Array.from({ length: total }, (_, index) => index + 1);
  }
  let start = Math.max(2, current - 1);
  let end = Math.min(total - 1, current + 1);
  if (current <= 3) {
    start = 2;
    end = 4;
  } else if (current >= total - 2) {
    start = total - 3;
    end = total - 1;
  }
  const items: Array<number | 'gap-start' | 'gap-end'> = [1];
  if (start > 2) {
    items.push('gap-start');
  }
  for (let page = start; page <= end; page += 1) {
    items.push(page);
  }
  if (end < total - 1) {
    items.push('gap-end');
  }
  items.push(total);
  return items;
}

interface SegmentedProps<T extends string | number> {
  label: string;
  value: T;
  options: ReadonlyArray<{ value: T; label: string }>;
  onChange: (value: T) => void;
  tone: 'primary' | 'dark' | 'soft';
  prefix?: string;
  disabled?: boolean;
  title?: string;
}

/** 分段选择：条数、阶段、结果共用 */
function Segmented<T extends string | number>({
  label,
  value,
  options,
  onChange,
  tone,
  prefix,
  disabled,
  title,
}: SegmentedProps<T>) {
  return (
    <fieldset
      className={`risk-seg risk-seg--${tone}`}
      aria-label={label}
      title={title}
      disabled={disabled}
    >
      {prefix && <span className="risk-seg__prefix">{prefix}</span>}
      {options.map((option) => {
        const active = option.value === value;
        return (
          <button
            key={String(option.value)}
            type="button"
            className={active ? 'risk-seg__btn risk-seg__btn--active' : 'risk-seg__btn'}
            aria-pressed={active}
            onClick={() => onChange(option.value)}
          >
            {option.label}
          </button>
        );
      })}
    </fieldset>
  );
}

function StageChip({ item }: { item: RiskTelemetryItem }) {
  let tone = 'chip--primary';
  let icon: IconName = 'sliders';
  let label = '出题评估';
  if (item.kind === 'verify') {
    if (item.success === true) {
      tone = 'chip--good';
      icon = 'check-circle';
      label = '校验通过';
    } else if (item.success === false) {
      tone = 'chip--danger';
      icon = 'x-circle';
      label = '校验失败';
    } else {
      tone = '';
      icon = 'shield';
      label = '校验评分';
    }
  }
  return (
    <span className={`chip ${tone} risk-stage`}>
      <Icon name={icon} size={12} />
      {label}
    </span>
  );
}

function AppCell({ label }: { label: AppLabel }) {
  if (label.deleted) {
    return (
      <div className="risk-app risk-app--deleted">
        <span className="risk-app__name mono">{label.id}</span>
        <span className="risk-app__id">应用已删除</span>
      </div>
    );
  }
  return (
    <div className="risk-app">
      <span className="risk-app__name">{label.name}</span>
      <span className="risk-app__id mono">{label.id}</span>
    </div>
  );
}

/** 核心打分：出题看最终难度与环境分，校验看风险分、行为分与结果 */
function ScoreCell({ item, kind }: { item: RiskTelemetryItem; kind: Kind }) {
  if (kind === 'issue') {
    const composition = difficultyText(item);
    const consistency = consistencyText(item.consistent);
    return (
      <div className="risk-score">
        <div className="risk-score__main">
          最终难度{' '}
          <span className="risk-score__num risk-score__num--primary">
            {isNumber(item.level) ? `${item.level} 级` : EMPTY}
          </span>
          {composition && <span className="risk-score__sub">（{composition}）</span>}
        </div>
        <div className="risk-score__note">
          环境分{' '}
          <span className="risk-score__plain">
            {isNumber(item.envScore) ? item.envScore : EMPTY}
          </span>
          {consistency && ` · ${consistency}`}
        </div>
      </div>
    );
  }

  const result = resultText(item.success);
  const tone =
    item.success === true
      ? ' risk-score__num--good'
      : item.success === false
        ? ' risk-score__num--bad'
        : '';
  return (
    <div className="risk-score">
      <div className="risk-score__main">
        风险分{' '}
        <span className={`risk-score__num${tone}`}>
          {isNumber(item.score) ? item.score : EMPTY}
        </span>
        <span className="risk-score__sep">/</span>
        行为分{' '}
        <span className="risk-score__plain">
          {isNumber(item.behaviorScore) ? item.behaviorScore : EMPTY}
        </span>
      </div>
      <div className="risk-score__note">
        结果{' '}
        <span
          className={
            item.success === true
              ? 'risk-score__result risk-score__result--pass'
              : item.success === false
                ? 'risk-score__result risk-score__result--fail'
                : 'risk-score__result'
          }
        >
          {result ?? EMPTY}
        </span>
        {item.mode === 'log' && ' · log 模式不拦截'}
      </div>
    </div>
  );
}

function ReasonsCell({ reasons, tone }: { reasons: string[]; tone: string }) {
  if (reasons.length === 0) {
    return (
      <span className="risk-none">
        <Icon name="check" size={12} className="risk-none__icon" />无
      </span>
    );
  }
  const shown = reasons.slice(0, MAX_INLINE_REASONS);
  const rest = reasons.length - shown.length;
  return (
    <div className="risk-reasons">
      {shown.map((reason, index) => (
        // 原因可能重复（同名规则多次命中），用位置补齐 key
        // biome-ignore lint/suspicious/noArrayIndexKey: 原因列表只读且不重排
        <span key={`${reason}-${index}`} className={`risk-reason risk-reason--${tone}`}>
          {reason}
        </span>
      ))}
      {rest > 0 && <span className="risk-reasons__more">+{rest} 项</span>}
    </div>
  );
}

/** 原因形如 name(+N)：拆出名称与权重；出题的 client_history / ip_pressure / env_boost 权重是级数，其余是分值 */
const REASON_PATTERN = /^(.*?)\(([+-]?\d+(?:\.\d+)?)\)$/;

function ReasonLines({ reasons, tone }: { reasons: string[]; tone: string }) {
  if (reasons.length === 0) {
    return <span className="risk-field__empty">{EMPTY}</span>;
  }
  return (
    <ul className="risk-lines">
      {reasons.map((reason, index) => {
        const match = REASON_PATTERN.exec(reason);
        return (
          // biome-ignore lint/suspicious/noArrayIndexKey: 原因列表只读且不重排
          <li key={`${reason}-${index}`} className="risk-lines__item">
            <span className={`risk-lines__name risk-lines__name--${tone}`}>
              {match ? match[1] : reason}
            </span>
            {match && <span className="risk-lines__weight">{match[2]}</span>}
          </li>
        );
      })}
    </ul>
  );
}

function TagList({ values }: { values: string[] }) {
  if (values.length === 0) {
    return <span className="risk-field__empty">{EMPTY}</span>;
  }
  return (
    <span className="risk-tags">
      {values.map((value, index) => (
        // biome-ignore lint/suspicious/noArrayIndexKey: 只读列表且不重排
        <span key={`${value}-${index}`} className="risk-reason risk-reason--neutral">
          {value}
        </span>
      ))}
    </span>
  );
}

/** 额外字段的通用展示：布尔显示是 / 否，字符串数组显示标签，对象展开成键值网格 */
function ExtraValue({ value }: { value: unknown }) {
  if (value === null || value === undefined || value === '') {
    return <span className="risk-field__empty">{EMPTY}</span>;
  }
  if (typeof value === 'boolean') {
    return <>{value ? '是' : '否'}</>;
  }
  if (typeof value === 'number') {
    return <span className="risk-num">{formatNumber(value)}</span>;
  }
  if (typeof value === 'string') {
    return <span className="mono">{value}</span>;
  }
  if (Array.isArray(value)) {
    if (value.every((entry) => typeof entry === 'string')) {
      return <TagList values={value as string[]} />;
    }
    return <span className="mono risk-json">{JSON.stringify(value)}</span>;
  }
  if (isPlainObject(value)) {
    const entries = Object.entries(value);
    if (entries.length === 0) {
      return <span className="risk-field__empty">{EMPTY}</span>;
    }
    return (
      <span className="risk-kv">
        {entries.map(([key, entry]) => (
          <span key={key} className="risk-kv__item">
            <span className="risk-kv__key">{key}</span>
            <span className="risk-kv__value">
              {isNumber(entry)
                ? formatNumber(entry)
                : typeof entry === 'string' || typeof entry === 'boolean'
                  ? String(entry)
                  : JSON.stringify(entry)}
            </span>
          </span>
        ))}
      </span>
    );
  }
  return <>{String(value)}</>;
}

interface FieldProps {
  label: string;
  hint?: string;
  block?: boolean;
  mono?: boolean;
  children?: ReactNode;
}

/** 完整字段里的一行：没有值时显示占位 */
function Field({ label, hint, block, mono, children }: FieldProps) {
  const empty = children === null || children === undefined || children === '';
  const valueClass = [
    'risk-field__value',
    mono && !empty ? 'mono' : '',
    empty ? 'risk-field__empty' : '',
  ]
    .filter(Boolean)
    .join(' ');
  return (
    <div className={block ? 'risk-field risk-field--block' : 'risk-field'}>
      <dt className="risk-field__label">
        {label}
        {hint && <span className="risk-field__hint">{hint}</span>}
      </dt>
      <dd className={valueClass}>{empty ? EMPTY : children}</dd>
    </div>
  );
}

/** 10 档风险分直方图（0–100）：支持点击分桶按区间筛选 */
function ScoreHistogram({
  buckets,
  selectedBucket,
  onSelectBucket,
}: {
  buckets: ScoreHistogramBucket[];
  selectedBucket: number | null;
  onSelectBucket: (bucketIndex: number | null) => void;
}) {
  const [hovered, setHovered] = useState<number | null>(null);
  const totalVerify = buckets.reduce((sum, b) => sum + b.total, 0);
  const maxTotal = Math.max(1, ...buckets.map((b) => b.total));
  const activeBucket =
    hovered !== null ? buckets[hovered] : selectedBucket !== null ? buckets[selectedBucket] : null;

  const width = 620;
  const height = 108;
  const padLeft = 0;
  const padRight = 0;
  const padTop = 8;
  const padBottom = 22;
  const innerW = width - padLeft - padRight;
  const innerH = height - padTop - padBottom;
  const slotW = innerW / buckets.length;
  const barW = Math.min(34, slotW * 0.62);

  return (
    <div className="risk-histogram">
      <div className="risk-histogram__head">
        <div className="risk-histogram__title-wrap">
          <span className="risk-histogram__title">
            <Icon name="chart" size={15} />
            风险分分布（0–100）
          </span>
          <span className="risk-histogram__meta tabular">
            {totalVerify === 0
              ? '当前范围暂无校验打分记录'
              : activeBucket
                ? `${activeBucket.label} 分：共 ${activeBucket.total} 条（通过 ${activeBucket.pass} · 失败 ${activeBucket.fail}）`
                : '点击柱条可按分数段过滤列表'}
          </span>
        </div>
        <div className="risk-histogram__right">
          <span className="risk-histogram__legend" aria-hidden="true">
            <span className="risk-histogram__legend-item">
              <span className="risk-histogram__swatch risk-histogram__swatch--pass" />
              通过
            </span>
            <span className="risk-histogram__legend-item">
              <span className="risk-histogram__swatch risk-histogram__swatch--fail" />
              失败
            </span>
          </span>
          {selectedBucket !== null && (
            <button
              type="button"
              className="btn btn--text risk-histogram__clear"
              onClick={() => onSelectBucket(null)}
            >
              取消分段（{buckets[selectedBucket]?.label}）
            </button>
          )}
        </div>
      </div>

      {totalVerify > 0 && (
        <div className="risk-histogram__stage">
          <svg
            className="risk-histogram__svg"
            viewBox={`0 0 ${width} ${height}`}
            aria-hidden="true"
          >
            <line
              x1={padLeft}
              y1={padTop + innerH}
              x2={width - padRight}
              y2={padTop + innerH}
              className="risk-histogram__baseline"
            />
            {buckets.map((bucket) => {
              const slotX = padLeft + bucket.index * slotW;
              const barX = slotX + (slotW - barW) / 2;
              const totalH =
                bucket.total === 0 ? 0 : Math.max(3, (bucket.total / maxTotal) * innerH);
              const failH = bucket.fail === 0 ? 0 : (bucket.fail / bucket.total) * totalH;
              const passH = Math.max(0, totalH - failH);
              const selected = selectedBucket === bucket.index;
              const isHovered = hovered === bucket.index;
              const dimmed = selectedBucket !== null && !selected;
              const colClass = [
                'risk-histogram__col',
                selected ? 'risk-histogram__col--selected' : '',
                isHovered ? 'risk-histogram__col--hovered' : '',
                dimmed ? 'risk-histogram__col--dimmed' : '',
              ]
                .filter(Boolean)
                .join(' ');
              return (
                <g key={bucket.label} className={colClass}>
                  <rect
                    x={slotX + 2}
                    y={padTop}
                    width={Math.max(4, slotW - 4)}
                    height={innerH}
                    rx={4}
                    className="risk-histogram__hit"
                  />
                  {passH > 0 && (
                    <rect
                      x={barX}
                      y={padTop + innerH - totalH}
                      width={barW}
                      height={passH}
                      rx={2}
                      className="risk-histogram__pass"
                    />
                  )}
                  {failH > 0 && (
                    <rect
                      x={barX}
                      y={padTop + innerH - failH}
                      width={barW}
                      height={failH}
                      rx={2}
                      className="risk-histogram__fail"
                    />
                  )}
                  <text
                    x={slotX + slotW / 2}
                    y={height - 5}
                    textAnchor="middle"
                    className="risk-histogram__label"
                  >
                    {bucket.label}
                  </text>
                </g>
              );
            })}
          </svg>
          <fieldset className="risk-histogram__buttons" aria-label="按风险分数段筛选">
            {buckets.map((bucket) => {
              const selected = selectedBucket === bucket.index;
              return (
                <button
                  key={bucket.label}
                  type="button"
                  className="risk-histogram__btn"
                  aria-pressed={selected}
                  aria-label={`${bucket.label} 分：共 ${bucket.total} 条，通过 ${bucket.pass}，失败 ${bucket.fail}`}
                  onClick={() => onSelectBucket(selected ? null : bucket.index)}
                  onMouseEnter={() => setHovered(bucket.index)}
                  onMouseLeave={() => setHovered(null)}
                  onFocus={() => setHovered(bucket.index)}
                  onBlur={() => setHovered(null)}
                />
              );
            })}
          </fieldset>
        </div>
      )}
    </div>
  );
}

/** 右侧抽屉内容：按基础 / 网络与指纹 / 评分三组列出全部字段 */
function RecordDetail({
  item,
  kind,
  app,
  reasonTone,
}: {
  item: RiskTelemetryItem;
  kind: Kind;
  app: AppLabel;
  reasonTone: string;
}) {
  const copy = useCopy();
  const reasons = stringList(item.reasons);
  const anomalies = stringList(item.anomalies);
  const extras = Object.keys(item)
    .filter((key) => !LISTED_KEYS.has(key))
    .sort((a, b) => Number(isPlainObject(item[a])) - Number(isPlainObject(item[b])));
  const consistency = consistencyText(item.consistent);
  const difficulty = difficultyText(item);
  const result = resultText(item.success);

  return (
    <div className="risk-detail">
      <div className="risk-detail__top">
        <span className="risk-detail__hint">按 ↑ / ↓ 方向键可快速切换相邻记录</span>
        <button
          type="button"
          className="btn btn--sm"
          onClick={() => void copy(JSON.stringify(item, null, 2), '已复制整条记录 JSON')}
        >
          <Icon name="copy" size={14} />
          复制整条 JSON
        </button>
      </div>

      <section className="risk-group">
        <h3 className="risk-group__title">基础字段</h3>
        <dl className="risk-group__list">
          <Field label="阶段">{kind === 'issue' ? '出题评估' : '校验评分'}</Field>
          <Field label="时间">
            {item.time ? (
              <span className="tabular" title={item.time}>
                {formatTime(item.time)}
              </span>
            ) : null}
          </Field>
          <Field label="所属应用">
            {app.id ? (
              <span>
                {app.deleted ? '应用已删除' : app.name}{' '}
                <span className="mono risk-field__aside">{app.id}</span>
              </span>
            ) : null}
          </Field>
          <Field label="题目 key" mono>
            {item.key}
          </Field>
          <Field label="题型" mono>
            {item.type}
          </Field>
          <Field label="客户端标识" hint="IP 与 UA 的哈希前 8 位" mono>
            {item.client}
          </Field>
        </dl>
      </section>

      <section className="risk-group">
        <h3 className="risk-group__title">网络与环境指纹</h3>
        <dl className="risk-group__list">
          <Field label="IP" hint="已脱敏" mono>
            {item.ip}
          </Field>
          <Field label="UA" hint="浏览器 / 系统摘要">
            {item.ua}
          </Field>
          <Field label="TLS 指纹" hint="由可信指纹代理注入" mono>
            {item.tls}
          </Field>
          <Field label="HTTP/2 指纹" hint="由可信指纹代理注入" mono>
            {item.h2}
          </Field>
          <Field label="请求头一致性">{consistency}</Field>
          <Field label="环境指纹" hint="SDK 上报的环境哈希" mono>
            {typeof item.fp === 'string' ? item.fp : null}
          </Field>
        </dl>
      </section>

      <section className="risk-group">
        <h3 className="risk-group__title">评分细节</h3>
        <dl className="risk-group__list">
          <Field label="风险分" hint="0–100，越高越像脚本">
            {isNumber(item.score) ? <span className="risk-num">{item.score} / 100</span> : null}
          </Field>
          <Field label="行为分" hint="越高越像脚本，人类特征可为负">
            {isNumber(item.behaviorScore) ? (
              <span className="risk-num">{item.behaviorScore}</span>
            ) : null}
          </Field>
          <Field label="环境分" hint="已按上限裁剪">
            {isNumber(item.envScore) ? <span className="risk-num">{item.envScore}</span> : null}
          </Field>
          <Field label="难度构成">
            {difficulty ? `最终 ${item.level} 级 = ${difficulty}` : null}
          </Field>
          <Field label="结果">
            {result ? (
              <span
                className={
                  item.success ? 'risk-result risk-result--pass' : 'risk-result risk-result--fail'
                }
              >
                {result}
              </span>
            ) : null}
          </Field>
          <Field label="原因" block>
            <ReasonLines reasons={reasons} tone={reasonTone} />
          </Field>
          <Field label="异常项" hint="SDK 检测到的无头浏览器 / API 篡改痕迹" block>
            <TagList values={anomalies} />
          </Field>
        </dl>
      </section>

      {extras.length > 0 && (
        <section className="risk-group">
          <h3 className="risk-group__title">其他字段</h3>
          <dl className="risk-group__list">
            {extras.map((key) => (
              <Field
                key={key}
                label={EXTRA_LABELS[key] ?? key}
                hint={EXTRA_LABELS[key] ? key : undefined}
                block={isPlainObject(item[key])}
              >
                <ExtraValue value={item[key]} />
              </Field>
            ))}
          </dl>
        </section>
      )}
    </div>
  );
}

function Pager({
  page,
  pageCount,
  total,
  onChange,
}: {
  page: number;
  pageCount: number;
  total: number;
  onChange: (page: number) => void;
}) {
  const from = total === 0 ? 0 : (page - 1) * PAGE_SIZE + 1;
  const to = Math.min(page * PAGE_SIZE, total);
  return (
    <div className="risk-pager tabular">
      <span>
        显示 {from} 至 {to} 条，共 {total} 条
      </span>
      {pageCount > 1 && (
        <nav className="risk-pager__pages" aria-label="分页">
          <button
            type="button"
            className="risk-pager__btn"
            disabled={page <= 1}
            onClick={() => onChange(page - 1)}
          >
            上一页
          </button>
          {pageItems(page, pageCount).map((entry) =>
            typeof entry === 'number' ? (
              <button
                key={entry}
                type="button"
                className={
                  entry === page ? 'risk-pager__btn risk-pager__btn--current' : 'risk-pager__btn'
                }
                aria-current={entry === page ? 'page' : undefined}
                onClick={() => onChange(entry)}
              >
                {entry}
              </button>
            ) : (
              <span key={entry} className="risk-pager__gap">
                …
              </span>
            ),
          )}
          <button
            type="button"
            className="risk-pager__btn"
            disabled={page >= pageCount}
            onClick={() => onChange(page + 1)}
          >
            下一页
          </button>
        </nav>
      )}
    </div>
  );
}

/** 风控观察：最近的出题评估与校验评分，只读，用于调权重时对照原因 */
export function RiskPage({ token, onSessionExpired, hrefFor }: RiskPageProps) {
  const [items, setItems] = useState<RiskTelemetryItem[]>([]);
  const [limit, setLimit] = useState<LimitOption>(100);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [loadedAt, setLoadedAt] = useState<Date | null>(null);
  /** 应用 ID → 名称，用于"所属应用"列；应用已删除时显示 ID */
  const [appNames, setAppNames] = useState<Record<string, string>>({});
  const [filters, setFilters] = useState<Filters>(DEFAULT_FILTERS);
  const [page, setPage] = useState(1);
  const [selectedId, setSelectedId] = useState<string | null>(null);

  const load = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const [data, apps] = await Promise.all([
        adminApi.riskTelemetry(token, limit),
        adminApi.listApps(token),
      ]);
      setItems(data.items);
      setAppNames(Object.fromEntries(apps.items.map((app) => [app.id, app.name])));
      setLoadedAt(new Date());
      setPage(1);
    } catch (err) {
      if (isAdminApiError(err) && err.status === 401) {
        onSessionExpired();
        return;
      }
      setError(getErrorMessage(err, '加载遥测记录失败'));
    } finally {
      setLoading(false);
    }
  }, [token, limit, onSessionExpired]);

  useEffect(() => {
    void load();
  }, [load]);

  // 同一题目可能有多条校验记录，用出现次数而不是数组下标补齐稀疏的行 key
  const rows = useMemo(() => {
    const seen = new Map<string, number>();
    return items.map((item) => {
      const kind = item.kind;
      const base = `${item.key ?? 'row'}-${kind}-${item.time ?? ''}`;
      const count = (seen.get(base) ?? 0) + 1;
      seen.set(base, count);
      return { id: count === 1 ? base : `${base}#${count}`, item, kind };
    });
  }, [items]);

  // 筛选选项来自已加载的记录；当前选中的值即使刷新后不在记录里也保留，避免下拉框与筛选状态不一致
  const appOptions = useMemo(() => {
    const keys = new Set(rows.map(({ item }) => item.app));
    if (filters.app !== ALL) {
      keys.add(filters.app);
    }
    return [...keys]
      .map((key) => ({ value: key, label: appOptionLabel(key, appNames) }))
      .sort((a, b) => a.label.localeCompare(b.label, 'zh-CN'));
  }, [rows, appNames, filters.app]);

  const typeOptions = useMemo(() => {
    const types = new Set(
      rows.map(({ item }) => item.type).filter((type): type is string => Boolean(type)),
    );
    if (filters.type !== ALL) {
      types.add(filters.type);
    }
    return [...types].sort();
  }, [rows, filters.type]);

  // 应用与题型圈定统计范围；阶段、结果、关键词与分数段过滤表格记录
  const scoped = useMemo(
    () =>
      rows.filter(
        ({ item }) =>
          (filters.app === ALL || item.app === filters.app) &&
          (filters.type === ALL || item.type === filters.type),
      ),
    [rows, filters.app, filters.type],
  );

  const histogram = useMemo(() => buildScoreHistogram(scoped.map(({ item }) => item)), [scoped]);

  const filtered = useMemo(() => {
    const activeBucket =
      filters.scoreBucket !== null ? (histogram[filters.scoreBucket] ?? null) : null;
    return scoped.filter(({ item, kind }) => {
      if (filters.stage !== 'all' && kind !== filters.stage) {
        return false;
      }
      if (filters.result === 'pass' && !(kind === 'verify' && item.success === true)) {
        return false;
      }
      if (filters.result === 'fail' && !(kind === 'verify' && item.success === false)) {
        return false;
      }
      if (activeBucket) {
        if (kind !== 'verify' || !isNumber(item.score)) {
          return false;
        }
        const clamped = Math.min(100, Math.max(0, Math.round(item.score)));
        if (clamped < activeBucket.min || clamped > activeBucket.max) {
          return false;
        }
      }
      return matchesRiskQuery(item, filters.query, appNames);
    });
  }, [
    scoped,
    histogram,
    filters.stage,
    filters.result,
    filters.scoreBucket,
    filters.query,
    appNames,
  ]);

  const summary = useMemo(() => {
    let issue = 0;
    let verify = 0;
    let pass = 0;
    let fail = 0;
    for (const { item, kind } of scoped) {
      if (kind === 'issue') {
        issue += 1;
        continue;
      }
      verify += 1;
      if (item.success === true) {
        pass += 1;
      } else if (item.success === false) {
        fail += 1;
      }
    }
    return { issue, verify, pass, fail };
  }, [scoped]);

  const judged = summary.pass + summary.fail;
  const failRate = judged === 0 ? EMPTY : `${((summary.fail / judged) * 100).toFixed(1)}%`;

  const pageCount = Math.max(1, Math.ceil(filtered.length / PAGE_SIZE));
  const currentPage = Math.min(page, pageCount);
  const pageRows = filtered.slice((currentPage - 1) * PAGE_SIZE, currentPage * PAGE_SIZE);
  const filtersActive =
    filters.stage !== 'all' ||
    filters.result !== 'all' ||
    filters.app !== ALL ||
    filters.type !== ALL ||
    filters.query.trim() !== '' ||
    filters.scoreBucket !== null;

  const selectedIndex = useMemo(
    () => (selectedId ? filtered.findIndex((row) => row.id === selectedId) : -1),
    [filtered, selectedId],
  );
  const selectedRow = selectedIndex >= 0 ? filtered[selectedIndex] : null;

  useEffect(() => {
    if (!selectedRow) {
      return;
    }
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key !== 'ArrowUp' && event.key !== 'ArrowDown') {
        return;
      }
      const target = event.target as HTMLElement | null;
      if (target && (target.tagName === 'INPUT' || target.tagName === 'TEXTAREA')) {
        return;
      }
      event.preventDefault();
      const nextIdx =
        event.key === 'ArrowUp'
          ? Math.max(0, selectedIndex - 1)
          : Math.min(filtered.length - 1, selectedIndex + 1);
      const next = filtered[nextIdx];
      if (next) {
        setSelectedId(next.id);
      }
    };
    window.addEventListener('keydown', onKeyDown);
    return () => window.removeEventListener('keydown', onKeyDown);
  }, [selectedRow, selectedIndex, filtered]);

  const updateFilters = (patch: Partial<Filters>) => {
    setFilters((prev) => ({ ...prev, ...patch }));
    setPage(1);
  };

  const clearFilters = () => {
    setFilters(DEFAULT_FILTERS);
    setPage(1);
  };

  const settingsHref = hrefFor('settings');

  return (
    <div className="page risk">
      <header className="page-head risk__head">
        <div className="risk__intro">
          <div className="page-head__title">
            <h1 className="page-title">风控观察</h1>
          </div>
          <p className="page-desc">
            只读查看最近的出题评估与校验评分，调整策略前对照真人与脚本的原因分布；IP 与 UA 已脱敏。
          </p>
        </div>
        <div className="page-head__actions risk__actions">
          <Segmented
            label="读取条数"
            prefix="读取条数"
            tone="soft"
            value={limit}
            options={LIMIT_OPTIONS.map((option) => ({ value: option, label: String(option) }))}
            onChange={setLimit}
            disabled={loading}
          />
          <button
            type="button"
            className="btn risk__refresh"
            onClick={() => void load()}
            disabled={loading}
          >
            <Icon name="refresh" size={16} className={loading ? 'spin' : undefined} />
            {loading ? '加载中…' : '刷新'}
          </button>
          {loadedAt && (
            <span className="risk__meta tabular" title={formatDateTime(loadedAt.toISOString())}>
              共 {items.length} 条 · 更新于 {formatClock(loadedAt)}
            </span>
          )}
        </div>
      </header>

      {error && (
        <div className="alert alert--error risk__alert" role="alert">
          <Icon name="error" size={16} />
          <span className="risk__alert-text">{error}</span>
          <button
            type="button"
            className="btn btn--text btn--sm"
            onClick={() => void load()}
            disabled={loading}
          >
            重试
          </button>
        </div>
      )}

      {loading && items.length === 0 ? (
        <section className="risk-skeleton" aria-busy="true" aria-label="正在加载风控记录">
          <span className="skeleton risk-skeleton__filter" />
          <div className="risk-board">
            {SKELETON_ROWS.map((key) => (
              <div key={key} className="risk-skeleton__row">
                <span className="skeleton risk-skeleton__cell risk-skeleton__cell--sm" />
                <span className="skeleton risk-skeleton__cell risk-skeleton__cell--md" />
                <span className="skeleton risk-skeleton__cell risk-skeleton__cell--lg" />
                <span className="skeleton risk-skeleton__cell risk-skeleton__cell--sm" />
              </div>
            ))}
          </div>
        </section>
      ) : items.length === 0 ? (
        !error && (
          <div className="risk-board">
            <div className="risk-empty">
              <span className="risk-empty__icon">
                <Icon name="activity" size={22} />
              </span>
              <p className="risk-empty__title">还没有风控观察记录</p>
              <p className="risk-empty__desc">
                出题与校验发生后会自动写入。「风控评分」关闭或「风控观察保留条数」为 0
                时不记录，可在<a href={settingsHref}>策略配置</a>中检查。
              </p>
            </div>
          </div>
        )
      ) : (
        <>
          <section className="risk-filter" aria-label="筛选与分布">
            <div className="risk-filter__row">
              <div className="risk-filter__controls">
                <Segmented
                  label="阶段"
                  tone="primary"
                  value={filters.stage}
                  options={STAGE_OPTIONS}
                  onChange={(stage) =>
                    updateFilters(
                      stage === 'issue' ? { stage, result: 'all', scoreBucket: null } : { stage },
                    )
                  }
                />
                <Segmented
                  label="结果"
                  tone="dark"
                  value={filters.result}
                  options={RESULT_OPTIONS}
                  onChange={(result) => updateFilters({ result })}
                  disabled={filters.stage === 'issue'}
                  title={filters.stage === 'issue' ? '出题评估没有通过 / 失败结果' : undefined}
                />
                <span className="risk-select">
                  <select
                    aria-label="所属应用"
                    value={filters.app}
                    onChange={(event) => updateFilters({ app: event.target.value })}
                  >
                    <option value={ALL}>全部应用（{appOptions.length} 个）</option>
                    {appOptions.map((option) => (
                      <option key={option.value} value={option.value}>
                        {option.label}
                      </option>
                    ))}
                  </select>
                  <Icon name="chevron-down" size={14} className="risk-select__icon" />
                </span>
                <span className="risk-select risk-select--mono">
                  <select
                    aria-label="题型"
                    value={filters.type}
                    onChange={(event) => updateFilters({ type: event.target.value })}
                  >
                    <option value={ALL}>全部题型</option>
                    {typeOptions.map((type) => (
                      <option key={type} value={type}>
                        {type}
                      </option>
                    ))}
                  </select>
                  <Icon name="chevron-down" size={14} className="risk-select__icon" />
                </span>
                <label className="risk-search">
                  <Icon name="search" size={15} className="risk-search__icon" />
                  <input
                    type="search"
                    className="risk-search__input"
                    placeholder="搜索 key / IP / UA / 原因码…"
                    value={filters.query}
                    onChange={(event) => updateFilters({ query: event.target.value })}
                    aria-label="关键字搜索风控记录"
                  />
                </label>
              </div>
              <div className="risk-filter__hit-wrap">
                <span className="risk-filter__hit tabular">
                  命中{' '}
                  <strong>
                    {filtered.length} / {items.length}
                  </strong>{' '}
                  条
                </span>
                {filtersActive && (
                  <button
                    type="button"
                    className="btn btn--text risk-filter__reset"
                    onClick={clearFilters}
                  >
                    重置筛选
                  </button>
                )}
              </div>
            </div>

            <div
              className="risk-summary"
              title="按所选应用与题型统计，不受阶段、结果与关键词筛选影响"
            >
              <span className="risk-summary__item">
                <span className="risk-summary__dot risk-summary__dot--issue" />
                <span className="risk-summary__label">出题评估</span>
                <span className="risk-summary__value">{summary.issue} 条</span>
              </span>
              <span className="risk-summary__item">
                <span className="risk-summary__dot risk-summary__dot--verify" />
                <span className="risk-summary__label">校验评分</span>
                <span className="risk-summary__value">{summary.verify} 条</span>
              </span>
              <span className="risk-summary__item">
                <span className="risk-summary__dot risk-summary__dot--pass" />
                <span className="risk-summary__label">通过 / 失败</span>
                <span className="risk-summary__value">
                  {summary.pass}
                  <span className="risk-summary__slash">/</span>
                  <span className={summary.fail > 0 ? 'risk-summary__fail' : undefined}>
                    {summary.fail}
                  </span>
                </span>
              </span>
              <span className="risk-summary__item risk-summary__item--end">
                <span className="risk-summary__label">校验失败率</span>
                <span
                  className={summary.fail > 0 ? 'risk-rate risk-rate--bad' : 'risk-rate'}
                  title="失败条数 / 校验评分条数"
                >
                  {failRate}
                </span>
              </span>
            </div>

            <ScoreHistogram
              buckets={histogram}
              selectedBucket={filters.scoreBucket}
              onSelectBucket={(scoreBucket) =>
                updateFilters(
                  scoreBucket !== null && filters.stage === 'issue'
                    ? { scoreBucket, stage: 'all' }
                    : { scoreBucket },
                )
              }
            />
          </section>

          <section className="risk__records">
            <div className="risk__section-head">
              <h2 className="risk__section-title">
                记录明细
                <span className="risk__section-sub tabular">（最近 {items.length} 条）</span>
              </h2>
              <span className="risk__hint">
                <Icon name="info" size={13} />
                点击任意记录行可在右侧抽屉查看完整字段与轨迹特征
              </span>
            </div>

            <div className={loading ? 'risk-board risk-board--loading' : 'risk-board'}>
              {filtered.length === 0 ? (
                <div className="risk-empty">
                  <span className="risk-empty__icon">
                    <Icon name="filter" size={22} />
                  </span>
                  <p className="risk-empty__title">没有匹配的记录</p>
                  <p className="risk-empty__desc">
                    筛选只在已加载的 {items.length} 条记录内进行
                    {items.length >= limit && limit < LIMIT_OPTIONS[LIMIT_OPTIONS.length - 1]
                      ? '，可以清除筛选或读取更多条数。'
                      : '，可以清除筛选后再看。'}
                  </p>
                  {filtersActive && (
                    <button
                      type="button"
                      className="btn btn--sm risk-empty__action"
                      onClick={clearFilters}
                    >
                      清除筛选
                    </button>
                  )}
                </div>
              ) : (
                <>
                  <div className="risk-board__scroll">
                    <table className="table risk-table">
                      <thead>
                        <tr>
                          <th>阶段 / 判定</th>
                          <th>时间</th>
                          <th>所属应用</th>
                          <th>题型</th>
                          <th>核心打分</th>
                          <th>原因</th>
                          <th>客户端摘要</th>
                          <th className="risk-table__action">详情</th>
                        </tr>
                      </thead>
                      <tbody>
                        {pageRows.map(({ id, item, kind }) => {
                          const isSelected = selectedId === id;
                          const time = parseTime(item.time);
                          const app = appLabelOf(item.app, appNames);
                          const reasons = stringList(item.reasons);
                          const reasonTone =
                            kind === 'issue'
                              ? 'issue'
                              : item.success === false
                                ? 'fail'
                                : 'neutral';
                          return (
                            <tr
                              key={id}
                              className={isSelected ? 'risk-row risk-row--selected' : 'risk-row'}
                              onClick={() => setSelectedId(id)}
                            >
                              <td>
                                <StageChip item={item} />
                              </td>
                              <td className="risk-row__time" title={formatTime(item.time)}>
                                {time ? formatClock(time) : (item.time ?? EMPTY)}
                              </td>
                              <td>
                                <AppCell label={app} />
                              </td>
                              <td>
                                {item.type ? (
                                  <span className="risk-type">{item.type}</span>
                                ) : (
                                  <span className="muted">{EMPTY}</span>
                                )}
                              </td>
                              <td>
                                <ScoreCell item={item} kind={kind} />
                              </td>
                              <td className="risk-row__reasons">
                                <ReasonsCell reasons={reasons} tone={reasonTone} />
                              </td>
                              <td>
                                <div className="risk-client">
                                  <span className="risk-client__ip">{item.ip || EMPTY}</span>
                                  <span className="risk-client__ua" title={item.ua}>
                                    {item.ua || EMPTY}
                                  </span>
                                </div>
                              </td>
                              <td className="risk-table__action">
                                <button
                                  type="button"
                                  className={
                                    isSelected
                                      ? 'btn btn--sm btn--primary risk-toggle'
                                      : 'btn btn--sm risk-toggle'
                                  }
                                  onClick={(event) => {
                                    event.stopPropagation();
                                    setSelectedId(id);
                                  }}
                                >
                                  查看详情
                                </button>
                              </td>
                            </tr>
                          );
                        })}
                      </tbody>
                    </table>
                  </div>
                  <Pager
                    page={currentPage}
                    pageCount={pageCount}
                    total={filtered.length}
                    onChange={setPage}
                  />
                </>
              )}
            </div>
          </section>
        </>
      )}

      {selectedRow && (
        <Modal
          open
          onClose={() => setSelectedId(null)}
          variant="drawer"
          drawerSize="lg"
          icon="braces"
          title={
            <span className="risk-drawer__title">
              <StageChip item={selectedRow.item} />
              <span>记录详情</span>
            </span>
          }
          subtitle={
            <span className="mono tabular">
              {selectedRow.item.key ?? '无题目 key'} ·{' '}
              {selectedRow.item.time ? formatTime(selectedRow.item.time) : '未知时间'}
            </span>
          }
          footer={
            <>
              <div className="risk-drawer__nav">
                <button
                  type="button"
                  className="btn btn--sm"
                  disabled={selectedIndex <= 0}
                  onClick={() => {
                    const prev = filtered[selectedIndex - 1];
                    if (prev) {
                      setSelectedId(prev.id);
                    }
                  }}
                >
                  <Icon name="chevron-up" size={14} />
                  上一条
                </button>
                <span className="risk-drawer__pos tabular">
                  {selectedIndex + 1} / {filtered.length}
                </span>
                <button
                  type="button"
                  className="btn btn--sm"
                  disabled={selectedIndex >= filtered.length - 1}
                  onClick={() => {
                    const next = filtered[selectedIndex + 1];
                    if (next) {
                      setSelectedId(next.id);
                    }
                  }}
                >
                  下一条
                  <Icon name="chevron-down" size={14} />
                </button>
              </div>
              <a className="btn btn--primary btn--sm" href={settingsHref}>
                前往策略配置
                <Icon name="arrow-right" size={14} />
              </a>
            </>
          }
        >
          <RecordDetail
            item={selectedRow.item}
            kind={selectedRow.kind}
            app={appLabelOf(selectedRow.item.app, appNames)}
            reasonTone={
              selectedRow.kind === 'issue'
                ? 'issue'
                : selectedRow.item.success === false
                  ? 'fail'
                  : 'neutral'
            }
          />
        </Modal>
      )}

      <aside className="risk-notice">
        <Icon name="info" size={18} className="risk-notice__icon" />
        <p className="risk-notice__text">
          <strong>说明：</strong>
          记录由服务端保存，按「风控观察保留条数」只保留最近的记录；「风控评分」关闭或保留条数为 0
          时不写入。本页一次读取最近 50 / 100 / 200 条，筛选与分页都在已读取的记录内进行。可在
          <a className="risk-notice__link" href={settingsHref}>
            「策略配置」
            <Icon name="arrow-right" size={12} />
          </a>
          中调整。
        </p>
      </aside>
    </div>
  );
}
