import type { RiskTelemetryItem } from '../api';

export interface TimelineBucket {
  key: string;
  label: string;
  fullLabel: string;
  issued: number;
  verified: number;
  passed: number;
  failed: number;
  passRate: number | null;
}

export interface TypeBreakdownItem {
  type: string;
  label: string;
  issued: number;
  verified: number;
  passed: number;
  total: number;
  share: number;
}

export interface ScoreHistogramBucket {
  index: number;
  label: string;
  min: number;
  max: number;
  pass: number;
  fail: number;
  total: number;
}

const TYPE_LABELS: Record<string, string> = {
  'click-text': '点选文字',
  'click-shape': '点选图形',
  'slide-text': '滑动文字',
  'slide-region': '滑动拼图',
  rotate: '旋转图片',
};

export function labelForCaptchaType(type: string): string {
  return TYPE_LABELS[type] ?? type;
}

function pad2(n: number): string {
  return String(n).padStart(2, '0');
}

function formatShortClock(ms: number, includeDate: boolean): string {
  const date = new Date(ms);
  const hhmm = `${pad2(date.getHours())}:${pad2(date.getMinutes())}`;
  if (!includeDate) {
    return hhmm;
  }
  return `${pad2(date.getMonth() + 1)}-${pad2(date.getDate())} ${hhmm}`;
}

/**
 * 将遥测记录按时间顺序等分分桶，用于概览页时间趋势图与 KPI 迷你折线。
 * 当记录时间跨度很短或只有一条有效时间戳时，仍返回稳定分桶以保持图表结构对齐。
 */
export function bucketTelemetryTimeline(
  items: ReadonlyArray<RiskTelemetryItem>,
  bucketCount = 12,
): TimelineBucket[] {
  const count = Math.max(2, Math.min(24, Math.floor(bucketCount)));
  const stamped: Array<{ ts: number; item: RiskTelemetryItem }> = [];

  for (const item of items) {
    const ts = item.time ? Date.parse(item.time) : Number.NaN;
    if (Number.isFinite(ts)) {
      stamped.push({ ts, item });
    }
  }

  if (stamped.length === 0) {
    return [];
  }

  stamped.sort((a, b) => a.ts - b.ts);
  const minTs = stamped[0].ts;
  const maxTs = stamped[stamped.length - 1].ts;
  const span = Math.max(1, maxTs - minTs);
  const includeDate = span >= 18 * 60 * 60 * 1000;
  const step = span / count;

  const buckets: TimelineBucket[] = Array.from({ length: count }, (_, index) => {
    const startMs = Math.round(minTs + index * step);
    const endMs = Math.round(minTs + (index + 1) * step);
    const label = formatShortClock(startMs, includeDate);
    const endLabel = formatShortClock(endMs, includeDate);
    return {
      key: `${index}-${startMs}`,
      label,
      fullLabel: startMs === endMs ? label : `${label} ~ ${endLabel}`,
      issued: 0,
      verified: 0,
      passed: 0,
      failed: 0,
      passRate: null,
    };
  });

  for (const { ts, item } of stamped) {
    const rawIdx = maxTs === minTs ? count - 1 : Math.floor(((ts - minTs) / span) * count);
    const idx = Math.min(count - 1, Math.max(0, rawIdx));
    const bucket = buckets[idx];
    if (item.kind === 'issue') {
      bucket.issued += 1;
    } else {
      bucket.verified += 1;
      if (item.success === true) {
        bucket.passed += 1;
      } else if (item.success === false) {
        bucket.failed += 1;
      }
    }
  }

  for (const bucket of buckets) {
    if (bucket.verified > 0) {
      bucket.passRate = Math.round((bucket.passed / bucket.verified) * 1000) / 10;
    }
  }

  return buckets;
}

/** 统计各验证码题型在遥测记录中的出题与校验占比 */
export function summarizeCaptchaTypes(
  items: ReadonlyArray<RiskTelemetryItem>,
): TypeBreakdownItem[] {
  const map = new Map<string, { issued: number; verified: number; passed: number }>();

  for (const item of items) {
    const type = item.type?.trim() || 'unknown';
    const current = map.get(type) ?? { issued: 0, verified: 0, passed: 0 };
    if (item.kind === 'issue') {
      current.issued += 1;
    } else {
      current.verified += 1;
      if (item.success === true) {
        current.passed += 1;
      }
    }
    map.set(type, current);
  }

  const grandTotal = items.length || 1;
  return [...map.entries()]
    .map(([type, counts]) => {
      const total = counts.issued + counts.verified;
      return {
        type,
        label: labelForCaptchaType(type),
        issued: counts.issued,
        verified: counts.verified,
        passed: counts.passed,
        total,
        share: total / grandTotal,
      };
    })
    .sort((a, b) => b.total - a.total || a.type.localeCompare(b.type));
}

/** 构建 0–100 分的 10 档风险分分布直方图（仅统计带有数值分数的校验记录） */
export function buildScoreHistogram(
  items: ReadonlyArray<RiskTelemetryItem>,
): ScoreHistogramBucket[] {
  const buckets: ScoreHistogramBucket[] = Array.from({ length: 10 }, (_, index) => {
    const min = index * 10;
    const max = index === 9 ? 100 : min + 9;
    return {
      index,
      label: `${min}-${max}`,
      min,
      max,
      pass: 0,
      fail: 0,
      total: 0,
    };
  });

  for (const item of items) {
    if (item.kind !== 'verify' || typeof item.score !== 'number' || !Number.isFinite(item.score)) {
      continue;
    }
    const clamped = Math.min(100, Math.max(0, Math.round(item.score)));
    const idx = clamped === 100 ? 9 : Math.floor(clamped / 10);
    const bucket = buckets[idx];
    bucket.total += 1;
    if (item.success === true) {
      bucket.pass += 1;
    } else {
      bucket.fail += 1;
    }
  }

  return buckets;
}

/** 风控观察表格关键字匹配：支持 key / ip / ua / 应用名或 ID / 题型 / 原因码 */
export function matchesRiskQuery(
  item: RiskTelemetryItem,
  rawQuery: string,
  appNames: Record<string, string>,
): boolean {
  const q = rawQuery.trim().toLowerCase();
  if (!q) {
    return true;
  }
  const appName = appNames[item.app] ?? '';
  const reasons = Array.isArray(item.reasons) ? item.reasons.join(' ') : '';
  const anomalies = Array.isArray(item.anomalies) ? item.anomalies.join(' ') : '';
  const haystack = [item.key, item.ip, item.ua, item.app, appName, item.type, reasons, anomalies]
    .filter((v): v is string => typeof v === 'string' && v.length > 0)
    .join('\n')
    .toLowerCase();

  return haystack.includes(q);
}
