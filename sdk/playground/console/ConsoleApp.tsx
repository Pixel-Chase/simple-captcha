import { CaptchaProvider } from '@aetherlib/g-captcha';
import { CONSOLE_APP_ID } from '../api';
import { ConsoleShell } from './ConsoleShell';
import { ConsoleView, parseConsoleSection } from './ConsoleView';
import type { ConsoleSection } from './ConsoleView';
import { useConsoleAuth, useLocationHash } from './useConsoleAuth';

const consoleHref = (section: ConsoleSection) => (section === 'overview' ? '#/' : `#/${section}`);

/**
 * 内嵌在 Go 服务里的管理后台（默认挂在 /console/，见 backend/console）。
 * 与 API 同源，不需要配置跨域；只包含控制台，不含接入文档。路由：#/ 概览、#/apps、#/risk、#/settings、#/images、#/account
 */
export function ConsoleApp() {
  const section = parseConsoleSection(useLocationHash().replace(/^#/, ''));
  const { auth, handleLogout, ...handlers } = useConsoleAuth();

  return (
    <CaptchaProvider api={{ appId: CONSOLE_APP_ID }}>
      <ConsoleShell auth={auth} section={section} hrefFor={consoleHref} onLogout={handleLogout}>
        <ConsoleView auth={auth} section={section} hrefFor={consoleHref} {...handlers} />
      </ConsoleShell>
    </CaptchaProvider>
  );
}
