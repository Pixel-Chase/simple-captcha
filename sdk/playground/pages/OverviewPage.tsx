import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import type { ReactNode } from 'react';
import { adminApi, formatDateTime, getErrorMessage, isAdminApiError } from '../api';
import type {
  AppItem,
  BackgroundItem,
  BackgroundListResponse,
  RiskTelemetryItem,
  RuntimeSettings,
  UploadConstraints,
} from '../api';
import { Icon } from '../components/Icon';
import type { IconName } from '../components/Icon';
import type { ConsoleSection } from '../console/ConsoleView';
import './OverviewPage.css';

/** 「最近验证情况」统计的风控观察记录条数（接口上限 500） */
const TELEMETRY_LIMIT = 200;
/** 失败原因只展示出现次数最多的几项 */
const TOP_REASONS = 5;
/** 基础难度的等级数（策略配置里为 1-5） */
const DIFFICULTY_LEVELS = [1, 2, 3, 4, 5] as const;

interface OverviewPageProps {
  token: string;
  onSessionExpired: () => void;
  hrefFor: (section: ConsoleSection) => string;
}

/** 单个区块的加载结果：某个接口失败只影响对应区块 */
interface Block<T> {
  data: T | null;
  error: string | null;
}

interface OverviewData {
  apps: Block<AppItem[]>;
  backgrounds: Block<BackgroundListResponse>;
  settings: Block<RuntimeSettings>;
  telemetry: Block<RiskTelemetryItem[]>;
}

const PENDING: Block<never> = { data: null, error: null };
const INITIAL_DATA: OverviewData = {
  apps: PENDING,
  backgrounds: PENDING,
  settings: PENDING,
  telemetry: PENDING,
};

function toBlock<T>(result: PromiseSettledResult<T>, fallback: string): Block<T> {
  return result.status === 'fulfilled'
    ? { data: result.value, error: null }
    : { data: null, error: getErrorMessage(result.reason, fallback) };
}

function mapBlock<T, R>(block: Block<T>, fn: (data: T) => R): Block<R> {
  return { data: block.data === null ? null : fn(block.data), error: block.error };
}

const isUnauthorized = (result: PromiseSettledResult<unknown>) =>
  result.status === 'rejected' && isAdminApiError(result.reason) && result.reason.status === 401;

/** 自建应用统计：内置系统应用（管理后台）不计入 */
function summarizeApps(apps: AppItem[]) {
  const own = apps.filter((app) => !app.isSystem);
  const enabled = own.filter((app) => app.enabled).length;
  return {
    total: own.length,
    enabled,
    disabled: own.length - enabled,
    noOrigin: own.filter((app) => app.origins.length === 0).length,
    neverUsed: own.filter((app) => app.lastUsedAt === null).length,
  };
}

/** 与背景图页的清晰度标签一致：达到建议尺寸为高清，达到下限为达标，否则出图需要放大 */
function gradeOf(item: BackgroundItem, constraints: UploadConstraints): 'hd' | 'ok' | 'low' {
  if (item.width >= constraints.recommendedWidth && item.height >= constraints.recommendedHeight) {
    return 'hd';
  }
  if (item.width >= constraints.minWidth && item.height >= constraints.minHeight) {
    return 'ok';
  }
  return 'low';
}

function summarizeBackgrounds({ items, constraints }: BackgroundListResponse) {
  const counts = { hd: 0, ok: 0, low: 0 };
  for (const item of items) {
    counts[gradeOf(item, constraints)] += 1;
  }
  return { total: items.length, ...counts, constraints };
}

/** 原因形如 webdriver(+60)，去掉括号里的分值只留原因码 */
const reasonCode = (reason: unknown) =>
  typeof reason === 'string' ? reason.replace(/\(.*\)\s*$/, '').trim() : '';

