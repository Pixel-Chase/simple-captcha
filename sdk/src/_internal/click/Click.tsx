import { forwardRef, memo, useEffect, useImperativeHandle, useState } from 'react';
import type { ClickConfig, ClickData, ClickDot, ClickEvent } from './click.types';
import { defaultClickConfig } from './click.types';
import { CloseIcon, RefreshIcon, LoadingIcon } from '../icons';
import { useClickHandler } from './use-click-handler';
import { useCaptchaConfig } from '../../CaptchaProvider';

export interface ClickRef {
  reset: () => void;
  clear: () => void;
  refresh: () => void;
  close: () => void;
}

export interface ClickProps extends React.HTMLAttributes<HTMLElement> {
  data: ClickData;
  config?: ClickConfig;
  events?: ClickEvent;
}

const Click = forwardRef<ClickRef, ClickProps>((props, ref) => {
  const { locale } = useCaptchaConfig();
  const [localConfig, setLocalConfig] = useState<ClickConfig>({ ...defaultClickConfig(), ...(props.config || {}) });
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
  const width = (localConfig.width || 0) + (hPadding * 2) + (localConfig.showTheme ? 2 : 0);
  const hasDisplayWrapperState = (localConfig.width || 0) > 0 || (localConfig.height || 0) > 0;
  const hasDisplayImageState = (localData.image && localData.image.length > 0) && (localData.thumb && localData.thumb.length > 0);

  useImperativeHandle(ref, () => ({
    reset: handler.resetData,
    clear: handler.clearData,
    refresh: handler.refresh,
    close: handler.close,
  }));

  return (
    <div
      className={`gc-wrapper ${localConfig.showTheme ? 'gc-wrapper-theme' : ''}`}
      style={{
        width: width + 'px',
        paddingLeft: hPadding + 'px',
        paddingRight: hPadding + 'px',
        paddingTop: vPadding + 'px',
        paddingBottom: vPadding + 'px',
        display: hasDisplayWrapperState ? 'block' : 'none',
      }}
    >
      <div className="gc-header">
        <span>{localConfig.title || locale.clickTitle}</span>
        <img
          className={localData.thumb === '' ? 'gc-hide' : ''}
          style={{
            width: localConfig.thumbWidth + 'px',
            height: localConfig.thumbHeight + 'px',
            display: hasDisplayImageState ? 'block' : 'none',
          }}
          src={localData.thumb}
          alt=""
        />
      </div>
      <div
        className="gc-body"
        style={{
          width: localConfig.width + 'px',
          height: localConfig.height + 'px',
        }}
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
          {handler.getDots().map((dot: ClickDot) => (
            <div
              className="gc-dot"
              style={{
                width: localConfig.dotSize + 'px',
                height: localConfig.dotSize + 'px',
                borderRadius: localConfig.dotSize + 'px',
                top: (dot.y - ((localConfig.dotSize || 1) / 2) - 1) + 'px',
                left: (dot.x - ((localConfig.dotSize || 1) / 2) - 1) + 'px',
              }}
              key={dot.key + '-' + dot.index}
            >
              {dot.index}
            </div>
          ))}
        </div>
      </div>
      <div className="gc-footer">
        <div className="gc-icon-block">
          <CloseIcon
            width={localConfig.iconSize}
            height={localConfig.iconSize}
            onClick={handler.closeEvent}
          />
          <RefreshIcon
            width={localConfig.iconSize}
            height={localConfig.iconSize}
            onClick={handler.refreshEvent}
          />
        </div>
        <div className="gc-button-block">
          <button
            className={!hasDisplayImageState ? 'gc-disabled' : ''}
            onClick={handler.confirmEvent}
          >
            {localConfig.buttonText || locale.confirmButton}
          </button>
        </div>
      </div>
    </div>
  );
});

Click.displayName = 'Click';

export default memo(Click);
