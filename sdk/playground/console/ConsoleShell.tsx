import { useEffect, useRef, useState } from 'react';
import type { ReactNode } from 'react';
import { formatDateTime } from '../api';
import { Icon } from '../components/Icon';
import { ToastProvider } from '../components/Toast';
import { NAV_SECTIONS } from './ConsoleView';
import type { ConsoleSection } from './ConsoleView';
import type { AuthState } from './useConsoleAuth';

interface ConsoleShellProps {
  auth: AuthState;
  /** 当前分区；不在控制台（演示站的接入文档页）时为 null */
  section: ConsoleSection | null;
  /** 分区链接：演示站为 #/console/apps，内嵌管理后台为 #/apps */
  hrefFor: (section: ConsoleSection) => string;
  onLogout: () => void;
  /** 演示站顶栏右侧的「接入文档」入口；内嵌管理后台没有文档页 */
  docs?: { href: string; active: boolean };
  /** 主区域的额外类名，例如接入文档页更宽 */
  mainClassName?: string;
  children: ReactNode;
}

/** 控制台外壳：顶栏（品牌、分区导航、管理员菜单）+ 主区域 + 页脚；未登录时只有品牌与页脚说明 */
export function ConsoleShell({
  auth,
  section,
  hrefFor,
  onLogout,
  docs,
  mainClassName,
  children,
}: ConsoleShellProps) {
  const authed = auth.status === 'authed' ? auth : null;

  // 切换分区、登录或退出时页面内容整体换掉，但 hash 路由不会重置滚动位置，手动回到顶部；首次渲染不处理
  const view = `${auth.status}/${section ?? ''}`;
  const lastView = useRef(view);
  useEffect(() => {
    if (lastView.current !== view) {
      lastView.current = view;
      window.scrollTo(0, 0);
    }
  }, [view]);

  return (
    <ToastProvider>
      <div className={authed ? 'app' : 'app app--guest'}>
        <header className="topbar">
          <div className="topbar__inner">
            <div className="topbar__start">
              <a className="brand" href={hrefFor('overview')}>
                <span className="brand__logo" aria-hidden="true">
                  <Icon name="shield-check" size={authed ? 16 : 20} />
                </span>
                <span className="brand__name">simple-captcha</span>
              </a>
              <span className="brand__chip">自托管服务</span>

              {authed && (
                <nav className="nav" aria-label="控制台">
                  {NAV_SECTIONS.map((item) => (
                    <a
                      key={item.id}
                      href={hrefFor(item.id)}
                      className={section === item.id ? 'nav__link nav__link--active' : 'nav__link'}
                      aria-current={section === item.id ? 'page' : undefined}
                    >
                      {item.label}
                    </a>
                  ))}
                </nav>
              )}
            </div>

            <div className="topbar__end">
              {docs && (
                <a
                  className={docs.active ? 'topbar__link topbar__link--active' : 'topbar__link'}
                  href={docs.href}
                  aria-current={docs.active ? 'page' : undefined}
                >
                  <Icon name="book" size={16} />
                  <span className="topbar__link-text">接入文档</span>
                </a>
              )}
              {authed && (
                <>
                  {docs && <span className="topbar__divider" aria-hidden="true" />}
                  <UserMenu
                    username={authed.username}
                    expiresAt={authed.expiresAt}
                    accountHref={hrefFor('account')}
                    onLogout={onLogout}
                  />
                </>
              )}
              {!authed && docs?.active && (
                <a className="btn btn--primary btn--sm" href={hrefFor('overview')}>
                  登录控制台
                </a>
              )}
            </div>
          </div>
        </header>

        <main className={mainClassName ? `main ${mainClassName}` : 'main'}>{children}</main>

        <footer className="footer">
          <div className="footer__inner">
            {authed ? (
              <>
                <span>simple-captcha · 自托管行为验证码服务</span>
                <div className="footer__links">
                  <a href={hrefFor('account')}>账号与安全</a>
                  <span className="footer__sep" aria-hidden="true" />
                  <button type="button" onClick={onLogout}>
                    退出登录
                  </button>
                </div>
              </>
            ) : (
              <>
                <span className="footer__note">
                  <Icon name="info" size={16} />
                  初始化口令打印在服务启动日志中
                </span>
                <span>simple-captcha 自托管行为验证码服务</span>
              </>
            )}
          </div>
        </footer>
      </div>
    </ToastProvider>
  );
}

interface UserMenuProps {
  username: string;
  /** 会话到期时间，Unix 秒 */
  expiresAt: number;
  accountHref: string;
  onLogout: () => void;
}

/** 顶栏右侧的管理员菜单：账号与安全、退出登录 */
function UserMenu({ username, expiresAt, accountHref, onLogout }: UserMenuProps) {
  const [open, setOpen] = useState(false);
  const rootRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!open) {
      return;
    }
    const onPointerDown = (event: PointerEvent) => {
      if (!rootRef.current?.contains(event.target as Node)) {
        setOpen(false);
      }
    };
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        setOpen(false);
      }
    };
    document.addEventListener('pointerdown', onPointerDown);
    document.addEventListener('keydown', onKeyDown);
    return () => {
      document.removeEventListener('pointerdown', onPointerDown);
      document.removeEventListener('keydown', onKeyDown);
    };
  }, [open]);

  return (
    <div className="user-menu" ref={rootRef}>
      <button
        type="button"
        className="user-menu__trigger"
        aria-haspopup="menu"
        aria-expanded={open}
        onClick={() => setOpen((value) => !value)}
      >
        <span className="user-menu__dot" aria-hidden="true" />
        <span className="user-menu__name">{username}</span>
        <Icon name="chevron-down" size={16} className="user-menu__chevron" />
      </button>

      {open && (
        <div className="user-menu__panel" role="menu">
          <div className="user-menu__head">
            <span className="user-menu__label">当前管理员</span>
            <span className="user-menu__user">{username}</span>
            <span className="user-menu__label">
              会话到期 {formatDateTime(new Date(expiresAt * 1000).toISOString())}
            </span>
          </div>
          <a
            role="menuitem"
            className="user-menu__item"
            href={accountHref}
            onClick={() => setOpen(false)}
          >
            <Icon name="shield" size={16} />
            账号与安全
          </a>
          <div className="user-menu__sep" />
          <button
            role="menuitem"
            type="button"
            className="user-menu__item user-menu__item--danger"
            onClick={() => {
              setOpen(false);
              onLogout();
            }}
          >
            <Icon name="logout" size={16} />
            退出登录
          </button>
        </div>
      )}
    </div>
  );
}
