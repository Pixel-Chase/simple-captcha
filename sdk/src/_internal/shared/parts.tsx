/* eslint-disable @typescript-eslint/no-explicit-any */
import type { MouseEvent, ReactNode, RefObject } from 'react';
import { useCaptchaConfig } from '../../CaptchaProvider';
import {
  UiCheckIcon,
  UiCloseIcon,
  UiDoubleChevronIcon,
  UiFingerprintIcon,
  UiMoveIcon,
  UiNotchIcon,
  UiPuzzleIcon,
  UiRefreshIcon,
  UiShieldIcon,
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
  /** 标题右侧的小标签，空字符串不显示 */
  tag?: string;
  /** 标签配色：默认跟随主题色，success 为绿色 */
  tagTone?: 'primary' | 'success';
  /** 标题左侧图标瓷砖里的图标，随题型变化 */
  icon: keyof typeof HEADER_ICONS;
  status?: CaptchaStatus;
  onRefresh?: (e: MouseEvent) => void;
  onClose?: (e: MouseEvent) => void;
}

export const CardHeader = ({
  title,
  subtitle,
  tag,
  tagTone = 'primary',
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
          <div className="gc-title-row">
            <h3 className="gc-title-main">{title}</h3>
            {tag && (
              <span
                className={`gc-title-tag ${tagTone === 'success' ? 'gc-title-tag-success' : ''}`}
              >
                {tag}
              </span>
            )}
          </div>
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
  /** 轨道下方的说明行：左侧为辅助说明，右侧为主题色的强调文本 */
  metaLeft?: ReactNode;
  metaRight?: ReactNode;
  /** 提供时右侧显示真实拖动进度，{p} 为百分比数值 */
  percentText?: string;
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
  percentText,
}: DragTrackProps) => {
  const { locale } = useCaptchaConfig();
  const done = status !== 'idle';
  const Icon = handleIcon === 'sync' ? UiSyncIcon : UiDoubleChevronIcon;

  let right = metaRight;
  if (percentText) {
    const max = (barRef.current?.offsetWidth ?? 0) - (blockRef.current?.offsetWidth ?? 0);
    const percent = max > 0 ? Math.min(100, Math.round((dragLeft / max) * 100)) : 0;
    right = percentText.replace('{p}', `${percent}`);
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

export interface SecureNoteProps {
  children?: ReactNode;
  /** 盾牌用于滑块 / 空间拼图，缺口圆环用于旋转 */
  icon?: 'shield' | 'notch';
}

/** 卡片底部的安全提示，文案为空时不渲染 */
export const SecureNote = ({ children, icon = 'shield' }: SecureNoteProps) => {
  if (!children) {
    return null;
  }
  const Icon = icon === 'notch' ? UiNotchIcon : UiShieldIcon;

  return (
    <div className="gc-secure-note">
      <Icon width={10} height={10} />
      <span>{children}</span>
    </div>
  );
};
