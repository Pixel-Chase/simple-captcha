import { CaptchaProvider } from '@aetherlib/g-captcha';
import { CONSOLE_APP_ID } from './api';
import { ConsoleShell } from './console/ConsoleShell';
import { ConsoleView, parseConsoleSection } from './console/ConsoleView';
import type { ConsoleSection } from './console/ConsoleView';
import { useConsoleAuth, useLocationHash } from './console/useConsoleAuth';
import { DocsPage } from './pages/DocsPage';

/** 两个顶级页面：#/console[/apps|/risk|…] 控制台，#/docs[/<anchor>] 接入文档 */
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
  section === 'overview' ? '#/console' : `#/console/${section}`;

/** 本地演示站：接入文档 + 控制台。线上管理后台见 console/ConsoleApp（内嵌在 Go 服务里，不含接入文档） */
export default function App() {
  const route = parseRoute(useLocationHash());
  const { auth, handleLogout, ...handlers } = useConsoleAuth();

  return (
    <CaptchaProvider api={{ appId: CONSOLE_APP_ID }}>
      <ConsoleShell
        auth={auth}
        section={route.name === 'console' ? route.section : null}
        hrefFor={consoleHref}
        onLogout={handleLogout}
        docs={{ href: '#/docs', active: route.name === 'docs' }}
        mainClassName={route.name === 'docs' ? 'main--docs' : undefined}
      >
        {route.name === 'docs' ? (
          <DocsPage anchor={route.anchor} />
        ) : (
          <ConsoleView auth={auth} section={route.section} hrefFor={consoleHref} {...handlers} />
        )}
      </ConsoleShell>
    </CaptchaProvider>
  );
}