function summarizeTelemetry(items: RiskTelemetryItem[]) {
  let issued = 0;
  let verified = 0;
  let passed = 0;
  let failedWithoutReason = 0;
  let earliest: number | null = null;
  const reasonCounts = new Map<string, number>();

  for (const item of items) {
    const time = item.time ? Date.parse(item.time) : Number.NaN;
    if (!Number.isNaN(time) && (earliest === null || time < earliest)) {
      earliest = time;
    }
    if (item.kind === 'issue') {
      issued += 1;
      continue;
    }
    verified += 1;
    if (item.success === true) {
      passed += 1;
      continue;
    }
    // 同一条记录里的原因码只计一次，条形长度即「带有该原因的失败校验占比」
    const codes = new Set((Array.isArray(item.reasons) ? item.reasons : []).map(reasonCode));
    codes.delete('');
    if (codes.size === 0) {
      failedWithoutReason += 1;
    }
    for (const code of codes) {
      reasonCounts.set(code, (reasonCounts.get(code) ?? 0) + 1);
    }
  }

  const topReasons = [...reasonCounts.entries()]
    .sort((a, b) => b[1] - a[1] || a[0].localeCompare(b[0]))
    .slice(0, TOP_REASONS)
    .map(([code, count]) => ({ code, count }));

  return {
    total: items.length,
    issued,
    verified,
    passed,
    failed: verified - passed,
    failedWithoutReason,
    topReasons,
    earliest: earliest === null ? null : new Date(earliest).toISOString(),
  };
}

const formatPercent = (ratio: number) => `${(ratio * 100).toFixed(1).replace(/\.0$/, '')}%`;

function powText(pow: RuntimeSettings['pow']) {
  if (!pow.enabled) {
    return '未下发';
  }
  return pow.required ? '已下发，强制提交' : '已下发，未强制';
}

/** 接入指引：步骤说明以 README 的接入与二次验证（S2S）章节为准 */
const GUIDE_STEPS: ReadonlyArray<{
  icon: IconName;
  title: string;
  desc: string;
  tag: string;
  mono?: boolean;
}> = [
  {
    icon: 'grid',
    title: '创建应用',
    desc: '在「接入应用」新建应用，得到公开的应用 ID 与保密的应用密钥，密钥明文只显示一次。',
    tag: '应用 ID · 应用密钥',
  },
  {
    icon: 'link',
    title: '填写允许来源',
    desc: '填写前端页面的来源（协议 + 域名 + 端口），只做精确匹配；没有来源的应用，前端出题与提交一律被拒绝。',
    tag: 'https://www.example.com',
    mono: true,
  },
  {
    icon: 'code',
    title: '前端传入应用 ID',
    desc: '在 CaptchaProvider 的 api.appId 传入应用 ID；验证通过后，把 second_key 随表单提交到业务后端。',
    tag: 'api.appId',
    mono: true,
  },
  {
    icon: 'shield-check',
    title: '业务后端二次校验',
    desc: '业务后端在请求头带上 Authorization: Bearer <应用密钥> 查询 second_key，返回 valid: true 才放行，每个 key 只能查询一次。',
    tag: 'GET /api/v1/captchas/states/:key',
    mono: true,
  },
];

