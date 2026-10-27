import { useCallback, useEffect, useRef, useState, useSyncExternalStore } from 'react';
import { Icon } from '../components/Icon';
import './GuestPages.css';

/** 只有鼠标 / 触控板这类能悬停的精确指针才有连续轨迹；触屏只在拖动时产生事件，不展示 */
const FINE_POINTER = '(hover: hover) and (pointer: fine)';
const REDUCED_MOTION = '(prefers-reduced-motion: reduce)';

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
 * 未登录页的背景氛围层：网格、光晕、扫描线都是纯 CSS；鼠标轨迹层只在精确指针且未要求减少动态效果时启用。
 * 整层 fixed 铺满视口、不参与布局，也不接收指针事件
 */
export function GuestBackdrop() {
  const finePointer = useMediaQuery(FINE_POINTER);
  const reducedMotion = useMediaQuery(REDUCED_MOTION);

  return (
    <>
      <div className="guest-backdrop" aria-hidden="true">
        <div className="guest-backdrop__grid" />
        <div className="guest-backdrop__glow guest-backdrop__glow--violet" />
        <div className="guest-backdrop__glow guest-backdrop__glow--cyan" />
        <div className="guest-backdrop__scan" />
      </div>
      {finePointer && !reducedMotion && <TrailCanvas />}
    </>
  );
}

/** 轨迹点与停顿标记的存活时间（毫秒） */
const TRAIL_LIFE = 900;
const PAUSE_LIFE = 1600;
/** 高回报率鼠标下限制点数，避免一帧里画太多段 */
const TRAIL_MAX_POINTS = 240;

/**
 * 鼠标轨迹层：连线 + 采样点 + 停顿标记，随时间淡出。
 * 监听器是 passive 的、不拦截也不阻止冒泡，canvas 不接收指针事件，不影响 SDK 自己的轨迹采集；
 * 没有存活的点时不排下一帧，鼠标不动就不占 CPU
 */
function TrailCanvas() {
  const canvasRef = useRef<HTMLCanvasElement>(null);

  useEffect(() => {
    const canvas = canvasRef.current;
    const ctx = canvas?.getContext('2d');
    if (!canvas || !ctx) {
      return;
    }

    const points: TrailPoint[] = [];
    const pauses: TrailPoint[] = [];
    let lastMove: TrailPoint | null = null;
    let frame = 0;

    const resize = () => {
      const dpr = Math.min(window.devicePixelRatio || 1, 2);
      canvas.width = Math.round(window.innerWidth * dpr);
      canvas.height = Math.round(window.innerHeight * dpr);
      ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    };

    const draw = () => {
      const now = performance.now();
      while (points.length > 0 && now - points[0].t > TRAIL_LIFE) {
        points.shift();
      }
      while (pauses.length > 0 && now - pauses[0].t > PAUSE_LIFE) {
        pauses.shift();
      }

      ctx.clearRect(0, 0, window.innerWidth, window.innerHeight);
      ctx.lineWidth = 1.5;
      ctx.lineCap = 'round';

      // 连线：越早的段越淡
      for (let i = 1; i < points.length; i++) {
        const from = points[i - 1];
        const to = points[i];
        const life = 1 - (now - to.t) / TRAIL_LIFE;
        ctx.strokeStyle = `rgba(143, 132, 255, ${(life * 0.7).toFixed(3)})`;
        ctx.beginPath();
        ctx.moveTo(from.x, from.y);
        ctx.lineTo(to.x, to.y);
        ctx.stroke();
      }

      // 采样点：每个 pointermove 一个
      for (const point of points) {
        const life = 1 - (now - point.t) / TRAIL_LIFE;
        ctx.fillStyle = `rgba(34, 211, 238, ${(life * 0.85).toFixed(3)})`;
        ctx.beginPath();
        ctx.arc(point.x, point.y, 1.4, 0, Math.PI * 2);
        ctx.fill();
      }

      // 停顿：在停留过的位置画一个菱形
      for (const pause of pauses) {
        const life = 1 - (now - pause.t) / PAUSE_LIFE;
        ctx.strokeStyle = `rgba(251, 191, 36, ${(life * 0.9).toFixed(3)})`;
        ctx.beginPath();
        ctx.moveTo(pause.x, pause.y - 5);
        ctx.lineTo(pause.x + 5, pause.y);
        ctx.lineTo(pause.x, pause.y + 5);
        ctx.lineTo(pause.x - 5, pause.y);
        ctx.closePath();
        ctx.stroke();
      }

      // 当前位置：一圈光环
      const head = points[points.length - 1];
      if (head) {
        const life = 1 - (now - head.t) / TRAIL_LIFE;
        ctx.strokeStyle = `rgba(34, 211, 238, ${(life * 0.6).toFixed(3)})`;
        ctx.beginPath();
        ctx.arc(head.x, head.y, 7, 0, Math.PI * 2);
        ctx.stroke();
      }

      frame = points.length > 0 || pauses.length > 0 ? requestAnimationFrame(draw) : 0;
    };

    const onMove = (event: PointerEvent) => {
      if (!isTrackedPointer(event)) {
        return;
      }
      const now = performance.now();
      if (lastMove && now - lastMove.t > PAUSE_GAP) {
        pauses.push({ x: lastMove.x, y: lastMove.y, t: now });
      }
      lastMove = { x: event.clientX, y: event.clientY, t: now };
      points.push(lastMove);
      if (points.length > TRAIL_MAX_POINTS) {
        points.shift();
      }
      if (!frame) {
        frame = requestAnimationFrame(draw);
      }
    };

    resize();
    window.addEventListener('resize', resize);
    window.addEventListener('pointermove', onMove, { passive: true });
    return () => {
      window.removeEventListener('resize', resize);
      window.removeEventListener('pointermove', onMove);
      cancelAnimationFrame(frame);
    };
  }, []);

  return <canvas ref={canvasRef} className="guest-trail" />;
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
