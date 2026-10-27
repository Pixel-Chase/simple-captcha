import { useCallback, useEffect, useMemo, useState } from 'react';
import { adminApi, formatDateTime, getErrorMessage, isAdminApiError } from '../api';
import type { RiskTelemetryItem } from '../api';

interface RiskPageProps {
  token: string;
  onSessionExpired: () => void;
}

const LIMIT_OPTIONS = [50, 100, 200] as const;
type LimitOption = (typeof LIMIT_OPTIONS)[number];

/** 出题评估（有 level）还是校验评分；早期记录没有 kind，按校验记录展示 */
const kindOf = (item: RiskTelemetryItem): 'issue' | 'verify' =>
  item.kind === 'issue' || (item.kind === undefined && typeof item.level === 'number')
    ? 'issue'
    : 'verify';

const formatTime = (value?: string) => {
  if (!value) {
    return '—';
  }
  try {
    return formatDateTime(value);
  } catch {
    return value;
  }
};

/** 风控观察：最近的出题评估与校验评分，只读，用于调权重时对照原因 */
export function RiskPage({ token, onSessionExpired }: RiskPageProps) {
  const [items, setItems] = useState<RiskTelemetryItem[]>([]);
  const [limit, setLimit] = useState<LimitOption>(100);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [loadedAt, setLoadedAt] = useState<Date | null>(null);

  const load = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const data = await adminApi.riskTelemetry(token, limit);
      setItems(data.items);
      setLoadedAt(new Date());
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
      const base = `${item.key ?? 'row'}-${kindOf(item)}-${item.time ?? ''}`;
      const count = (seen.get(base) ?? 0) + 1;
      seen.set(base, count);
      return { id: count === 1 ? base : `${base}#${count}`, item };
    });
  }, [items]);

  return (
    <section className="risk">
      <div className="risk__head">
        <div>
          <p className="eyebrow">风控</p>
          <h2>风控观察</h2>
          <p className="muted">
            最近的出题评估（难度来源）与校验评分（分数与原因），IP 与 UA 已脱敏。调整 RISK_*
            权重前先在这里对照真人与脚本的原因分布。
          </p>
        </div>
        <div className="risk__controls">
          <label className="risk__limit">
            条数
            <select
              value={limit}
              onChange={(event) => setLimit(Number(event.target.value) as LimitOption)}
              disabled={loading}
            >
              {LIMIT_OPTIONS.map((option) => (
                <option key={option} value={option}>
                  {option}
                </option>
              ))}
            </select>
          </label>
          <button
            type="button"
            className="btn btn--ghost"
            onClick={() => void load()}
            disabled={loading}
          >
            {loading ? '加载中…' : '刷新'}
          </button>
        </div>
      </div>

      {error && (
        <div className="alert alert--error" role="alert">
          {error}
        </div>
      )}

      {loading && items.length === 0 ? (
        <div className="page-status">正在加载…</div>
      ) : items.length === 0 ? (
        <div className="card risk__empty">
          <p>还没有遥测记录。</p>
          <p className="muted">
            出题与校验发生后会自动写入；RISK_ENABLED=false 或 RISK_TELEMETRY_LOG_SIZE=0 时不记录。
          </p>
        </div>
      ) : (
        <div className="card risk__table-wrap">
          <table className="risk-table">
            <thead>
              <tr>
                <th className="risk-table__cell risk-table__head">时间</th>
                <th className="risk-table__cell risk-table__head">阶段</th>
                <th className="risk-table__cell risk-table__head">题型</th>
                <th className="risk-table__cell risk-table__head">难度</th>
                <th className="risk-table__cell risk-table__head">分数</th>
                <th className="risk-table__cell risk-table__head">结果</th>
                <th className="risk-table__cell risk-table__head">原因</th>
                <th className="risk-table__cell risk-table__head">客户端</th>
              </tr>
            </thead>
            <tbody>
              {rows.map(({ id, item }) => {
                const kind = kindOf(item);
                const reasons = Array.isArray(item.reasons) ? item.reasons : [];
                return (
                  <tr key={id}>
                    <td className="risk-table__cell risk-table__time">{formatTime(item.time)}</td>
                    <td className="risk-table__cell">
                      <span className={kind === 'issue' ? 'badge badge--ok' : 'badge'}>
                        {kind === 'issue' ? '出题' : '校验'}
                      </span>
                    </td>
                    <td className="risk-table__cell">{item.type ?? '—'}</td>
                    <td className="risk-table__cell">
                      {kind === 'issue' && typeof item.level === 'number' ? (
                        <span
                          title={`默认 ${item.base ?? '—'} + 环境加难 ${item.boost ?? 0}，环境分 ${item.envScore ?? 0}${item.consistent ? '，请求头一致' : ''}`}
                        >
                          {item.level}
                        </span>
                      ) : (
                        '—'
                      )}
                    </td>
                    <td className="risk-table__cell">
                      {kind === 'issue'
                        ? typeof item.envScore === 'number'
                          ? `环境 ${item.envScore}`
                          : '—'
                        : typeof item.score === 'number'
                          ? item.score
                          : '—'}
                    </td>
                    <td className="risk-table__cell">
                      {kind === 'verify' ? (
                        <span className={item.success ? 'badge badge--good' : 'badge badge--warn'}>
                          {item.success ? '通过' : '失败'}
                        </span>
                      ) : (
                        '—'
                      )}
                    </td>
                    <td className="risk-table__cell risk-table__reasons">
                      {reasons.length === 0 ? (
                        <span className="muted">无</span>
                      ) : (
                        <ul>
                          {reasons.map((reason) => (
                            <li key={reason} className="risk-table__reason">
                              {reason}
                            </li>
                          ))}
                        </ul>
                      )}
                      {item.tls && <div className="muted risk-table__fp">tls {item.tls}</div>}
                    </td>
                    <td className="risk-table__cell risk-table__client">
                      <div>{item.client ?? '—'}</div>
                      <div className="muted">
                        {[item.ip, item.ua].filter(Boolean).join(' · ') || '—'}
                      </div>
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}

      {loadedAt && (
        <p className="muted risk__footer">
          共 {items.length} 条 · 更新于 {formatDateTime(loadedAt.toISOString())}
        </p>
      )}
    </section>
  );
}
