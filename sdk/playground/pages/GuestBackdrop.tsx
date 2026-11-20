import { useCallback, useEffect, useState, useSyncExternalStore } from 'react';
import { Icon } from '../components/Icon';
import './GuestPages.css';

/** 只有鼠标 / 触控板这类能悬停的精确指针才有连续轨迹；触屏只在拖动时产生事件，不展示 */
const FINE_POINTER = '(hover: hover) and (pointer: fine)';

/** 两次移动的间隔超过这个值（毫秒），视为在上一个位置停顿过 */
const PAUSE_GAP = 160;

function useMediaQuery(query: string) {
  const subscribe = useCallback(
    (onChange: () => void) => {
      const list = window.matchMedia(query);
      list.addEventListener('change', onChange);
      return () => list.removeEventListener('change', onChange);
    },
    [query],
  );
  return useSyncExternalStore(subscribe, () => window.matchMedia(query).matches);
}

/** 只认鼠标和手写笔；触摸拖动会和页面滚动抢事件，采到的轨迹没有代表性 */
const isTrackedPointer = (event: PointerEvent) =>
  event.pointerType === 'mouse' || event.pointerType === 'pen';

interface TrailPoint {
  x: number;
  y: number;
  /** performance.now() 时间戳 */
  t: number;
}

/**
 * 未登录页的背景氛围层：网格、光晕、扫描线都是纯 CSS。
 * 整层 fixed 铺满视口、不参与布局，也不接收指针事件
 */
export function GuestBackdrop() {
  return (
    <div className="guest-backdrop" aria-hidden="true">
      <div className="guest-backdrop__grid" />
      <div className="guest-backdrop__glow guest-backdrop__glow--violet" />
      <div className="guest-backdrop__glow guest-backdrop__glow--cyan" />
      <div className="guest-backdrop__scan" />
    </div>
  );
}

/** 读数刷新间隔与速度曲线长度：48 × 100ms ≈ 最近 5 秒 */
const METER_TICK = 100;
const HISTORY_SIZE = 48;
/** 速度取最近 250ms 的路程，直线度取最近 1.2s 的轨迹 */
const SPEED_WINDOW = 250;
const SHAPE_WINDOW = 1200;
/** 路程太短时直线度没有意义，保留上一次的值 */
const MIN_PATH = 40;
/** 速度曲线的纵轴下限（px/s），慢速移动时曲线不会被放大成满格 */
const SPEED_FLOOR = 800;
const CHART_HEIGHT = 32;

interface TrailStats {
  /** px/s */
  speed: number;
  /** 位移 / 路程，越接近 1 越直；还没有足够长的轨迹时为 null */
  straightness: number | null;
  pauses: number;
  samples: number;
  /** 速度历史，最新的在最后 */
  history: ReadonlyArray<number>;
  /** 最近一个速度窗口内有移动 */
  active: boolean;
}

const EMPTY_STATS: TrailStats = {
  speed: 0,
  straightness: null,
  pauses: 0,
  samples: 0,
  history: Array.from({ length: HISTORY_SIZE }, () => 0),
  active: false,
};

const pathLength = (points: ReadonlyArray<TrailPoint>) => {
  let length = 0;
  for (let i = 1; i < points.length; i++) {
    length += Math.hypot(points[i].x - points[i - 1].x, points[i].y - points[i - 1].y);
  }
  return length;
};

/**
 * 轨迹读数：示意 SDK 验证时采集的几类轨迹特征（速度、直线度、停顿），只在本页计算、不上报。
 * 只给原始量，不给评分或阈值，避免成为脚本调参的反馈
 */
