/* eslint-disable @typescript-eslint/no-explicit-any */
import { forwardRef, memo, useEffect, useImperativeHandle, useRef, useState } from 'react';
import type { SlideRegionConfig, SlideRegionData, SlideRegionEvent } from './slide-region.types';
import { defaultSlideRegionConfig, defaultSlideRegionData } from './slide-region.types';
import { CloseIcon, RefreshIcon, LoadingIcon } from '../icons';
import { useSlideRegionHandler } from './use-slide-region-handler';
import { useCaptchaConfig } from '../../CaptchaProvider';

export interface SlideRegionRef {
  reset: () => void;
  clear: () => void;
  refresh: () => void;
  close: () => void;
}

export interface SlideRegionProps extends React.HTMLAttributes<HTMLElement> {
  data: SlideRegionData;
  config?: SlideRegionConfig;
  events?: SlideRegionEvent;
}

const SlideRegion = forwardRef<SlideRegionRef, SlideRegionProps>((props, ref) => {
  const { locale } = useCaptchaConfig();
  const [localConfig, setLocalConfig] = useState<SlideRegionConfig>({ ...defaultSlideRegionConfig(), ...(props.config || {}) });
  const [localData, setLocalData] = useState<SlideRegionData>({ ...defaultSlideRegionData(), ...(props.data || {}) });
  const [localEvents, setLocalEvents] = useState<SlideRegionEvent>({ ...(props.events || {}) });

  useEffect(() => {
    setLocalConfig((prev) => ({ ...prev, ...(props.config || {}) }));
  }, [props.config]);

  useEffect(() => {
    setLocalData((prev) => ({ ...prev, ...(props.data || {}) }));
  }, [props.data]);

  useEffect(() => {
    setLocalEvents((prev) => ({ ...prev, ...(props.events || {}) }));
  }, [props.events]);

  const rootRef = useRef<any>(null);
  const containerRef = useRef<any>(null);
  const tileRef = useRef<any>(null);

  const handler = useSlideRegionHandler(
    localData, localEvents, localConfig,
    rootRef, containerRef, tileRef,
    () => { setLocalData((prev) => ({ ...prev, ...defaultSlideRegionData() })); },
  );

  const hPadding = localConfig.horizontalPadding || 0;
  const vPadding = localConfig.verticalPadding || 0;
  const width = (localConfig.width || 0) + (hPadding * 2) + (localConfig.showTheme ? 2 : 0);
  const hasDisplayWrapperState = (localConfig.width || 0) > 0 || (localConfig.height || 0) > 0;
  const hasDisplayImageState = (localData.image && localData.image.length > 0) && (localData.thumb && localData.thumb.length > 0);

  useImperativeHandle(ref, () => ({
    reset: handler.resetData, clear: handler.clearData,
    refresh: handler.refresh, close: handler.close,
  }));

  useEffect(() => {
    const fn = (event: Event) => event.preventDefault();
    tileRef.current && tileRef.current.addEventListener('dragstart', fn);
    return () => { tileRef.current && tileRef.current.removeEventListener('dragstart', fn); };
  }, [tileRef]);

  return (
    <div
      className={`gc-wrapper ${localConfig.showTheme ? 'gc-wrapper-theme' : ''}`}
      style={{
        width: width + 'px', paddingLeft: hPadding + 'px', paddingRight: hPadding + 'px',
        paddingTop: vPadding + 'px', paddingBottom: vPadding + 'px',
        display: hasDisplayWrapperState ? 'block' : 'none',
      }}
      ref={rootRef}
    >
      <div className="gc-header gc-header-center">
        <span>{localConfig.title || locale.slideTitle}</span>
      </div>
      <div className="gc-body" ref={containerRef}
        style={{ width: localConfig.width + 'px', height: localConfig.height + 'px' }}>
        <div className="gc-loading"><LoadingIcon /></div>
        <img className={`gc-picture ${localData.image === '' ? 'gc-hide' : ''}`}
          src={localData.image}
          style={{ width: localConfig.width + 'px', height: localConfig.height + 'px', display: hasDisplayImageState ? 'block' : 'none' }} alt="" />
        <div className="gc-tile gc-tile-region" ref={tileRef}
          style={{ width: (localData.thumbWidth || 0) + 'px', height: (localData.thumbHeight || 0) + 'px',
            top: handler.thumbPoint.y + 'px', left: handler.thumbPoint.x + 'px' }}
          onMouseDown={handler.dragEvent} onTouchStart={handler.dragEvent}>
          <img className={localData.thumb === '' ? 'gc-hide' : ''}
            style={{ display: hasDisplayImageState ? 'block' : 'none' }} src={localData.thumb} alt="" />
        </div>
      </div>
      <div className="gc-footer">
        <div className="gc-icon-block">
          <CloseIcon width={localConfig.iconSize} height={localConfig.iconSize} onClick={handler.closeEvent} />
          <RefreshIcon width={localConfig.iconSize} height={localConfig.iconSize} onClick={handler.refreshEvent} />
        </div>
      </div>
    </div>
  );
});

SlideRegion.displayName = 'SlideRegion';
export default memo(SlideRegion);
