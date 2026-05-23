import { forwardRef, useImperativeHandle, useMemo, useRef, useState } from 'react';
import type {
  CaptchaErrorResponse,
  CaptchaProps,
  CaptchaRef,
  CaptchaVerifySuccessData,
} from './Captcha.types';
import { noop, getRandomType } from './utils';
import Popup from './Popup';
import { CaptchaStyle } from './CaptchaStyle';
import { Click, Slide, SlideRegion, Rotate } from './_internal';
import type { ClickDot, SlideRegionPoint, SlidePoint } from './_internal';
import { getCaptchaErrorMessage, useCaptchaRequest } from './hooks';
import { CaptchaProvider, useCaptchaConfig } from './CaptchaProvider';
import type { ClickData } from './_internal/click/click.types';
import type { RotateData } from './_internal/rotate/rotate.types';
import type { SlideData } from './_internal/slide/slide.types';
import type { SlideRegionData } from './_internal/slide-region/slide-region.types';

const CaptchaInternal = forwardRef<CaptchaRef, Omit<CaptchaProps, 'locale' | 'theme' | 'api'>>((props, ref) => {
  const {
    onCancel = noop,
    onSuccess = noop as (data: CaptchaVerifySuccessData) => void,
    onFail = noop as (error: CaptchaErrorResponse | string) => void,
    className,
    style,
    config,
    type: typeProp = 'auto',
    children,
  } = props;
  const [visible, setVisible] = useState<boolean>(false);
  const { locale } = useCaptchaConfig();

  const type = useMemo(() => {
    if (typeProp === 'auto') {
      return getRandomType();
    }

    return typeProp || 'click-text';
  }, [typeProp]);

  const { get, check, data } = useCaptchaRequest({ type, mode: 'dark' });
  const captchaRef = useRef<any>(null);

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
      thumbSize: data.thumbWidth || data.thumbHeight || 0,
    };
  }, [data]);

  const slideData = useMemo<SlideData | null>(() => {
    if (!data?.image || !data.thumb) {
      return null;
    }

    return {
      image: data.image,
      thumb: data.thumb,
      thumbX: data.thumbX || 0,
      thumbY: data.thumbY || 0,
      thumbWidth: data.thumbWidth || 0,
      thumbHeight: data.thumbHeight || 0,
    };
  }, [data]);

  const slideRegionData = useMemo<SlideRegionData | null>(() => {
    if (!data?.image || !data.thumb) {
      return null;
    }

    return {
      image: data.image,
      thumb: data.thumb,
      thumbX: data.thumbX || 0,
      thumbY: data.thumbY || 0,
      thumbWidth: data.thumbWidth || 0,
      thumbHeight: data.thumbHeight || 0,
    };
  }, [data]);

  const start = async () => {
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

  const close = () => {
    setVisible(false);
    onFail(locale.userCancel!);
    onCancel();
  };

  const confirm = async (rawBody: Record<string, string>) => {
    try {
      const result = await check(rawBody);
      setVisible(false);
      onSuccess(result);
    } catch (error) {
      onFail(error as CaptchaErrorResponse | string);
      const errorMessage = getCaptchaErrorMessage(error as CaptchaErrorResponse | string);
      if (errorMessage === locale.userCancel) {
        return;
      }
      await start();
    }
  };

  const click = async (dots: ClickDot[]) => {
    const dotStr = dots.flatMap((item) => [item.x, item.y]).join(',');
    await confirm({ dots: dotStr });
  };

  const slide = async (point: SlideRegionPoint | SlidePoint) => {
    await confirm({ point: [point.x, point.y].join(',') });
  };

  const rotate = async (angle: number) => {
    await confirm({ angle: `${angle}` });
  };

  useImperativeHandle(ref, () => ({ verify: start }), [start]);

  const events = {
    close,
    refresh: start,
  };

  return (
    <div className={className} style={{ position: 'relative', display: 'inline-block', ...style }}>
      <Popup visible={visible}>
        {clickData && ['click-text', 'click-shape'].includes(type) && (
          <Click
            config={config}
            data={clickData}
            ref={captchaRef}
            events={{ ...events, confirm: click }}
          />
        )}
        {rotateData && type === 'rotate' && (
          <Rotate
            config={config}
            data={rotateData}
            ref={captchaRef}
            events={{ ...events, confirm: rotate }}
          />
        )}
        {slideRegionData && type === 'slide-region' && (
          <SlideRegion
            config={config}
            data={slideRegionData}
            ref={captchaRef}
            events={{ ...events, confirm: slide }}
          />
        )}
        {slideData && type === 'slide-text' && (
          <Slide
            config={config}
            data={slideData}
            ref={captchaRef}
            events={{ ...events, confirm: slide }}
          />
        )}
      </Popup>
      {children}
    </div>
  );
});

const Captcha = forwardRef<CaptchaRef, CaptchaProps>((props, ref) => {
  const { locale, theme, api, ...restProps } = props;

  return (
    <CaptchaProvider locale={locale} theme={theme} api={api}>
      <CaptchaStyle />
      <CaptchaInternal {...restProps} ref={ref} />
    </CaptchaProvider>
  );
});

export default Captcha;
