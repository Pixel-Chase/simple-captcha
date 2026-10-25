import { CaptchaProvider } from '@aetherlib/g-captcha';
import { API_BASE, API_PREFIX } from './api';
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
  const { auth, handleSession, handleLogout, handleSessionExpired } = useConsoleAuth();

  // 未登录的控制台首页是暗色展台，顶栏与页脚随之切换
  const home = route.name === 'console' && auth.status === 'anonymous';

  const tabClass = (name: Route['name']) =>
    route.name === name ? 'topbar__tab topbar__tab--active' : 'topbar__tab';

  return (
    <CaptchaProvider api={{ basePath: API_BASE, prefix: API_PREFIX }}>
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
            />
          )}
        </main>

        <footer className="footer">
          后端 {API_BASE} · 管理员初始密码在后端首次启动时打印于服务控制台
        </footer>
      </div>
    </CaptchaProvider>
  );
}
