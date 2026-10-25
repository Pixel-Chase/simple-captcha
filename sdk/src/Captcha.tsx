import { forwardRef, useEffect, useImperativeHandle, useMemo, useRef, useState } from 'react';
import type { ForwardRefExoticComponent, RefAttributes } from 'react';
import type {
  CaptchaEnv,
  CaptchaErrorResponse,
  CaptchaPowSolution,
  CaptchaProps,
  CaptchaRef,
  CaptchaVerifyPayload,
  CaptchaVerifySuccessData,
  ResolvedCaptchaType,
} from './Captcha.types';
import { noop } from './utils';
import Popup from './Popup';
import { CaptchaStyle } from './CaptchaStyle';
import { Click, Slide, SlideRegion, Rotate } from './_internal';
import type { ClickDot, SlideRegionPoint, SlidePoint } from './_internal';
import { collectEnv, solvePow, useBehaviorTrace } from './_internal/telemetry';
import { useCaptchaRequest } from './hooks';
import { useCaptchaConfig } from './CaptchaProvider';
import { CaptchaError } from './CaptchaError';
import type { CaptchaStatus } from './_internal/shared/status.types';
import type { ClickData } from './_internal/click/click.types';
import type { RotateData } from './_internal/rotate/rotate.types';
import type { SlideData } from './_internal/slide/slide.types';
import type { SlideRegionData } from './_internal/slide-region/slide-region.types';

const DEFAULT_MAX_ATTEMPTS = 5;
/** 与各验证模式默认配置一致的渲染宽度（CSS 像素），题图统一 3:2 */
const DEFAULT_RENDER_WIDTH = 300;
const RENDER_ASPECT = 3 / 2;
const DEFAULT_ROTATE_SIZE = 128;
/** 校验结果画面的停留时长（毫秒） */
const SUCCESS_HOLD_MS = 900;
const FAIL_HOLD_MS = 1000;

interface Ratio {
  x: number;
  y: number;
}

const normalizeMaxAttempts = (value?: number) => {
  if (typeof value !== 'number' || !Number.isFinite(value)) {
    return DEFAULT_MAX_ATTEMPTS;
  }

  return Math.max(1, Math.floor(value));
};

const normalizeWidth = (value?: number) =>
  typeof value === 'number' && Number.isFinite(value) && value > 0
    ? Math.round(value)
    : DEFAULT_RENDER_WIDTH;

/** 图像像素 → CSS 像素 */
const toCss = (value: number, ratio: number) => Math.round(value / ratio);
/** CSS 像素 → 图像像素 */
const toImage = (value: number, ratio: number) => Math.round(value * ratio);

export interface CaptchaInternalProps extends CaptchaProps {
  /** 弹窗彻底关闭后触发（含校验结果画面的展示时间），useCaptcha 据此延后卸载 holder */
  onClosed?: () => void;
}

