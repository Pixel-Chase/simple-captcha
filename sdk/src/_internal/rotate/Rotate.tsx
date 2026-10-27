/* eslint-disable @typescript-eslint/no-explicit-any */
import type { CSSProperties } from 'react';
import { forwardRef, memo, useEffect, useImperativeHandle, useRef, useState } from 'react';
import type { RotateConfig, RotateData, RotateEvent } from './rotate.types';
import { defaultRotateConfig, defaultRotateData } from './rotate.types';
import { LoadingIcon } from '../icons';
import { useRotateHandler } from './use-rotate-handler';
import { useCaptchaConfig } from '../../CaptchaProvider';
import { CardHeader, DragTrack, ResultOverlay, SecureNote } from '../shared/parts';
import type { StatusProps } from '../shared/status.types';

export interface RotateRef {
  reset: () => void;
  clear: () => void;
  refresh: () => void;
  close: () => void;
}

/** 虚线刻度环比图片圆大出的直径（两侧各 16px） */
const RING_GAP = 32;
/** 最外圈实线刻度环比图片圆大出的直径（两侧各 24px） */
const OUTER_RING_GAP = 48;
/** 旋转区域上下留白合计 */
const ROTATE_PAD = 20;

export interface RotateProps extends React.HTMLAttributes<HTMLElement>, StatusProps {
  data: RotateData;
  config?: RotateConfig;
  events?: RotateEvent;
}

const Rotate = forwardRef<RotateRef, RotateProps>((props, ref) => {
  const { locale } = useCaptchaConfig();
  const status = props.status || 'idle';
  const [localConfig, setLocalConfig] = useState<RotateConfig>({
    ...defaultRotateConfig(),
    ...(props.config || {}),
  });
  const [localData, setLocalData] = useState<RotateData>({
    ...defaultRotateData(),
    ...(props.data || {}),
  });
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
    localData,
    localEvents,
    localConfig,
    rootRef,
    dragBlockRef,
    dragBarRef,
    () => {
      setLocalData((prev) => ({ ...prev, ...defaultRotateData() }));
    },
  );

  const hPadding = localConfig.horizontalPadding || 0;
  const vPadding = localConfig.verticalPadding || 0;
  const width = (localConfig.width || 0) + hPadding * 2;
  const size = (localConfig.size || 0) > 0 ? localConfig.size : defaultRotateConfig().size;
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

  const angle = handler.getState().thumbAngle;
  const ringSize = (size || 0) + RING_GAP;
  const ringRadius = ringSize / 2;
  const ringLength = 2 * Math.PI * ringRadius;
  const outerRingSize = (size || 0) + OUTER_RING_GAP;
  const angleText = `${Math.round(angle)}`;

  return (
    <div
      className={`gc-wrapper gc-type-rotate gc-status-${status} ${localConfig.showTheme ? 'gc-wrapper-theme' : ''}`}
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
        icon="rotate"
        title={locale.rotateTitle}
        subtitle={locale.rotateSubtitle}
        tag={locale.rotateTag}
        status={status}
        onRefresh={handler.refreshEvent}
        onClose={handler.closeEvent}
      />
      <div
        className="gc-body gc-body-rotate"
        style={{ width: localConfig.width + 'px', height: ringSize + ROTATE_PAD + 'px' }}
      >
        <div className="gc-dial" style={{ width: ringSize + 'px', height: ringSize + 'px' }}>
          {/* 虚线环随角度整体转动（起点即当前角度），已转过的 0°→angle 段用实线弧覆盖 */}
          <svg className="gc-dial-track" viewBox={`0 0 ${ringSize} ${ringSize}`} aria-hidden="true">
            <circle
              className="gc-dial-track-dash"
              cx={ringRadius}
              cy={ringRadius}
              r={ringRadius}
              transform={`rotate(${angle - 90} ${ringRadius} ${ringRadius})`}
            />
            <circle
              className="gc-dial-track-done"
              cx={ringRadius}
              cy={ringRadius}
              r={ringRadius}
              strokeDasharray={`${(ringLength * angle) / 360} ${ringLength}`}
              transform={`rotate(-90 ${ringRadius} ${ringRadius})`}
            />
          </svg>
          <i
            className="gc-dial-ring"
            style={{ width: outerRingSize + 'px', height: outerRingSize + 'px' }}
          />
          <i
            className={`gc-dial-dot gc-dial-dot-right ${angle >= 90 ? 'gc-dial-dot-passed' : ''}`}
          />
          <i
            className={`gc-dial-dot gc-dial-dot-bottom ${angle >= 180 ? 'gc-dial-dot-passed' : ''}`}
          />
          <i
            className={`gc-dial-dot gc-dial-dot-left ${angle >= 270 ? 'gc-dial-dot-passed' : ''}`}
          />
          <i className="gc-dial-pointer" style={{ transform: `rotate(${angle}deg)` }}>
            <i className="gc-dial-dot gc-dial-dot-top" />
          </i>
          <div
            className="gc-body-inner gc-body-inner-rotate"
            style={{ width: size + 'px', height: size + 'px' }}
          >
            <div className="gc-loading">
              <LoadingIcon />
            </div>
            <div
              className="gc-picture-rotate"
              style={{ width: localConfig.size + 'px', height: localConfig.size + 'px' }}
            >
              <img
                className={localData.image === '' ? 'gc-hide' : ''}
                src={localData.image}
                style={{ display: hasDisplayImageState ? 'block' : 'none' }}
                alt=""
              />
            </div>
            <div className="gc-thumb">
              <div
                className="gc-thumb-block"
                style={{
                  transform: `rotate(${angle}deg)`,
                  ...(localData.thumbSize > 0
                    ? { width: localData.thumbSize + 'px', height: localData.thumbSize + 'px' }
                    : {}),
                }}
              >
                <img
                  className={localData.thumb === '' ? 'gc-hide' : ''}
                  src={localData.thumb}
                  style={{ visibility: hasDisplayImageState ? 'visible' : 'hidden' }}
                  alt=""
                />
              </div>
            </div>
            <ResultOverlay status={status} elapsed={props.elapsed} compact />
          </div>
          <i className="gc-dial-cross" />
        </div>
        {hasDisplayImageState && status === 'idle' && (
          <div className="gc-angle-badge gc-glass">
            {locale.rotateAngle}
            <strong>
              {angle >= 0 ? '+' : ''}
              {angleText}°
            </strong>
          </div>
        )}
      </div>
      <DragTrack
        barRef={dragBarRef}
        blockRef={dragBlockRef}
        dragLeft={handler.getState().dragLeft}
        disabled={!hasDisplayImageState}
        hint={locale.rotateHint}
        status={status}
        handleIcon="sync"
        onDragStart={handler.dragEvent}
        metaLeft={`${angleText}° / 360°`}
        metaRight={locale.rotateProgress}
      />
      <SecureNote icon="notch">{locale.rotateNote}</SecureNote>
    </div>
  );
});

Rotate.displayName = 'Rotate';
export default memo(Rotate);