/** 概览：登录后的默认首页，汇总各接口数据，一眼看出服务是否配置妥当与最近验证情况 */
export function OverviewPage({ token, onSessionExpired, hrefFor }: OverviewPageProps) {
  const [data, setData] = useState<OverviewData>(INITIAL_DATA);
  const [loading, setLoading] = useState(true);
  const [loadedAt, setLoadedAt] = useState<Date | null>(null);
  /** 只采用最后一次加载的结果：连续刷新或卸载后，过期的响应直接丢弃 */
  const requestSeq = useRef(0);

  const load = useCallback(async () => {
    requestSeq.current += 1;
    const seq = requestSeq.current;
    setLoading(true);

    const results = await Promise.allSettled([
      adminApi.listApps(token).then((res) => res.items),
      adminApi.listBackgrounds(token),
      adminApi.getSettings(token).then((res) => res.settings),
      adminApi.riskTelemetry(token, TELEMETRY_LIMIT).then((res) => res.items),
    ]);
    if (seq !== requestSeq.current) {
      return;
    }
    if (results.some(isUnauthorized)) {
      onSessionExpired();
      return;
    }

    const [apps, backgrounds, settings, telemetry] = results;
    setData({
      apps: toBlock(apps, '加载接入应用失败'),
      backgrounds: toBlock(backgrounds, '加载背景图失败'),
      settings: toBlock(settings, '加载策略配置失败'),
      telemetry: toBlock(telemetry, '加载风控观察记录失败'),
    });
    setLoadedAt(new Date());
    setLoading(false);
  }, [token, onSessionExpired]);

  useEffect(() => {
    void load();
    return () => {
      requestSeq.current += 1;
    };
  }, [load]);

  const appStats = useMemo(() => mapBlock(data.apps, summarizeApps), [data.apps]);
  const imageStats = useMemo(
    () => mapBlock(data.backgrounds, summarizeBackgrounds),
    [data.backgrounds],
  );
  const telemetryStats = useMemo(
    () => mapBlock(data.telemetry, summarizeTelemetry),
    [data.telemetry],
  );

  const settings = data.settings.data;
  /** 风控关闭或保留条数为 0 时，出题与校验都不写入风控观察 */
  const notRecording = settings
    ? !settings.risk.enabled || settings.risk.telemetryRetention === 0
    : false;

  const reminders = useMemo(() => {
    const stats = appStats.data;
    if (!stats) {
      return [];
    }
    const list: Array<Omit<ReminderProps, 'href'> & { id: string }> = [];
    if (stats.total === 0) {
      list.push({
        id: 'none',
        tone: 'warn',
        title: '还没有自建应用',
        desc: '前端出题需要应用 ID，业务后端二次校验需要应用密钥，请先创建一个接入应用。',
        action: '前往「接入应用」创建',
      });
    }
    if (stats.noOrigin > 0) {
      list.push({
        id: 'origin',
        tone: 'warn',
        title: '有应用未配置允许来源',
        desc: `${stats.noOrigin} 个应用没有允许来源，来自前端的出题与提交会被一律拒绝。`,
        action: '前往「接入应用」配置',
      });
    }
    if (stats.neverUsed > 0) {
      list.push({
        id: 'unused',
        tone: 'info',
        title: '有应用从未被使用',
        desc: `${stats.neverUsed} 个应用的密钥还没有被业务后端用来查询过二次校验结果；若已上线，请确认业务后端完成了接入指引第 4 步。`,
        action: '前往「接入应用」查看',
      });
    }
    return list;
  }, [appStats.data]);

  return (
    <div className="page overview">
      <header className="overview__head">
        <div className="overview__head-text">
          <div className="overview__title-row">
            <h1 className="overview__title">系统概览</h1>
            <span className="chip">
              <Icon name="clock" size={12} />
              {loadedAt ? `更新于 ${formatDateTime(loadedAt.toISOString())}` : '正在加载…'}
            </span>
          </div>
          <p className="overview__desc">
            服务配置与最近验证情况速览，数据汇总自接入应用、策略配置、背景图与风控观察。
          </p>
        </div>
        <div className="overview__head-actions">
          <button
            type="button"
            className="btn overview__refresh"
            onClick={() => void load()}
            disabled={loading}
          >
            <Icon name="refresh" size={16} className={loading ? 'spin' : undefined} />
            {loading ? '刷新中…' : '刷新数据'}
          </button>
          <a className="btn btn--primary" href={hrefFor('settings')}>
            <Icon name="sliders" size={16} />
            策略配置
          </a>
        </div>
      </header>

      {reminders.length > 0 && (
        <section className="overview__reminders" aria-label="接入提醒">
          {reminders.map(({ id, ...reminder }) => (
            <Reminder key={id} {...reminder} href={hrefFor('apps')} />
          ))}
        </section>
      )}

      <section className="overview__panel overview-guide">
        <div className="overview-guide__head">
          <div>
            <h2 className="overview-guide__title">接入指引</h2>
            <p className="overview-guide__desc">从创建应用到业务后端二次校验的完整接入路径</p>
          </div>
          <span className="overview-guide__badge">四步完成接入</span>
        </div>
        <ol className="overview-guide__steps">
          {GUIDE_STEPS.map((step, index) => (
            <li key={step.title} className="overview-step">
              <div className="overview-step__body">
                <div className="overview-step__top">
                  <span className="overview-step__num">{index + 1}</span>
                  <Icon name={step.icon} size={18} className="overview-step__icon" />
                </div>
                <h3 className="overview-step__title">{step.title}</h3>
                <p className="overview-step__desc">{step.desc}</p>
              </div>
              <span className={step.mono ? 'overview-step__tag mono' : 'overview-step__tag'}>
                {step.tag}
              </span>
            </li>
          ))}
        </ol>
      </section>

      <section className="overview__cards" aria-label="服务状态">
        <StatusCard
          icon="layers"
          title="接入应用"
          tag={appStats.data && <AppsTag stats={appStats.data} />}
          href={hrefFor('apps')}
          action="管理接入应用"
        >
          <BlockBody block={appStats}>
            {(stats) => (
              <>
                <div>
                  <div className="overview-card__metric">
                    <span className="overview-card__value">{stats.total}</span>
                    <span className="overview-card__unit">个自建应用</span>
                  </div>
                  <p className="overview-card__note">内置「管理后台」系统应用不计入统计</p>
                </div>
                <dl className="overview-card__rows">
                  <CardRow label="已启用">{stats.enabled} 个</CardRow>
                  <CardRow label="已停用">{stats.disabled} 个</CardRow>
                  <CardRow label="未配置来源" warn={stats.noOrigin > 0}>
                    {stats.noOrigin > 0 ? `${stats.noOrigin} 个` : '无'}
                  </CardRow>
                  <CardRow label="从未使用">
                    {stats.neverUsed > 0 ? `${stats.neverUsed} 个` : '无'}
                  </CardRow>
                </dl>
              </>
            )}
          </BlockBody>
        </StatusCard>

        <StatusCard
          icon="shield-check"
          title="当前策略"
          tag={settings && <RiskTag risk={settings.risk} />}
          href={hrefFor('settings')}
          action="修改策略配置"
        >
          <BlockBody block={data.settings}>
            {({ captcha, pow, risk }) => (
              <>
                <dl className="overview-card__rows overview-card__rows--plain">
                  <CardRow label="基础难度">
                    <span
                      className="overview-level"
                      title={`基础难度 ${captcha.defaultDifficulty} / 5`}
                    >
                      {DIFFICULTY_LEVELS.map((level) => (
                        <span
                          key={level}
                          className={
                            level <= captcha.defaultDifficulty
                              ? 'overview-level__dot overview-level__dot--on'
                              : 'overview-level__dot'
                          }
                        />
                      ))}
                    </span>
                    {captcha.defaultDifficulty} 级
                  </CardRow>
                  <CardRow label="工作量证明 (PoW)">{powText(pow)}</CardRow>
                  <CardRow label="风控评分">
                    <span
                      className={risk.enabled ? 'overview-dot overview-dot--on' : 'overview-dot'}
                    />
                    {risk.enabled ? '已启用' : '已关闭'}
                  </CardRow>
                  <CardRow label="风控观察保留">
                    {risk.telemetryRetention > 0 ? `${risk.telemetryRetention} 条` : '不记录'}
                  </CardRow>
                </dl>
                <p className="overview-card__hint">
                  {!risk.enabled ? (
                    '风控评分已关闭：校验时不做行为与环境评分，风控观察也不再记录。'
                  ) : risk.mode === 'log' ? (
                    <>
                      运行模式为 <strong>log</strong>：仅记录分数，不会拦截。
                    </>
                  ) : (
                    <>
                      运行模式为 <strong>enforce</strong>：风险分达到拦截阈值 {risk.blockThreshold}{' '}
                      且满足行为证据要求时，本次校验判为失败。
                    </>
                  )}
                </p>
              </>
            )}
          </BlockBody>
        </StatusCard>

        <StatusCard
          icon="image"
          title="背景图"
          tag={
            imageStats.data && (
              <span className="chip">
                下限 {imageStats.data.constraints.minWidth}×{imageStats.data.constraints.minHeight}
              </span>
            )
          }
          href={hrefFor('images')}
          action="查看背景图"
        >
          <BlockBody block={imageStats}>
            {(stats) => (
              <>
                <div>
                  <div className="overview-card__metric">
                    <span className="overview-card__value">{stats.total}</span>
                    <span className="overview-card__unit">张背景图</span>
                  </div>
                  <p className="overview-card__note">
                    所有题型共用，建议 {stats.constraints.recommendedWidth}×
                    {stats.constraints.recommendedHeight}（3:2）及以上
                  </p>
                </div>
                <dl className="overview-card__rows">
                  <CardRow label="高清">{stats.hd} 张</CardRow>
                  <CardRow label="达标">{stats.ok} 张</CardRow>
                  <CardRow label="低于下限" warn={stats.low > 0}>
                    {stats.low > 0 ? `${stats.low} 张` : '无'}
                  </CardRow>
                </dl>
                {stats.low > 0 && (
                  <div className="overview-card__alert">
                    <div className="overview-card__alert-title">
                      <Icon name="warning" size={16} />
                      {stats.low} 张低于清晰度下限
                    </div>
                    <p>
                      出图时需要放大，建议替换为 {stats.constraints.recommendedWidth}×
                      {stats.constraints.recommendedHeight} 以上的图片。
                    </p>
                  </div>
                )}
              </>
            )}
          </BlockBody>
        </StatusCard>
      </section>

      <section className="overview__panel overview-recent">
        <div className="overview-recent__head">
          <div className="overview-recent__title-row">
            <h2 className="overview-recent__title">最近验证情况</h2>
            {telemetryStats.data && telemetryStats.data.total > 0 && (
              <span className="overview-recent__count">
                最近 {telemetryStats.data.total} 条记录
              </span>
            )}
          </div>
          <a className="overview__link" href={hrefFor('risk')}>
            查看风控观察
            <Icon name="arrow-right" size={15} />
          </a>
        </div>

        {settings && notRecording && (
          <div className="overview-recent__off">
            <Icon name="info" size={18} className="overview-recent__off-icon" />
            <div className="overview-recent__off-text">
              <div className="overview-recent__off-title">未记录</div>
              <p>
                {!settings.risk.enabled
                  ? '风控评分已关闭，出题与校验不会写入风控观察，这里不会有新数据。'
                  : '「风控观察保留条数」为 0，出题与校验不会写入风控观察，这里不会有新数据。'}
              </p>
            </div>
            <a className="btn btn--sm" href={hrefFor('settings')}>
              前往策略配置
            </a>
          </div>
        )}

        <BlockBody block={telemetryStats}>
          {(stats) =>
            stats.total === 0 ? (
              notRecording ? null : (
                <div className="overview-recent__empty">
                  <Icon name="activity" size={22} />
                  <p className="overview-recent__empty-title">还没有验证记录</p>
                  <p>出题与校验发生后会自动写入风控观察，届时这里会给出统计。</p>
                </div>
              )
            ) : (
              <RecentStats stats={stats} />
            )
          }
        </BlockBody>
      </section>
    </div>
  );
}

