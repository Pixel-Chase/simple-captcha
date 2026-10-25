/* eslint-disable @typescript-eslint/no-explicit-any */
import type { CSSProperties } from 'react';
import { forwardRef, memo, useEffect, useImperativeHandle, useRef, useState } from 'react';
import type { SlideConfig, SlideData, SlideEvent } from './slide.types';
import { defaultSlideConfig, defaultSlideData } from './slide.types';
import { LoadingIcon } from '../icons';
import { useSlideHandler } from './use-slide-handler';
import { useCaptchaConfig } from '../../CaptchaProvider';
import { CardHeader, DragTrack, ResultOverlay } from '../shared/parts';
import type { StatusProps } from '../shared/status.types';

export interface SlideRef {
  reset: () => void;
  clear: () => void;
  refresh: () => void;
  close: () => void;
}

export interface SlideProps extends React.HTMLAttributes<HTMLElement>, StatusProps {
  data: SlideData;
  config?: SlideConfig;
  events?: SlideEvent;
}

const Slide = forwardRef<SlideRef, SlideProps>((props, ref) => {
  const { locale } = useCaptchaConfig();
  const status = props.status || 'idle';
  const [localConfig, setLocalConfig] = useState<SlideConfig>({
    ...defaultSlideConfig(),
    ...(props.config || {}),
  });
  const [localData, setLocalData] = useState<SlideData>({
    ...defaultSlideData(),
    ...(props.data || {}),
  });
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
    localData,
    localEvents,
    localConfig,
    rootRef,
    containerRef,
    tileRef,
    dragBlockRef,
    dragBarRef,
    () => {
      setLocalData((prev) => ({ ...prev, ...defaultSlideData() }));
    },
  );

  const hPadding = localConfig.horizontalPadding || 0;
  const vPadding = localConfig.verticalPadding || 0;
  const width = (localConfig.width || 0) + hPadding * 2;
  const hasDisplayWrapperState = (localConfig.width || 0) > 0 || (localConfig.height || 0) > 0;
  const hasDisplayImageState =
    localData.image && localData.image.length > 0 && localData.thumb && localData.thumb.length > 0;

  useImperativeHandle(ref, () => ({
    reset: handler.resetData,
    clear: handler.clearData,
    refresh: handler.refresh,
    close: handler.close,
  }));

  useEffect(() => {
    const fn = (event: Event) => event.preventDefault();
    dragBlockRef.current && dragBlockRef.current.addEventListener('dragstart', fn);
    return () => {
      dragBlockRef.current && dragBlockRef.current.removeEventListener('dragstart', fn);
    };
  }, [dragBlockRef]);

  return (
    <div
      className={`gc-wrapper gc-type-slide gc-status-${status} ${localConfig.showTheme ? 'gc-wrapper-theme' : ''}`}
      style={
        {
          width: width + 'px',
          '--gc-px': hPadding + 'px',
          '--gc-py': vPadding + 'px',
          display: hasDisplayWrapperState ? 'flex' : 'none',
        } as CSSProperties
      }
      ref={rootRef}
    >
      <CardHeader
        icon="puzzle"
        title={localConfig.title || locale.slideTitle}
        subtitle={locale.slideSubtitle}
        status={status}
        onRefresh={handler.refreshEvent}
        onClose={handler.closeEvent}
      />
      <div
        className="gc-body"
        ref={containerRef}
        style={{ width: localConfig.width + 'px', height: localConfig.height + 'px' }}
      >
        <div className="gc-loading">
          <LoadingIcon />
        </div>
        <img
          className={`gc-picture ${localData.image === '' ? 'gc-hide' : ''}`}
          style={{
            width: localConfig.width + 'px',
            height: localConfig.height + 'px',
            display: hasDisplayImageState ? 'block' : 'none',
          }}
          src={localData.image}
          alt=""
        />
        <div
          className="gc-tile"
          ref={tileRef}
          style={{
            width: (localData.thumbWidth || 0) + 'px',
            height: (localData.thumbHeight || 0) + 'px',
            top: (localData.thumbY || 0) + 'px',
            left: handler.getState().thumbLeft + 'px',
          }}
        >
          <img
            className={localData.thumb === '' ? 'gc-hide' : ''}
            style={{ display: hasDisplayImageState ? 'block' : 'none' }}
            src={localData.thumb}
            alt=""
          />
        </div>
        <ResultOverlay status={status} elapsed={props.elapsed} />
      </div>
      <DragTrack
        barRef={dragBarRef}
        blockRef={dragBlockRef}
        dragLeft={handler.getState().dragLeft}
        disabled={!hasDisplayImageState}
        hint={locale.slideHint}
        status={status}
        onDragStart={handler.dragEvent}
        metaLeft={locale.slideProgress}
        showPercent
      />
    </div>
  );
});

Slide.displayName = 'Slide';
export default memo(Slide);
