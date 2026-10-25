import { AppsPage } from '../pages/AppsPage';
import { ImagesPage } from '../pages/ImagesPage';
import { LoginPage } from '../pages/LoginPage';
import { PasswordPage } from '../pages/PasswordPage';
import type { AuthState, useConsoleAuth } from './useConsoleAuth';

/** 控制台分区：背景图（默认）、接入应用、修改密码 */
export type ConsoleSection = 'images' | 'apps' | 'account';

const SECTIONS: ReadonlyArray<{ id: ConsoleSection; label: string }> = [
  { id: 'images', label: '背景图' },
  { id: 'apps', label: '接入应用' },
  { id: 'account', label: '修改密码' },
];

/** 从去掉入口前缀后的路径解析分区：'' → 背景图，'/apps'、'/account' → 对应分区 */
export function parseConsoleSection(rest: string): ConsoleSection {
  const match = /^\/(apps|account)$/.exec(rest);
  return (match?.[1] as ConsoleSection | undefined) ?? 'images';
}

type ConsoleHandlers = Omit<ReturnType<typeof useConsoleAuth>, 'auth' | 'handleLogout'>;

interface ConsoleViewProps extends ConsoleHandlers {
  auth: AuthState;
  section: ConsoleSection;
  /** 分区链接：演示站为 #/console/apps，内嵌管理后台为 #/apps */
  hrefFor: (section: ConsoleSection) => string;
}

/** 控制台主体：登录、强制改密、分区导航与各分区页面 */
export function ConsoleView({
  auth,
  section,
  hrefFor,
  handleSession,
  handleSessionExpired,
}: ConsoleViewProps) {
  if (auth.status === 'checking') {
    return <div className="page-status">正在恢复登录状态…</div>;
  }
  if (auth.status === 'anonymous') {
    return <LoginPage onLogin={handleSession} />;
  }

  // 仍在使用随机初始密码：服务端只开放改密，其他分区一律先改密，也不显示分区导航
  if (auth.mustChangePassword) {
    return (
      <PasswordPage
        token={auth.token}
        username={auth.username}
        forced
        onChanged={handleSession}
        onSessionExpired={handleSessionExpired}
      />
    );
  }

  return (
    <>
      <nav className="subnav" aria-label="控制台">
        {SECTIONS.map((item) => (
          <a
            key={item.id}
            href={hrefFor(item.id)}
            className={section === item.id ? 'subnav__tab subnav__tab--active' : 'subnav__tab'}
            aria-current={section === item.id ? 'page' : undefined}
          >
            {item.label}
          </a>
        ))}
      </nav>
      {section === 'account' && (
        <PasswordPage
          token={auth.token}
          username={auth.username}
          forced={false}
          onChanged={handleSession}
          onSessionExpired={handleSessionExpired}
        />
      )}
      {section === 'apps' && (
        <AppsPage token={auth.token} onSessionExpired={handleSessionExpired} />
      )}
      {section === 'images' && (
        <ImagesPage token={auth.token} onSessionExpired={handleSessionExpired} />
      )}
    </>
  );
}

interface TopbarUserProps {
  auth: AuthState;
  onLogout: () => void;
}

/** 顶栏右侧：当前管理员与退出按钮 */
export function TopbarUser({ auth, onLogout }: TopbarUserProps) {
  return (
    <div className="topbar__user">
      {auth.status === 'authed' && (
        <>
          <span className="topbar__name">{auth.username}</span>
          <button type="button" className="btn btn--ghost btn--sm" onClick={onLogout}>
            退出登录
          </button>
        </>
      )}
    </div>
  );
}