type AppStats = ReturnType<typeof summarizeApps>;
type TelemetryStats = ReturnType<typeof summarizeTelemetry>;

/** 区块的加载中 / 失败 / 正常三种状态 */
function BlockBody<T>({ block, children }: { block: Block<T>; children: (data: T) => ReactNode }) {
  if (block.error) {
    return (
      <div className="alert alert--error" role="alert">
        <Icon name="error" size={16} />
        <span>{block.error}</span>
      </div>
    );
  }
  if (block.data === null) {
    return <p className="overview__placeholder">正在加载…</p>;
  }
  return <>{children(block.data)}</>;
}

interface ReminderProps {
  tone: 'warn' | 'info';
  title: string;
  desc: string;
  action: string;
  href: string;
}

function Reminder({ tone, title, desc, action, href }: ReminderProps) {
  return (
    <div className={`overview-reminder overview-reminder--${tone}`}>
      <div className="overview-reminder__main">
        <span className="overview-reminder__icon">
          <Icon name={tone === 'warn' ? 'warning' : 'info'} size={18} />
        </span>
        <div>
          <div className="overview-reminder__title">{title}</div>
          <div className="overview-reminder__desc">{desc}</div>
        </div>
      </div>
      <a className="overview__link overview-reminder__link" href={href}>
        {action}
        <Icon name="arrow-right" size={15} />
      </a>
    </div>
  );
}

