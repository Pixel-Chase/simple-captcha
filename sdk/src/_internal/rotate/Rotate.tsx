/* eslint-disable @typescript-eslint/no-explicit-any */
import { forwardRef, memo, useEffect, useImperativeHandle, useRef, useState } from 'react';
import type { RotateConfig, RotateData, RotateEvent } from './rotate.types';
import { defaultRotateConfig, defaultRotateData } from './rotate.types';
import { CloseIcon, RefreshIcon, LoadingIcon, ArrowsIcon } from '../icons';
import { useRotateHandler } from './use-rotate-handler';
import { useCaptchaConfig } from '../../CaptchaProvider';

export interface RotateRef {
  reset: () => void;
  clear: () => void;
  refresh: () => void;
  close: () => void;
}

export interface RotateProps extends React.HTMLAttributes<HTMLElement> {
  data: RotateData;
  config?: RotateConfig;
  events?: RotateEvent;
}

const Rotate = forwardRef<RotateRef, RotateProps>((props, ref) => {
  const { locale } = useCaptchaConfig();
  const [localConfig, setLocalConfig] = useState<RotateConfig>({ ...defaultRotateConfig(), ...(props.config || {}) });
  const [localData, setLocalData] = useState<RotateData>({ ...defaultRotateData(), ...(props.data || {}) });
  const [localEvents, setLocalEvents] = useState<RotateEvent>({ ...(props.events || {}) });

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
  const dragBlockRef = useRef<any>(null);

  const handler = useRotateHandler(
    localData, localEvents, localConfig,
    rootRef, dragBlockRef, dragBarRef,
    () => { setLocalData((prev) => ({ ...prev, ...defaultRotateData() })); },
  );

  const hPadding = localConfig.horizontalPadding || 0;
  const vPadding = localConfig.verticalPadding || 0;
  const width = (localConfig.width || 0) + (hPadding * 2) + (localConfig.showTheme ? 2 : 0);
  const size = (localConfig.size || 0) > 0 ? localConfig.size : defaultRotateConfig().size;
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
      <div className="gc-body gc-body-rotate"
        style={{ width: localConfig.width + 'px', height: localConfig.height + 'px' }}>
        <div className="gc-body-inner gc-body-inner-rotate"
          style={{ width: size + 'px', height: size + 'px' }}>
          <div className="gc-loading"><LoadingIcon /></div>
          <div className="gc-picture-rotate"
            style={{ width: localConfig.size + 'px', height: localConfig.size + 'px' }}>
            <img className={localData.image === '' ? 'gc-hide' : ''}
              src={localData.image} style={{ display: hasDisplayImageState ? 'block' : 'none' }} alt="" />
            <div className="gc-round" />
          </div>
          <div className="gc-thumb">
            <div className="gc-thumb-block"
              style={{
                transform: `rotate(${handler.getState().thumbAngle}deg)`,
                ...(localData.thumbSize > 0 ? { width: localData.thumbSize + 'px', height: localData.thumbSize + 'px' } : {}),
              }}>
              <img className={localData.thumb === '' ? 'gc-hide' : ''}
                src={localData.thumb} style={{ visibility: hasDisplayImageState ? 'visible' : 'hidden' }} alt="" />
            </div>
          </div>
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

Rotate.displayName = 'Rotate';
export default memo(Rotate);