export function TrailMeter() {
  const finePointer = useMediaQuery(FINE_POINTER);
  const [stats, setStats] = useState<TrailStats>(EMPTY_STATS);

  useEffect(() => {
    if (!finePointer) {
      return;
    }

    const recent: TrailPoint[] = [];
    let lastMove: TrailPoint | null = null;
    let pauses = 0;
    let samples = 0;
    let straightness: number | null = null;
    let history = EMPTY_STATS.history;
    let timer = 0;

    const tick = () => {
      const now = performance.now();
      while (recent.length > 0 && now - recent[0].t > SHAPE_WINDOW) {
        recent.shift();
      }

      const length = pathLength(recent);
      if (length >= MIN_PATH) {
        const first = recent[0];
        const last = recent[recent.length - 1];
        straightness = Math.hypot(last.x - first.x, last.y - first.y) / length;
      }
      const speed =
        pathLength(recent.filter((p) => now - p.t <= SPEED_WINDOW)) / (SPEED_WINDOW / 1000);
      history = [...history.slice(1), speed];

      const idle = lastMove ? now - lastMove.t : Number.POSITIVE_INFINITY;
      setStats({ speed, straightness, pauses, samples, history, active: idle <= SPEED_WINDOW });

      // 停下后再走完一轮历史（速度曲线回落到 0）就停表，空闲时不占 CPU
      if (idle > HISTORY_SIZE * METER_TICK) {
        clearInterval(timer);
        timer = 0;
      }
    };

    const onMove = (event: PointerEvent) => {
      if (!isTrackedPointer(event)) {
        return;
      }
      const now = performance.now();
      if (lastMove && now - lastMove.t > PAUSE_GAP) {
        pauses += 1;
      }
      lastMove = { x: event.clientX, y: event.clientY, t: now };
      recent.push(lastMove);
      samples += 1;
      if (!timer) {
        timer = window.setInterval(tick, METER_TICK);
      }
    };

    window.addEventListener('pointermove', onMove, { passive: true });
    return () => {
      window.removeEventListener('pointermove', onMove);
      clearInterval(timer);
    };
  }, [finePointer]);

  if (!finePointer) {
    return null;
  }

  const scale = Math.max(SPEED_FLOOR, ...stats.history);
  const line = stats.history
    .map(
      (value, i) => `${i},${(CHART_HEIGHT - 1 - (value / scale) * (CHART_HEIGHT - 2)).toFixed(1)}`,
    )
    .join(' ');

  return (
    <section className="trail-meter" aria-label="实时轨迹采样">
      <div className="trail-meter__head">
        <h2 className="trail-meter__title">
          <Icon name="activity" size={16} />
          实时轨迹采样
        </h2>
        <span
          className={
            stats.active ? 'trail-meter__state trail-meter__state--live' : 'trail-meter__state'
          }
        >
          <span className="trail-meter__dot" aria-hidden="true" />
          {stats.active ? '采样中' : '移动鼠标试试'}
        </span>
      </div>

      <dl className="trail-meter__stats">
        <div>
          <dt>速度</dt>
          <dd>
            {Math.round(stats.speed)}
            <small>px/s</small>
          </dd>
        </div>
        <div>
          <dt>直线度</dt>
          <dd>{stats.straightness === null ? '—' : stats.straightness.toFixed(2)}</dd>
        </div>
        <div>
          <dt>停顿</dt>
          <dd>
            {stats.pauses}
            <small>次</small>
          </dd>
        </div>
        <div>
          <dt>采样点</dt>
          <dd>{stats.samples}</dd>
        </div>
      </dl>

      <svg
        className="trail-meter__chart"
        viewBox={`0 0 ${HISTORY_SIZE - 1} ${CHART_HEIGHT}`}
        preserveAspectRatio="none"
        aria-hidden="true"
      >
        <polygon
          className="trail-meter__area"
          points={`0,${CHART_HEIGHT} ${line} ${HISTORY_SIZE - 1},${CHART_HEIGHT}`}
        />
        <polyline className="trail-meter__line" points={line} />
      </svg>

      <p className="trail-meter__note">
        示意 SDK 验证时采集的几类轨迹特征，只在本页计算，不会上报。
      </p>
    </section>
  );
}