function StatusCard({
  icon,
  title,
  tag,
  href,
  action,
  children,
}: {
  icon: IconName;
  title: string;
  tag?: ReactNode;
  href: string;
  action: string;
  children: ReactNode;
}) {
  return (
    <article className="overview-card">
      <div className="overview-card__body">
        <div className="overview-card__head">
          <h3 className="overview-card__title">
            <Icon name={icon} size={20} className="overview-card__icon" />
            {title}
          </h3>
          {tag}
        </div>
        {children}
      </div>
      <a className="overview-card__action" href={href}>
        {action}
        <Icon name="arrow-right" size={16} />
      </a>
    </article>
  );
}

function CardRow({
  label,
  warn = false,
  children,
}: {
  label: string;
  warn?: boolean;
  children: ReactNode;
}) {
  return (
    <div className="overview-card__row">
      <dt>{label}</dt>
      <dd className={warn ? 'overview-card__row-value--warn' : undefined}>
        {warn && <Icon name="error" size={14} />}
        {children}
      </dd>
    </div>
  );
}

/** 接入应用卡片右上角：按应用配置给出状态 */
function AppsTag({ stats }: { stats: AppStats }) {
  if (stats.total === 0) {
    return <span className="chip chip--warn">未创建</span>;
  }
  if (stats.noOrigin > 0) {
    return <span className="chip chip--warn">待配置来源</span>;
  }
  if (stats.enabled === 0) {
    return <span className="chip">全部已停用</span>;
  }
  return <span className="chip chip--good">来源已配置</span>;
}