export const CaptchaInternal = forwardRef<CaptchaRef, CaptchaInternalProps>((props, ref) => {
  const {
    onCancel = noop,
    onSuccess = noop as (data: CaptchaVerifySuccessData) => void,
    onFail = noop as (error: CaptchaError) => void,
    onClosed = noop,
    type: typeProp = 'auto',
    maxAttempts: maxAttemptsProp,
    width: widthProp,
    children,
  } = props;
  const [visible, setVisible] = useState<boolean>(false);
  const { telemetry } = useCaptchaConfig();

  const maxAttempts = normalizeMaxAttempts(maxAttemptsProp);
  const width = normalizeWidth(widthProp);
  const height = Math.round(width / RENDER_ASPECT);
  const config = useMemo(() => ({ width, height }), [width, height]);

  // auto 由服务端按风险等级解析为具体题型，响应中的 type 即实际题型
  const { get, check, data } = useCaptchaRequest({ type: typeProp, mode: 'dark' });
  const resolvedType: ResolvedCaptchaType =
    data?.type || (typeProp === 'auto' ? 'click-text' : typeProp);

  const captchaRef = useRef<any>(null);
  const failCountRef = useRef(0);
  const [status, setStatus] = useState<CaptchaStatus>('idle');
  const [elapsed, setElapsed] = useState<number>(0);
  const shownAtRef = useRef(0);
  const holdTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  // 关闭 / 重新加载时自增，使仍在等待中的“结果展示后刷新”失效，避免已关闭的弹窗被重新打开
  const sessionRef = useRef(0);

  // ---------- 行为采集 / 环境信号 / PoW ----------
  const telemetryEnabled = telemetry !== 'off';
  const { attach, markLoaded, recordClick, snapshot } = useBehaviorTrace(telemetryEnabled);
  const envRef = useRef<CaptchaEnv | undefined>(undefined);
  const powRef = useRef<Promise<CaptchaPowSolution | null>>(Promise.resolve(null));
  const powCancelRef = useRef<{ current: boolean }>({ current: false });

  useEffect(() => {
    if (!data) {
      return;
    }
    markLoaded();
    envRef.current = telemetryEnabled ? collectEnv(telemetry) : undefined;

    // 题目一到就在后台开始算 PoW，用户完成操作时通常已算完
    powCancelRef.current.current = true;
    const cancelled = { current: false };
    powCancelRef.current = cancelled;
    powRef.current = data.pow
      ? solvePow(data.pow, data.key, { cancelled }).catch(() => null)
      : Promise.resolve(null);
  }, [data, markLoaded, telemetry, telemetryEnabled]);

  useEffect(
    () => () => {
      powCancelRef.current.current = true;
      if (holdTimerRef.current) {
        clearTimeout(holdTimerRef.current);
      }
    },
    [],
  );

  // 题目图片就绪的时刻，作为“耗时”的起点
  useEffect(() => {
    if (data) {
      shownAtRef.current = Date.now();
    }
  }, [data]);

  const clearHoldTimer = () => {
    if (holdTimerRef.current) {
      clearTimeout(holdTimerRef.current);
      holdTimerRef.current = null;
    }
  };

  const hideAfter = (ms: number) => {
    clearHoldTimer();
    holdTimerRef.current = setTimeout(() => {
      holdTimerRef.current = null;
      setVisible(false);
      onClosed();
    }, ms);
  };

  // ---------- 坐标归一化：服务端按位图像素校验，SDK 按 CSS 像素渲染 ----------
  const ratio = useMemo<Ratio>(() => {
    if (!data?.imageWidth || !data?.imageHeight) {
      return { x: 1, y: 1 };
    }
    if (resolvedType === 'rotate') {
      return { x: data.imageWidth / DEFAULT_ROTATE_SIZE, y: data.imageWidth / DEFAULT_ROTATE_SIZE };
    }
    return { x: data.imageWidth / width, y: data.imageHeight / height };
  }, [data, resolvedType, width, height]);

  const clickData = useMemo<ClickData | null>(() => {
    if (!data?.image || !data.thumb) {
      return null;
    }

    return {
      image: data.image,
      thumb: data.thumb,
    };
  }, [data]);

  const rotateData = useMemo<RotateData | null>(() => {
    if (!data?.image || !data.thumb) {
      return null;
    }

    return {
      angle: 0,
      image: data.image,
      thumb: data.thumb,
      thumbSize: toCss(data.thumbWidth || data.thumbHeight || 0, ratio.x),
    };
  }, [data, ratio]);

  const slideData = useMemo<SlideData | null>(() => {
    if (!data?.image || !data.thumb) {
      return null;
    }

    return {
      image: data.image,
      thumb: data.thumb,
      thumbX: toCss(data.thumbX || 0, ratio.x),
      thumbY: toCss(data.thumbY || 0, ratio.y),
      thumbWidth: toCss(data.thumbWidth || 0, ratio.x),
      thumbHeight: toCss(data.thumbHeight || 0, ratio.y),
    };
  }, [data, ratio]);

  const slideRegionData = useMemo<SlideRegionData | null>(() => {
    if (!data?.image || !data.thumb) {
      return null;
    }

    return {
      image: data.image,
      thumb: data.thumb,
      thumbX: toCss(data.thumbX || 0, ratio.x),
      thumbY: toCss(data.thumbY || 0, ratio.y),
      thumbWidth: toCss(data.thumbWidth || 0, ratio.x),
      thumbHeight: toCss(data.thumbHeight || 0, ratio.y),
    };
  }, [data, ratio]);

  const loadCaptcha = async () => {
    if (captchaRef.current) {
      captchaRef.current.clear();
    }

    clearHoldTimer();
    sessionRef.current += 1;
    const session = sessionRef.current;
    setStatus('idle');
    setVisible(true);
    try {
      await get();
    } catch (error) {
      // 加载期间用户已关闭弹窗，取消已经通知过，不再补报失败
      if (session !== sessionRef.current) {
        return;
      }
      const response = error as CaptchaErrorResponse;
      setVisible(false);
      onFail(new CaptchaError('LOAD_FAILED', response.error, response));
      onClosed();
    }
  };

  const start = async () => {
    failCountRef.current = 0;
    await loadCaptcha();
  };

  const refresh = async () => {
    await loadCaptcha();
  };

  const close = () => {
    failCountRef.current = 0;
    clearHoldTimer();
    sessionRef.current += 1;
    setVisible(false);
    onCancel();
    onClosed();
  };

  const confirm = async (answer: CaptchaVerifyPayload) => {
    const payload: CaptchaVerifyPayload = { ...answer };
    if (telemetryEnabled) {
      payload.trace = snapshot();
      payload.env = envRef.current;
    }
    const pow = await powRef.current;
    if (pow) {
      payload.pow = pow;
    }

    try {
      const result = await check(payload);
      failCountRef.current = 0;
      setElapsed((Date.now() - shownAtRef.current) / 1000);
      setStatus('success');
      hideAfter(SUCCESS_HOLD_MS);
      onSuccess(result);
    } catch (error) {
      failCountRef.current += 1;
      setStatus('fail');
      if (failCountRef.current >= maxAttempts) {
        failCountRef.current = 0;
        hideAfter(FAIL_HOLD_MS);
        onFail(
          new CaptchaError('MAX_ATTEMPTS', '验证未通过次数已达上限', error as CaptchaErrorResponse),
        );
        return;
      }

      const session = sessionRef.current;
      await new Promise((resolve) => setTimeout(resolve, FAIL_HOLD_MS));
      if (session !== sessionRef.current) {
        return;
      }
      await refresh();
    }
  };

  const click = async (dots: ClickDot[]) => {
    const dotStr = dots
      .flatMap((item) => [toImage(item.x, ratio.x), toImage(item.y, ratio.y)])
      .join(',');
    await confirm({ dots: dotStr });
  };

  const slide = async (point: SlideRegionPoint | SlidePoint) => {
    await confirm({ point: [toImage(point.x, ratio.x), toImage(point.y, ratio.y)].join(',') });
  };

  const rotate = async (angle: number) => {
    await confirm({ angle: `${angle}` });
  };

  useImperativeHandle(ref, () => ({ open: start }), [start]);

  const events = {
    close,
    refresh,
  };

  return (
    <>
      <CaptchaStyle />
      <Popup visible={visible}>
        <div ref={attach}>
          {clickData && ['click-text', 'click-shape'].includes(resolvedType) && (
            <Click
              config={config}
              status={status}
              elapsed={elapsed}
              data={clickData}
              ref={captchaRef}
              events={{ ...events, confirm: click, click: recordClick }}
            />
          )}
          {rotateData && resolvedType === 'rotate' && (
            <Rotate
              config={config}
              status={status}
              elapsed={elapsed}
              data={rotateData}
              ref={captchaRef}
              events={{ ...events, confirm: rotate }}
            />
          )}
          {slideRegionData && resolvedType === 'slide-region' && (
            <SlideRegion
              config={config}
              status={status}
              elapsed={elapsed}
              data={slideRegionData}
              ref={captchaRef}
              events={{ ...events, confirm: slide }}
            />
          )}
          {slideData && resolvedType === 'slide-text' && (
            <Slide
              config={config}
              status={status}
              elapsed={elapsed}
              data={slideData}
              ref={captchaRef}
              events={{ ...events, confirm: slide }}
            />
          )}
        </div>
      </Popup>
      {children}
    </>
  );
});

/** 对外类型不含 onClosed，它只供 useCaptcha 使用 */
const Captcha: ForwardRefExoticComponent<CaptchaProps & RefAttributes<CaptchaRef>> =
  CaptchaInternal;

export default Captcha;
