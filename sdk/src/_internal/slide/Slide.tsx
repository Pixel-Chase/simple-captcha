/* eslint-disable @typescript-eslint/no-explicit-any */
import { forwardRef, memo, useEffect, useImperativeHandle, useRef, useState } from 'react';
import type { SlideConfig, SlideData, SlideEvent } from './slide.types';
import { defaultSlideConfig, defaultSlideData } from './slide.types';
import { CloseIcon, RefreshIcon, LoadingIcon, ArrowsIcon } from '../icons';
import { useSlideHandler } from './use-slide-handler';
import { useCaptchaConfig } from '../../CaptchaProvider';

export interface SlideRef {
  reset: () => void;
  clear: () => void;
  refresh: () => void;
  close: () => void;
}

export interface SlideProps extends React.HTMLAttributes<HTMLElement> {
  data: SlideData;
  config?: SlideConfig;
  events?: SlideEvent;
}

const Slide = forwardRef<SlideRef, SlideProps>((props, ref) => {
  const { locale } = useCaptchaConfig();
  const [localConfig, setLocalConfig] = useState<SlideConfig>({ ...defaultSlideConfig(), ...(props.config || {}) });
  const [localData, setLocalData] = useState<SlideData>({ ...defaultSlideData(), ...(props.data || {}) });
  const [localEvents, setLocalEvents] = useState<SlideEvent>({ ...(props.events || {}) });

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
  const dragBarRef = useRef<any>(null);
  const containerRef = useRef<any>(null);
  const dragBlockRef = useRef<any>(null);
  const tileRef = useRef<any>(null);

  const handler = useSlideHandler(
    localData, localEvents, localConfig,
    rootRef, containerRef, tileRef, dragBlockRef, dragBarRef,
    () => { setLocalData((prev) => ({ ...prev, ...defaultSlideData() })); },
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
    dragBlockRef.current && dragBlockRef.current.addEventListener('dragstart', fn);
    return () => { dragBlockRef.current && dragBlockRef.current.removeEventListener('dragstart', fn); };
  }, [dragBlockRef]);

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
      <div className="gc-header">
        <span>{localConfig.title || locale.slideTitle}</span>
        <div className="gc-icon-block">
          <CloseIcon width={localConfig.iconSize} height={localConfig.iconSize} onClick={handler.closeEvent} />
          <RefreshIcon width={localConfig.iconSize} height={localConfig.iconSize} onClick={handler.refreshEvent} />
        </div>
      </div>
      <div className="gc-body" ref={containerRef}
        style={{ width: localConfig.width + 'px', height: localConfig.height + 'px' }}>
        <div className="gc-loading"><LoadingIcon /></div>
        <img className={`gc-picture ${localData.image === '' ? 'gc-hide' : ''}`}
          style={{ width: localConfig.width + 'px', height: localConfig.height + 'px', display: hasDisplayImageState ? 'block' : 'none' }}
          src={localData.image} alt="" />
        <div className="gc-tile" ref={tileRef}
          style={{ width: (localData.thumbWidth || 0) + 'px', height: (localData.thumbHeight || 0) + 'px',
            top: (localData.thumbY || 0) + 'px', left: handler.getState().thumbLeft + 'px' }}>
          <img className={localData.thumb === '' ? 'gc-hide' : ''}
            style={{ display: hasDisplayImageState ? 'block' : 'none' }} src={localData.thumb} alt="" />
        </div>
      </div>
      <div className="gc-footer">
        <div className="gc-drag-slide-bar" ref={dragBarRef}>
          <div className="gc-drag-line" />
          <div className={`gc-drag-block ${!hasDisplayImageState ? 'gc-disabled' : ''}`}
            ref={dragBlockRef} onMouseDown={handler.dragEvent}
            style={{ left: handler.getState().dragLeft + 'px' }}>
            <div className="gc-drag-block-inline" onTouchStart={handler.dragEvent}>
              <ArrowsIcon />
            </div>
          </div>
        </div>
      </div>
    </div>
  );
});

Slide.displayName = 'Slide';
export default memo(Slide);
