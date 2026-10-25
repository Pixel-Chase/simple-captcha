import { useCallback, useRef } from 'react';
import type { CaptchaTrace, TracePoint } from '../../Captcha.types';

/** 采样点上限，超出后做二分降采样并把采样间隔翻倍 */
const MAX_POINTS = 240;
/** 基础采样间隔（毫秒），略高于 120Hz 指针事件频率 */
const BASE_INTERVAL_MS = 8;
const MAX_CLICKS = 40;

interface TraceState {
  loadedAt: number;
  startedAt: number;
  pointerType: string;
  points: TracePoint[];
  clicks: TracePoint[];
  downs: number;
  ups: number;
  moves: number;
  untrusted: number;
  lastPointAt: number;
  minInterval: number;
}

const now = (): number => (typeof performance !== 'undefined' ? performance.now() : Date.now());

const createState = (): TraceState => ({
  loadedAt: now(),
  startedAt: 0,
  pointerType: '',
  points: [],
  clicks: [],
  downs: 0,
  ups: 0,
  moves: 0,
  untrusted: 0,
  lastPointAt: -Infinity,
  minInterval: BASE_INTERVAL_MS,
});

const round1 = (value: number): number => Math.round(value * 10) / 10;

export interface BehaviorTraceHandle {
  /** 作为 ref 挂到需要监听的容器元素上 */
  attach: (element: HTMLElement | null) => void;
  /** 题目加载完成时调用，重置时间基准与采样数据 */
  markLoaded: () => void;
  /** 记录一次点选（坐标为相对图片的 CSS 像素） */
  recordClick: (x: number, y: number) => void;
  /** 生成当前轨迹快照 */
  snapshot: () => CaptchaTrace;
}

/**
 * 采集验证过程中的指针轨迹。
 * 监听挂在容器元素上（捕获阶段、passive），不干扰各验证模式自身的拖动逻辑。
 */
export function useBehaviorTrace(enabled: boolean): BehaviorTraceHandle {
  const stateRef = useRef<TraceState>(createState());
  const cleanupRef = useRef<(() => void) | null>(null);

  const relative = useCallback((): number => now() - stateRef.current.loadedAt, []);

  const pushPoint = useCallback(
    (event: PointerEvent, element: HTMLElement, force: boolean) => {
      const state = stateRef.current;
      const t = relative();
      if (!force && t - state.lastPointAt < state.minInterval) {
        return;
      }
      state.lastPointAt = t;

      const rect = element.getBoundingClientRect();
      state.points.push([
        round1(event.clientX - rect.left),
        round1(event.clientY - rect.top),
        Math.round(t),
      ]);

      if (state.points.length > MAX_POINTS) {
        state.points = state.points.filter((_, index) => index % 2 === 0);
        state.minInterval *= 2;
      }
    },
    [relative],
  );

  const attach = useCallback(
    (element: HTMLElement | null) => {
      if (cleanupRef.current) {
        cleanupRef.current();
        cleanupRef.current = null;
      }
      if (!element || !enabled || typeof window === 'undefined' || !('PointerEvent' in window)) {
        return;
      }

      const onDown = (event: PointerEvent) => {
        const state = stateRef.current;
        state.downs += 1;
        if (!event.isTrusted) {
          state.untrusted += 1;
        }
        if (!state.startedAt) {
          state.startedAt = relative();
          state.pointerType = event.pointerType || 'unknown';
        }
        pushPoint(event, element, true);
      };

      const onMove = (event: PointerEvent) => {
        const state = stateRef.current;
        state.moves += 1;
        if (!event.isTrusted) {
          state.untrusted += 1;
        }
        pushPoint(event, element, false);
      };

      const onUp = (event: PointerEvent) => {
        const state = stateRef.current;
        state.ups += 1;
        if (!event.isTrusted) {
          state.untrusted += 1;
        }
        pushPoint(event, element, true);
      };

      // 脚本常用 dispatchEvent 合成 mouse/touch 事件而非 pointer 事件，这里只统计其可信度，不参与采样
      const onLegacy = (event: Event) => {
        if (!event.isTrusted) {
          stateRef.current.untrusted += 1;
        }
      };
      const legacyEvents = [
        'mousedown',
        'mousemove',
        'mouseup',
        'touchstart',
        'touchmove',
        'touchend',
        'click',
      ];

      const options: AddEventListenerOptions = { capture: true, passive: true };
      element.addEventListener('pointerdown', onDown, options);
      element.addEventListener('pointermove', onMove, options);
      element.addEventListener('pointerup', onUp, options);
      element.addEventListener('pointercancel', onUp, options);
      legacyEvents.forEach((name) => {
        element.addEventListener(name, onLegacy, options);
      });

      cleanupRef.current = () => {
        element.removeEventListener('pointerdown', onDown, options);
        element.removeEventListener('pointermove', onMove, options);
        element.removeEventListener('pointerup', onUp, options);
        element.removeEventListener('pointercancel', onUp, options);
        legacyEvents.forEach((name) => {
          element.removeEventListener(name, onLegacy, options);
        });
      };
    },
    [enabled, pushPoint, relative],
  );

  const markLoaded = useCallback(() => {
    stateRef.current = createState();
  }, []);

  const recordClick = useCallback(
    (x: number, y: number) => {
      const state = stateRef.current;
      if (state.clicks.length >= MAX_CLICKS) {
        return;
      }
      state.clicks.push([round1(x), round1(y), Math.round(relative())]);
    },
    [relative],
  );

  const snapshot = useCallback((): CaptchaTrace => {
    const state = stateRef.current;
    return {
      points: state.points,
      clicks: state.clicks.length > 0 ? state.clicks : undefined,
      pointerType: state.pointerType || undefined,
      startedAt: Math.round(state.startedAt),
      submittedAt: Math.round(relative()),
      downs: state.downs,
      ups: state.ups,
      moves: state.moves,
      untrusted: state.untrusted,
    };
  }, [relative]);

  return { attach, markLoaded, recordClick, snapshot };
}
