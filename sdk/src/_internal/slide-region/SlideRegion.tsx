/* eslint-disable @typescript-eslint/no-explicit-any */
import type { CSSProperties } from 'react';
import { forwardRef, memo, useEffect, useImperativeHandle, useRef, useState } from 'react';
import type { SlideRegionConfig, SlideRegionData, SlideRegionEvent } from './slide-region.types';
import { defaultSlideRegionConfig, defaultSlideRegionData } from './slide-region.types';
import { LoadingIcon } from '../icons';
import { useSlideRegionHandler } from './use-slide-region-handler';
import { useCaptchaConfig } from '../../CaptchaProvider';
import { CardHeader, ResultOverlay, SecureNote } from '../shared/parts';
import { UiFingerprintIcon, UiPointerIcon, UiSyncIcon } from '../shared/icons';
import type { StatusProps } from '../shared/status.types';

export interface SlideRegionRef {
  reset: () => void;
  clear: () => void;
  refresh: () => void;
  close: () => void;
}

export interface SlideRegionProps extends React.HTMLAttributes<HTMLElement>, StatusProps {
  data: SlideRegionData;
  config?: SlideRegionConfig;
  events?: SlideRegionEvent;
}

const SlideRegion = forwardRef<SlideRegionRef, SlideRegionProps>((props, ref) => {
  const { locale } = useCaptchaConfig();
  const status = props.status || 'idle';
  const [localConfig, setLocalConfig] = useState<SlideRegionConfig>({
    ...defaultSlideRegionConfig(),
    ...(props.config || {}),
  });
  const [localData, setLocalData] = useState<SlideRegionData>({
    ...defaultSlideRegionData(),
    ...(props.data || {}),
  });
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
    localData,
    localEvents,
    localConfig,
    rootRef,
    containerRef,
    tileRef,
    () => {
      setLocalData((prev) => ({ ...prev, ...defaultSlideRegionData() }));
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
    tileRef.current && tileRef.current.addEventListener('dragstart', fn);
    return () => {
      tileRef.current && tileRef.current.removeEventListener('dragstart', fn);
    };
  }, [tileRef]);

  // 图块相对初始位置的位移，仅反映用户自己的拖动，不涉及缺口位置
  const formatOffset = (value: number) => `${value >= 0 ? '+' : ''}${Math.round(value)}px`;
  const offsetX = formatOffset(handler.thumbPoint.x - (localData.thumbX || 0));
  const offsetY = formatOffset(handler.thumbPoint.y - (localData.thumbY || 0));

  return (
    <div
      className={`gc-wrapper gc-type-region gc-status-${status} ${localConfig.showTheme ? 'gc-wrapper-theme' : ''}`}
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
        icon="move"
        title={localConfig.title || locale.regionTitle}
        subtitle={locale.regionSubtitle}
        tag={locale.regionTag}
        tagTone="success"
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
          src={localData.image}
          style={{
            width: localConfig.width + 'px',
            height: localConfig.height + 'px',
            display: hasDisplayImageState ? 'block' : 'none',
          }}
          alt=""
        />
        <svg className="gc-region-grid" aria-hidden="true">
          <line x1="0" x2="100%" y1="50%" y2="50%" />
          <line x1="50%" x2="50%" y1="0" y2="100%" />
        </svg>
        {hasDisplayImageState && status === 'idle' && (
          <div className="gc-region-offset gc-glass">
            <span>
              ΔX: <strong>{offsetX}</strong>
            </span>
            <span>
              ΔY: <strong>{offsetY}</strong>
            </span>
          </div>
        )}
        <div
          className="gc-tile gc-tile-region"
          ref={tileRef}
          style={{
            width: (localData.thumbWidth || 0) + 'px',
            height: (localData.thumbHeight || 0) + 'px',
            top: handler.thumbPoint.y + 'px',
            left: handler.thumbPoint.x + 'px',
          }}
          onMouseDown={handler.dragEvent}
          onTouchStart={handler.dragEvent}
        >
          <img
            className={localData.thumb === '' ? 'gc-hide' : ''}
            style={{ display: hasDisplayImageState ? 'block' : 'none' }}
            src={localData.thumb}
            alt=""
          />
          {hasDisplayImageState && (
            <span className="gc-tile-grip">
              <UiPointerIcon width={14} height={14} />
            </span>
          )}
        </div>
        <ResultOverlay status={status} elapsed={props.elapsed} />
      </div>
      {/* 放在图片下方而非叠在图上，避免遮挡停靠在底部的图块；结果态仅隐藏不卸载，保持卡片高度不跳动 */}
      <div className="gc-region-tip">
        <span>
          <UiFingerprintIcon width={12} height={12} />
          {locale.regionHint}
        </span>
        <em>{locale.regionBadge}</em>
      </div>
      <div className="gc-region-actions">
        <button type="button" onClick={handler.resetData}>
          <UiSyncIcon width={11} height={11} />
          {locale.regionReset}
        </button>
      </div>
      <SecureNote>{locale.poweredBy}</SecureNote>
    </div>
  );
});

SlideRegion.displayName = 'SlideRegion';
export default memo(SlideRegion);
