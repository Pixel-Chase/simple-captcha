import { forwardRef, useEffect, useImperativeHandle, useMemo, useRef, useState } from 'react';
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
import { getCaptchaErrorMessage, useCaptchaRequest } from './hooks';
import { CaptchaProvider, useCaptchaConfig } from './CaptchaProvider';
import type { ClickData } from './_internal/click/click.types';
import type { RotateData } from './_internal/rotate/rotate.types';
import type { SlideData } from './_internal/slide/slide.types';
import type { SlideRegionData } from './_internal/slide-region/slide-region.types';

const DEFAULT_MAX_RETRY_COUNT = 5;
/** 与各验证模式默认配置一致的渲染尺寸（CSS 像素） */
const DEFAULT_RENDER_WIDTH = 300;
const DEFAULT_RENDER_HEIGHT = 220;
const DEFAULT_ROTATE_SIZE = 220;

interface Ratio {
  x: number;
  y: number;
}

const normalizeMaxRetryCount = (value?: number) => {
  if (typeof value !== 'number' || !Number.isFinite(value)) {
    return DEFAULT_MAX_RETRY_COUNT;
  }

  return Math.max(0, Math.floor(value));
};

/** 图像像素 → CSS 像素 */
const toCss = (value: number, ratio: number) => Math.round(value / ratio);
/** CSS 像素 → 图像像素 */
const toImage = (value: number, ratio: number) => Math.round(value * ratio);

const CaptchaInternal = forwardRef<
  CaptchaRef,
  Omit<CaptchaProps, 'locale' | 'theme' | 'api' | 'telemetry'>
>((props, ref) => {
  const {
    onCancel = noop,
    onSuccess = noop as (data: CaptchaVerifySuccessData) => void,
    onFail = noop as (error: CaptchaErrorResponse | string) => void,
    className,
    style,
    config,
    type: typeProp = 'auto',
    maxRetryCount: maxRetryCountProp,
    children,
  } = props;
  const [visible, setVisible] = useState<boolean>(false);
  const { locale, telemetry } = useCaptchaConfig();

  const maxRetryCount = normalizeMaxRetryCount(maxRetryCountProp);

  // auto 由服务端按风险等级解析为具体题型，响应中的 type 即实际题型
  const { get, check, data } = useCaptchaRequest({ type: typeProp, mode: 'dark' });
  const resolvedType: ResolvedCaptchaType =
    data?.type || (typeProp === 'auto' ? 'click-text' : typeProp);

  const captchaRef = useRef<any>(null);
  const failCountRef = useRef(0);

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
    },
    [],
  );

  // ---------- 坐标归一化：服务端按位图像素校验，SDK 按 CSS 像素渲染 ----------
  const ratio = useMemo<Ratio>(() => {
    if (!data?.imageWidth || !data?.imageHeight) {
      return { x: 1, y: 1 };
    }
    if (resolvedType === 'rotate') {
      const size = config?.size || DEFAULT_ROTATE_SIZE;
      return { x: data.imageWidth / size, y: data.imageWidth / size };
    }
    const width = config?.width || DEFAULT_RENDER_WIDTH;
    const height = config?.height || DEFAULT_RENDER_HEIGHT;
    return { x: data.imageWidth / width, y: data.imageHeight / height };
  }, [data, resolvedType, config?.width, config?.height, config?.size]);

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

    setVisible(true);
    try {
      await get();
    } catch (error) {
      setVisible(false);
      onFail(error as CaptchaErrorResponse | string);
      throw error;
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
    setVisible(false);
    onFail(locale.userCancel!);
    onCancel();
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
      setVisible(false);
      onSuccess(result);
    } catch (error) {
      const normalizedError = error as CaptchaErrorResponse | string;
      const errorMessage = getCaptchaErrorMessage(normalizedError);
      if (errorMessage === locale.userCancel) {
        onFail(normalizedError);
        return;
      }

      failCountRef.current += 1;
      if (failCountRef.current >= maxRetryCount) {
        failCountRef.current = 0;
        setVisible(false);
        onFail(normalizedError);
        return;
      }

      await refresh().catch(noop);
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

  useImperativeHandle(ref, () => ({ verify: start }), [start]);

  const events = {
    close,
    refresh,
  };

  return (
    <div className={className} style={{ position: 'relative', display: 'inline-block', ...style }}>
      <Popup visible={visible}>
        <div ref={attach}>
          {clickData && ['click-text', 'click-shape'].includes(resolvedType) && (
            <Click
              config={config}
              data={clickData}
              ref={captchaRef}
              events={{ ...events, confirm: click, click: recordClick }}
            />
          )}
          {rotateData && resolvedType === 'rotate' && (
            <Rotate
              config={config}
              data={rotateData}
              ref={captchaRef}
              events={{ ...events, confirm: rotate }}
            />
          )}
          {slideRegionData && resolvedType === 'slide-region' && (
            <SlideRegion
              config={config}
              data={slideRegionData}
              ref={captchaRef}
              events={{ ...events, confirm: slide }}
            />
          )}
          {slideData && resolvedType === 'slide-text' && (
            <Slide
              config={config}
              data={slideData}
              ref={captchaRef}
              events={{ ...events, confirm: slide }}
            />
          )}
        </div>
      </Popup>
      {children}
    </div>
  );
});

const Captcha = forwardRef<CaptchaRef, CaptchaProps>((props, ref) => {
  const { locale, theme, api, telemetry, ...restProps } = props;

  return (
    <CaptchaProvider locale={locale} theme={theme} api={api} telemetry={telemetry}>
      <CaptchaStyle />
      <CaptchaInternal {...restProps} ref={ref} />
    </CaptchaProvider>
  );
});

export default Captcha;
