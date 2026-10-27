import { useEffect, useId, useRef } from 'react';
import type { ReactNode } from 'react';
import { createPortal } from 'react-dom';
import { Icon } from './Icon';
import type { IconName } from './Icon';

interface ModalProps {
  open: boolean;
  onClose: () => void;
  /** dialog 居中弹窗；drawer 右侧抽屉 */
  variant?: 'dialog' | 'drawer';
  /** 弹窗宽度：sm 448px、md 560px、lg 720px（抽屉固定 512px） */
  size?: 'sm' | 'md' | 'lg';
  title: ReactNode;
  /** 标题下的一行说明 */
  subtitle?: ReactNode;
  /** 标题左侧的图标与色调 */
  icon?: IconName;
  tone?: 'primary' | 'danger' | 'success';
  /** false 时点遮罩、按 Esc、右上角关闭都不生效（例如一次性密钥必须点「我已保存」） */
  dismissible?: boolean;
  /** 底部操作区 */
  footer?: ReactNode;
  children?: ReactNode;
}

/** 控制台通用弹窗 / 抽屉：挂到 body，打开时锁定页面滚动 */
export function Modal({
  open,
  onClose,
  variant = 'dialog',
  size = 'md',
  title,
  subtitle,
  icon,
  tone = 'primary',
  dismissible = true,
  footer,
  children,
}: ModalProps) {
  const titleId = useId();
  const panelRef = useRef<HTMLDivElement>(null);
  // onClose 往往是内联函数，放进 ref 里，避免每次渲染都重新绑定键盘事件
  const closeRef = useRef(onClose);
  closeRef.current = onClose;

  useEffect(() => {
    if (!open) {
      return;
    }
    const previous = document.activeElement as HTMLElement | null;
    panelRef.current?.focus();
    const { overflow } = document.body.style;
    document.body.style.overflow = 'hidden';

    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape' && dismissible) {
        event.stopPropagation();
        closeRef.current();
      }
    };
    document.addEventListener('keydown', onKeyDown);
    return () => {
      document.removeEventListener('keydown', onKeyDown);
      document.body.style.overflow = overflow;
      previous?.focus?.();
    };
  }, [open, dismissible]);

  if (!open) {
    return null;
  }

  return createPortal(
    <div
      className={`modal-backdrop modal-backdrop--${variant}`}
      onMouseDown={(event) => {
        if (dismissible && event.target === event.currentTarget) {
          onClose();
        }
      }}
    >
      <div
        ref={panelRef}
        className={variant === 'drawer' ? 'modal modal--drawer' : `modal modal--${size}`}
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleId}
        tabIndex={-1}
      >
        <div className="modal__head">
          {icon && (
            <span className={`modal__icon modal__icon--${tone}`}>
              <Icon name={icon} size={20} />
            </span>
          )}
          <div className="modal__heading">
            <h2 id={titleId} className="modal__title">
              {title}
            </h2>
            {subtitle && <p className="modal__subtitle">{subtitle}</p>}
          </div>
          {dismissible && (
            <button type="button" className="modal__close" onClick={onClose} aria-label="关闭">
              <Icon name="close" size={20} />
            </button>
          )}
        </div>
        <div className="modal__body">{children}</div>
        {footer && <div className="modal__foot">{footer}</div>}
      </div>
    </div>,
    document.body,
  );
}
