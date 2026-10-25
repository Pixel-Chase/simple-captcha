import type { CSSProperties } from 'react';
import { forwardRef, memo, useEffect, useImperativeHandle, useState } from 'react';
import type { ClickConfig, ClickData, ClickDot, ClickEvent } from './click.types';
import { defaultClickConfig } from './click.types';
import { LoadingIcon } from '../icons';
import { useClickHandler } from './use-click-handler';
import { useCaptchaConfig } from '../../CaptchaProvider';
import { CardHeader, ResultOverlay } from '../shared/parts';
import { UiArrowIcon, UiCheckCircleIcon, UiCheckIcon, UiRefreshIcon } from '../shared/icons';
import type { StatusProps } from '../shared/status.types';

export interface ClickRef {
  reset: () => void;
  clear: () => void;
  refresh: () => void;
  close: () => void;
}

export interface ClickProps extends React.HTMLAttributes<HTMLElement>, StatusProps {
  data: ClickData;
  config?: ClickConfig;
  events?: ClickEvent;
}

const Click = forwardRef<ClickRef, ClickProps>((props, ref) => {
  const { locale } = useCaptchaConfig();
  const status = props.status || 'idle';
  const [localConfig, setLocalConfig] = useState<ClickConfig>({
    ...defaultClickConfig(),
    ...(props.config || {}),
  });
  const [localData, setLocalData] = useState<ClickData>({ ...(props.data || {}) });
  const [localEvents, setLocalEvents] = useState<ClickEvent>({ ...(props.events || {}) });

  useEffect(() => {
    setLocalConfig((prev) => ({ ...prev, ...(props.config || {}) }));
  }, [props.config]);

  useEffect(() => {
    setLocalData((prev) => ({ ...prev, ...(props.data || {}) }));
  }, [props.data]);

  useEffect(() => {
    setLocalEvents((prev) => ({ ...prev, ...(props.events || {}) }));
  }, [props.events]);

  const handler = useClickHandler(localData, localEvents, () => {
    setLocalData((prev) => ({ ...prev, thumb: '', image: '' }));
  });

  const hPadding = localConfig.horizontalPadding || 0;
  const vPadding = localConfig.verticalPadding || 0;
  const width = (localConfig.width || 0) + hPadding * 2;
  const hasDisplayWrapperState = (localConfig.width || 0) > 0 || (localConfig.height || 0) > 0;
  const hasDisplayImageState =
    localData.image && localData.image.length > 0 && localData.thumb && localData.thumb.length > 0;
  const dots = handler.getDots();

  useImperativeHandle(ref, () => ({
    reset: handler.resetData,
    clear: handler.clearData,
    refresh: handler.refresh,
    close: handler.close,
  }));

  const [countBefore, countAfter = ''] = (locale.clickMeta || '').split('{n}');
  // 按点击顺序把相邻标记连成虚线，坐标与标记中心对齐
  const dotPath = dots.map((dot: ClickDot) => `${dot.x - 1},${dot.y - 1}`).join(' ');

  return (
    <div
      className={`gc-wrapper gc-type-click gc-status-${status} ${localConfig.showTheme ? 'gc-wrapper-theme' : ''}`}
      style={
        {
          width: width + 'px',
          '--gc-px': hPadding + 'px',
          '--gc-py': vPadding + 'px',
          display: hasDisplayWrapperState ? 'flex' : 'none',
        } as CSSProperties
      }
    >
      <CardHeader
        icon="fingerprint"
        title={localConfig.title || locale.clickTitle}
        subtitle={locale.clickSubtitle}
        tag={locale.clickTag}
        status={status}
        onRefresh={handler.refreshEvent}
        onClose={handler.closeEvent}
      />
      <div className="gc-hint-bar" style={{ display: hasDisplayImageState ? 'flex' : 'none' }}>
        <span>{locale.clickHint}</span>
        <img
          className={localData.thumb === '' ? 'gc-hide' : ''}
          style={{ width: localConfig.thumbWidth + 'px', height: localConfig.thumbHeight + 'px' }}
          src={localData.thumb}
          alt=""
        />
      </div>
      <div
        className="gc-body gc-body-click"
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
          onClick={handler.clickEvent}
        />
        <div className="gc-dots">
          {dots.length > 1 && (
            <svg className="gc-dot-path" aria-hidden="true">
              <polyline points={dotPath} />
            </svg>
          )}
          {dots.map((dot: ClickDot) => (
            <div
              className="gc-dot"
              style={{
                width: localConfig.dotSize + 'px',
                height: localConfig.dotSize + 'px',
                borderRadius: localConfig.dotSize + 'px',
                top: dot.y - (localConfig.dotSize || 1) / 2 - 1 + 'px',
                left: dot.x - (localConfig.dotSize || 1) / 2 - 1 + 'px',
              }}
              key={dot.key + '-' + dot.index}
            >
              {dot.index}
            </div>
          ))}
        </div>
        <ResultOverlay status={status} elapsed={props.elapsed} />
      </div>
      <div className="gc-status-row">
        <span className="gc-status-count">
          <UiCheckCircleIcon width={11} height={11} />
          <span>
            {countBefore}
            <strong>{dots.length}</strong>
            {countAfter}
          </span>
        </span>
        <button type="button" className="gc-clear" onClick={handler.resetData}>
          <UiRefreshIcon width={11} height={11} />
          {locale.clickClear}
        </button>
      </div>
      <button
        type="button"
        className={`gc-confirm ${!hasDisplayImageState ? 'gc-disabled' : ''}`}
        onClick={handler.confirmEvent}
      >
        {status === 'success' ? (
          <>
            <span>{locale.doneText}</span>
            <UiCheckIcon width={13} height={13} />
          </>
        ) : (
          <>
            <span>{localConfig.buttonText || locale.confirmButton}</span>
            <UiArrowIcon width={13} height={13} />
          </>
        )}
      </button>
    </div>
  );
});

Click.displayName = 'Click';

export default memo(Click);
