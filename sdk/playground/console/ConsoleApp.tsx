import { CaptchaProvider } from '@aetherlib/g-captcha';
import { API_BASE, API_PREFIX } from '../api';
import { ConsoleView, parseConsoleSection, TopbarUser } from './ConsoleView';
import type { ConsoleSection } from './ConsoleView';
import { useConsoleAuth, useLocationHash } from './useConsoleAuth';

const consoleHref = (section: ConsoleSection) => (section === 'images' ? '#/' : `#/${section}`);

/**
 * 内嵌在 Go 服务里的管理后台（默认挂在 /console/，见 backend/console）。
 * 与 API 同源，不需要配置跨域；只包含控制台，不含接入文档。路由：#/ 背景图、#/apps、#/account
 */
export function ConsoleApp() {
  const section = parseConsoleSection(useLocationHash().replace(/^#/, ''));
  const { auth, handleSession, handleLogout, handleSessionExpired } = useConsoleAuth();

  return (
    <CaptchaProvider
      api={{ basePath: API_BASE, prefix: API_PREFIX }}
      locale={{ buttonText: '点击完成验证', userCancel: '你已取消本次验证' }}
    >
      <div className={auth.status === 'anonymous' ? 'app app--home' : 'app'}>
        <header className="topbar">
          <div className="topbar__brand">
            <span className="topbar__logo" aria-hidden="true" />
            <div>
              <div className="topbar__title">simple-captcha</div>
              <div className="topbar__subtitle">管理后台 · 背景图与接入应用</div>
            </div>
          </div>

          <TopbarUser auth={auth} onLogout={handleLogout} />
        </header>

        <main className="main">
          <ConsoleView
            auth={auth}
            section={section}
            hrefFor={consoleHref}
            handleSession={handleSession}
            handleSessionExpired={handleSessionExpired}
          />
        </main>

        <footer className="footer">
          管理员初始密码在服务首次启动时打印于服务日志 · 遗失可执行{' '}
          <code>./simple-captcha reset-admin-password</code>
        </footer>
      </div>
    </CaptchaProvider>
  );
}
