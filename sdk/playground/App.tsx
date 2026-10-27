import { CaptchaProvider } from '@aetherlib/g-captcha';
import { API_PREFIX, CONSOLE_APP_ID } from './api';
import { ConsoleView, parseConsoleSection, TopbarUser } from './console/ConsoleView';
import type { ConsoleSection } from './console/ConsoleView';
import { useConsoleAuth, useLocationHash } from './console/useConsoleAuth';
import { DocsPage } from './pages/DocsPage';

/** 两个顶级页面：#/console[/apps|/account] 控制台，#/docs[/<anchor>] 接入文档 */
type Route = { name: 'console'; section: ConsoleSection } | { name: 'docs'; anchor: string | null };

function parseRoute(hash: string): Route {
  let path = hash.replace(/^#/, '');
  try {
    path = decodeURIComponent(path);
  } catch {
    // 非法转义序列按原文处理即可
  }
  const match = /^\/docs(?:\/(.+))?$/.exec(path);
  if (match) {
    return { name: 'docs', anchor: match[1] ?? null };
  }
  return { name: 'console', section: parseConsoleSection(path.replace(/^\/console/, '')) };
}

const consoleHref = (section: ConsoleSection) =>
  section === 'images' ? '#/console' : `#/console/${section}`;

/** 本地演示站：接入文档 + 控制台。线上管理后台见 console/ConsoleApp（内嵌在 Go 服务里，不含接入文档） */
export default function App() {
  const route = parseRoute(useLocationHash());
  const {
    auth,
    handleSession,
    handleLogout,
    handleSessionExpired,
    handleNotInitialized,
    handleAlreadyInitialized,
  } = useConsoleAuth();

  // 未登录（含未初始化）的控制台首页是暗色展台，顶栏与页脚随之切换
  const home = route.name === 'console' && (auth.status === 'anonymous' || auth.status === 'setup');

  const tabClass = (name: Route['name']) =>
    route.name === name ? 'topbar__tab topbar__tab--active' : 'topbar__tab';

  return (
    <CaptchaProvider api={{ appId: CONSOLE_APP_ID }}>
      <div className={home ? 'app app--home' : 'app'}>
        <header className="topbar">
          <div className="topbar__brand">
            <span className="topbar__logo" aria-hidden="true" />
            <div>
              <div className="topbar__title">simple-captcha</div>
              <div className="topbar__subtitle">行为验证码 · 接入文档 · 背景图与接入应用管理</div>
            </div>
          </div>

          <nav className="topbar__nav" aria-label="主导航">
            <a className={tabClass('console')} href="#/console">
              控制台
            </a>
            <a className={tabClass('docs')} href="#/docs">
              接入文档
            </a>
          </nav>

          <TopbarUser auth={auth} onLogout={handleLogout} />
        </header>

        <main className={route.name === 'docs' ? 'main main--docs' : 'main'}>
          {route.name === 'docs' ? (
            <DocsPage anchor={route.anchor} />
          ) : (
            <ConsoleView
              auth={auth}
              section={route.section}
              hrefFor={consoleHref}
              handleSession={handleSession}
              handleSessionExpired={handleSessionExpired}
              handleNotInitialized={handleNotInitialized}
              handleAlreadyInitialized={handleAlreadyInitialized}
            />
          )}
        </main>

        <footer className="footer">
          API 与页面同源（{API_PREFIX}，开发时由 Vite 代理转发）· 初始化口令打印在后端启动日志中
        </footer>
      </div>
    </CaptchaProvider>
  );
}
