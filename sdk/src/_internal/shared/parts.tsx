/* eslint-disable @typescript-eslint/no-explicit-any */
import type { MouseEvent, ReactNode, RefObject } from 'react';
import { useCaptchaConfig } from '../../CaptchaProvider';
import {
  UiCheckIcon,
  UiCloseIcon,
  UiDoubleChevronIcon,
  UiFingerprintIcon,
  UiMoveIcon,
  UiPuzzleIcon,
  UiRefreshIcon,
  UiSyncIcon,
} from './icons';
import type { CaptchaStatus } from './status.types';

const HEADER_ICONS = {
  fingerprint: UiFingerprintIcon,
  puzzle: UiPuzzleIcon,
  move: UiMoveIcon,
  rotate: UiSyncIcon,
} as const;

export interface CardHeaderProps {
  title?: string;
  subtitle?: string;
  /** 标题左侧图标瓷砖里的图标，随题型变化 */
  icon: keyof typeof HEADER_ICONS;
  status?: CaptchaStatus;
  onRefresh?: (e: MouseEvent) => void;
  onClose?: (e: MouseEvent) => void;
}

export const CardHeader = ({
  title,
  subtitle,
  icon,
  status = 'idle',
  onRefresh,
  onClose,
}: CardHeaderProps) => {
  const { locale } = useCaptchaConfig();
  const failed = status === 'fail';
  const Icon = HEADER_ICONS[icon];

  return (
    <div className={`gc-header ${failed ? 'gc-header-fail' : ''}`}>
      <div className="gc-title">
        <span className="gc-title-tile">
          <Icon />
        </span>
        <div className="gc-title-text">
          <h3 className="gc-title-main">{title}</h3>
          <p className="gc-subtitle">{failed ? locale.failTitle : subtitle}</p>
        </div>
      </div>
      <div className="gc-icon-block">
        {onRefresh && (
          <button
            type="button"
            className="gc-btn-refresh"
            aria-label={locale.refreshLabel}
            onClick={onRefresh}
          >
            <UiRefreshIcon />
          </button>
        )}
        {onClose && (
          <button
            type="button"
            className="gc-btn-close"
            aria-label={locale.closeLabel}
            onClick={onClose}
          >
            <UiCloseIcon />
          </button>
        )}
      </div>
    </div>
  );
};

export interface ResultOverlayProps {
  status?: CaptchaStatus;
  elapsed?: number;
}

/** 覆盖在图片区域上的校验结果：成功为绿色遮罩 + 对勾，失败为底部红色提示条 */
export const ResultOverlay = ({ status = 'idle', elapsed }: ResultOverlayProps) => {
  const { locale } = useCaptchaConfig();

  if (status === 'success') {
    const elapsedText =
      typeof elapsed === 'number'
        ? ` · ${(locale.elapsedText || '').replace('{s}', elapsed.toFixed(1))}`
        : '';
    return (
      <div className="gc-result gc-result-success">
        <span className="gc-result-badge">
          <UiCheckIcon width={24} height={24} />
        </span>
        <span className="gc-result-text">
          {locale.successText}
          {elapsedText}
        </span>
      </div>
    );
  }

  if (status === 'fail') {
    return (
      <div className="gc-result gc-result-fail">
        <div className="gc-result-banner">
          <UiRefreshIcon width={14} height={14} />
          <span>{locale.failBanner}</span>
        </div>
      </div>
    );
  }

  return null;
};

export interface DragTrackProps {
  barRef: RefObject<any>;
  blockRef: RefObject<any>;
  dragLeft: number;
  disabled: boolean;
  /** 待拖动时轨道内的提示 */
  hint?: string;
  status?: CaptchaStatus;
  onDragStart: (e: any) => void;
  /** 把手图标：滑块为双箭头，旋转为循环箭头 */
  handleIcon?: 'chevrons' | 'sync';
  /** 轨道下方的说明行：左侧文案，右侧为拖动进度百分比或自定义文本 */
  metaLeft?: string;
  metaRight?: string;
  showPercent?: boolean;
}

export const DragTrack = ({
  barRef,
  blockRef,
  dragLeft,
  disabled,
  hint,
  status = 'idle',
  onDragStart,
  handleIcon = 'chevrons',
  metaLeft,
  metaRight,
  showPercent = false,
}: DragTrackProps) => {
  const { locale } = useCaptchaConfig();
  const done = status !== 'idle';
  const Icon = handleIcon === 'sync' ? UiSyncIcon : UiDoubleChevronIcon;

  let right = metaRight;
  if (showPercent) {
    const max = (barRef.current?.offsetWidth ?? 0) - (blockRef.current?.offsetWidth ?? 0);
    const percent = max > 0 ? Math.min(100, Math.round((dragLeft / max) * 100)) : 0;
    right = `${percent}%`;
  }

  return (
    <div className="gc-track">
      <div className="gc-footer">
        <div className="gc-drag-slide-bar" ref={barRef}>
          <div className="gc-drag-line">
            {!done && <div className="gc-drag-fill" style={{ width: dragLeft + 'px' }} />}
            {!done && (
              <span className={`gc-drag-hint ${dragLeft > 4 ? 'gc-drag-hint-hidden' : ''}`}>
                {hint}
              </span>
            )}
            {done && (
              <span className="gc-drag-result">
                <span>{status === 'success' ? locale.trackSuccess : locale.trackFail}</span>
                <i className="gc-drag-result-icon">
                  {status === 'success' ? (
                    <UiCheckIcon width={18} height={18} />
                  ) : (
                    <UiCloseIcon width={16} height={16} />
                  )}
                </i>
              </span>
            )}
          </div>
          <div
            className={`gc-drag-block ${disabled ? 'gc-disabled' : ''} ${done ? 'gc-hide-block' : ''}`}
            ref={blockRef}
            onMouseDown={onDragStart}
            style={{ left: dragLeft + 'px' }}
          >
            <div
              className={`gc-drag-block-inline ${dragLeft <= 4 ? 'gc-drag-idle' : ''}`}
              onTouchStart={onDragStart}
            >
              <Icon width={14} height={14} />
            </div>
          </div>
        </div>
      </div>
      {(metaLeft || right) && (
        <div className="gc-track-meta">
          <span>{metaLeft}</span>
          <span className="gc-track-meta-value">{right}</span>
        </div>
      )}
    </div>
  );
};

export const SecureNote = ({ children }: { children: ReactNode }) => (
  <div className="gc-secure-note">
    <svg
      aria-hidden="true"
      viewBox="0 0 24 24"
      width={10}
      height={10}
      fill="currentColor"
      className="gc-secure-icon"
    >
      <path d="M12 1 3 5v6c0 5.55 3.84 10.74 9 12 5.16-1.26 9-6.45 9-12V5l-9-4z" />
    </svg>
    <span>{children}</span>
  </div>
);