function RiskTag({ risk }: { risk: RuntimeSettings['risk'] }) {
  if (!risk.enabled) {
    return <span className="chip">风控已关闭</span>;
  }
  return risk.mode === 'log' ? (
    <span className="chip chip--warn">log 模式</span>
  ) : (
    <span className="chip chip--primary">enforce 模式</span>
  );
}

function RecentStats({ stats }: { stats: TelemetryStats }) {
  const { issued, verified, passed, failed, topReasons, failedWithoutReason } = stats;

  return (
    <>
      <div className="overview-stats">
        <div className="overview-stat">
          <div className="overview-stat__label">出题次数</div>
          <div className="overview-stat__value">{issued}</div>
          <div className="overview-stat__hint">前端发起的出题</div>
        </div>
        <div className="overview-stat">
          <div className="overview-stat__label">校验次数</div>
          <div className="overview-stat__value">{verified}</div>
          <div className="overview-stat__hint">提交答案的次数</div>
        </div>
        <div className="overview-stat">
          <div className="overview-stat__label">校验通过</div>
          <div className="overview-stat__value">{passed}</div>
          <div className="overview-stat__hint overview-stat__hint--primary">已签发二次校验 key</div>
        </div>
        <div className={failed > 0 ? 'overview-stat overview-stat--danger' : 'overview-stat'}>
          <div className="overview-stat__label">校验失败</div>
          <div className="overview-stat__value">{failed}</div>
          <div className="overview-stat__hint">含答案错误与风控拦截</div>
        </div>
        <div className="overview-stat overview-stat--rate">
          <div className="overview-stat__label">通过率（通过 / 校验）</div>
          <div className="overview-stat__value">
            {verified > 0 ? formatPercent(passed / verified) : '—'}
          </div>
          <div className="overview-stat__hint">
            {verified > 0 ? `${passed} / ${verified}` : '暂无校验记录'}
          </div>
        </div>
      </div>

      <div className="overview-reasons">
        <div className="overview-reasons__head">
          <h3 className="overview-reasons__title">
            <Icon name="list-check" size={18} />
            失败原因分布（出现次数前 {TOP_REASONS}）
          </h3>
          {topReasons.length > 0 && <span className="overview-reasons__sort">按出现次数排序</span>}
        </div>

        {failed === 0 ? (
          <p className="overview__placeholder">最近的记录里没有失败的校验。</p>
        ) : topReasons.length === 0 ? (
          <p className="overview__placeholder">失败的校验都没有附带风控原因。</p>
        ) : (
          <ol className="overview-reasons__list">
            {topReasons.map(({ code, count }, index) => {
              const share = count / failed;
              return (
                <li key={code} className="overview-reason">
                  <span
                    className={
                      index === 0
                        ? 'overview-reason__rank overview-reason__rank--first'
                        : 'overview-reason__rank'
                    }
                  >
                    {index + 1}
                  </span>
                  <span className="overview-reason__code mono" title={code}>
                    {code}
                  </span>
                  <span
                    className="overview-reason__meter"
                    title={`占失败校验的 ${formatPercent(share)}`}
                  >
                    <span
                      className={`overview-reason__fill overview-reason__fill--${Math.min(index + 1, 3)}`}
                      style={{ width: `${share * 100}%` }}
                    />
                  </span>
                  <span className="overview-reason__count">{count} 次</span>
                </li>
              );
            })}
          </ol>
        )}

        {failedWithoutReason > 0 && topReasons.length > 0 && (
          <p className="overview-reasons__foot">
            另有 {failedWithoutReason} 次失败校验没有附带风控原因。
          </p>
        )}
      </div>

      <p className="overview-recent__foot">
        以上统计来自最近 {stats.total} 条风控观察记录
        {stats.earliest ? `（最早一条 ${formatDateTime(stats.earliest)}）` : ''}
        ，出题与校验合计，管理后台自身的登录验证也计入其中。
      </p>
    </>
  );
}
