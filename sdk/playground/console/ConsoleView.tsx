import { AccountPage } from '../pages/AccountPage';
import { AppsPage } from '../pages/AppsPage';
import { ImagesPage } from '../pages/ImagesPage';
import { LoginPage } from '../pages/LoginPage';
import { OverviewPage } from '../pages/OverviewPage';
import { RiskPage } from '../pages/RiskPage';
import { SettingsPage } from '../pages/SettingsPage';
import { SetupPage } from '../pages/SetupPage';
import type { AuthState, useConsoleAuth } from './useConsoleAuth';

/** 控制台分区：概览（默认）、接入应用、风控观察、策略配置、背景图，以及管理员菜单里的账号与安全 */
export type ConsoleSection = 'overview' | 'apps' | 'risk' | 'settings' | 'images' | 'account';

/** 顶栏导航的分区；账号与安全不在导航里，从管理员菜单进入 */
export const NAV_SECTIONS: ReadonlyArray<{ id: ConsoleSection; label: string }> = [
  { id: 'overview', label: '概览' },
  { id: 'apps', label: '接入应用' },
  { id: 'risk', label: '风控观察' },
  { id: 'settings', label: '策略配置' },
  { id: 'images', label: '背景图' },
];

/** 从去掉入口前缀后的路径解析分区：'' → 概览，'/apps'、'/risk' 等 → 对应分区 */
export function parseConsoleSection(rest: string): ConsoleSection {
  const match = /^\/(apps|risk|settings|images|account)$/.exec(rest);
  return (match?.[1] as ConsoleSection | undefined) ?? 'overview';
}

type ConsoleHandlers = Omit<ReturnType<typeof useConsoleAuth>, 'auth' | 'handleLogout'>;

interface ConsoleViewProps extends ConsoleHandlers {
  auth: AuthState;
  section: ConsoleSection;
  hrefFor: (section: ConsoleSection) => string;
}

/** 控制台主体：初始化、登录与各分区页面（导航在 ConsoleShell 的顶栏里） */
export function ConsoleView({
  auth,
  section,
  hrefFor,
  handleSession,
  handleSessionExpired,
  handleNotInitialized,
  handleAlreadyInitialized,
}: ConsoleViewProps) {
  if (auth.status === 'checking') {
    return <div className="page-status">正在恢复登录状态…</div>;
  }
  if (auth.status === 'setup') {
    return <SetupPage onSetup={handleSession} onAlreadyInitialized={handleAlreadyInitialized} />;
  }
  if (auth.status === 'anonymous') {
    return <LoginPage onLogin={handleSession} onNotInitialized={handleNotInitialized} />;
  }

  const { token } = auth;
  switch (section) {
    case 'apps':
      return <AppsPage token={token} onSessionExpired={handleSessionExpired} />;
    case 'risk':
      return <RiskPage token={token} onSessionExpired={handleSessionExpired} hrefFor={hrefFor} />;
    case 'settings':
      return (
        <SettingsPage token={token} onSessionExpired={handleSessionExpired} hrefFor={hrefFor} />
      );
    case 'images':
      return <ImagesPage token={token} onSessionExpired={handleSessionExpired} />;
    case 'account':
      return (
        <AccountPage
          token={token}
          username={auth.username}
          expiresAt={auth.expiresAt}
          onChanged={handleSession}
          onSessionExpired={handleSessionExpired}
        />
      );
    default:
      return (
        <OverviewPage token={token} onSessionExpired={handleSessionExpired} hrefFor={hrefFor} />
      );
  }
}
