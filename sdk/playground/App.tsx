import { useCallback, useEffect, useState } from 'react';
import { CaptchaProvider } from '@aetherlib/g-captcha';
import { API_BASE, adminApi, tokenStorage } from './api';
import type { LoginResponse } from './api';
import { DocsPage } from './pages/DocsPage';
import { ImagesPage } from './pages/ImagesPage';
import { LoginPage } from './pages/LoginPage';

type AuthState =
  | { status: 'checking' }
  | { status: 'anonymous' }
  | { status: 'authed'; token: string; username: string; expiresAt: number };

const ANONYMOUS: AuthState = { status: 'anonymous' };

/** 两个顶级页面：#/console（默认）控制台，#/docs[/<anchor>] 接入文档 */
type Route = { name: 'console' } | { name: 'docs'; anchor: string | null };

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
  return { name: 'console' };
}

function useHashRoute(): Route {
  const [route, setRoute] = useState<Route>(() => parseRoute(window.location.hash));

  useEffect(() => {
    const sync = () => setRoute(parseRoute(window.location.hash));
    window.addEventListener('hashchange', sync);
    return () => window.removeEventListener('hashchange', sync);
  }, []);

  return route;
}

export default function App() {
  const route = useHashRoute();
  const [auth, setAuth] = useState<AuthState>({ status: 'checking' });

  // 刷新后用 sessionStorage 中的令牌恢复登录态，令牌失效则回到登录页
  useEffect(() => {
    const token = tokenStorage.load();
    if (!token) {
      setAuth(ANONYMOUS);
      return;
    }

    let cancelled = false;
    adminApi
      .session(token)
      .then((session) => {
        if (!cancelled) {
          setAuth({ status: 'authed', token, ...session });
        }
      })
      .catch(() => {
        if (!cancelled) {
          tokenStorage.clear();
          setAuth(ANONYMOUS);
        }
      });
    return () => {
      cancelled = true;
    };
  }, []);

  const handleLogin = useCallback((result: LoginResponse) => {
    tokenStorage.save(result.token);
    setAuth({ status: 'authed', ...result });
  }, []);

  const handleLogout = useCallback(() => {
    if (auth.status === 'authed') {
      void adminApi.logout(auth.token).catch(() => undefined);
    }
    tokenStorage.clear();
    setAuth(ANONYMOUS);
  }, [auth]);

  const handleSessionExpired = useCallback(() => {
    tokenStorage.clear();
    setAuth(ANONYMOUS);
  }, []);

  const tabClass = (name: Route['name']) =>
    route.name === name ? 'topbar__tab topbar__tab--active' : 'topbar__tab';

  return (
    <CaptchaProvider
      api={{ basePath: API_BASE }}
      locale={{ buttonText: '点击完成验证', userCancel: '你已取消本次验证' }}
      theme={{ primaryColor: '#2f6bff', borderRadius: 12 }}
    >
      <div className="app">
        <header className="topbar">
          <div className="topbar__brand">
            <span className="topbar__logo" aria-hidden="true" />
            <div>
              <div className="topbar__title">simple-captcha</div>
              <div className="topbar__subtitle">行为验证码 · 接入文档 · 背景图管理</div>
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

          <div className="topbar__user">
            {auth.status === 'authed' && (
              <>
                <span className="topbar__name">{auth.username}</span>
                <button type="button" className="btn btn--ghost btn--sm" onClick={handleLogout}>
                  退出登录
                </button>
              </>
            )}
          </div>
        </header>

        <main className={route.name === 'docs' ? 'main main--docs' : 'main'}>
          {route.name === 'docs' ? (
            <DocsPage anchor={route.anchor} />
          ) : (
            <>
              {auth.status === 'checking' && <div className="page-status">正在恢复登录状态…</div>}
              {auth.status === 'anonymous' && <LoginPage onLogin={handleLogin} />}
              {auth.status === 'authed' && (
                <ImagesPage token={auth.token} onSessionExpired={handleSessionExpired} />
              )}
            </>
          )}
        </main>

        <footer className="footer">
          后端 {API_BASE} · 控制台需以 <code>ADMIN_ENABLED=true</code> 启动
        </footer>
      </div>
    </CaptchaProvider>
  );
}
