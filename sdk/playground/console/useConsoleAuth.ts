import { useCallback, useEffect, useState } from 'react';
import { adminApi, tokenStorage } from '../api';
import type { LoginResponse } from '../api';

export type AuthState =
  | { status: 'checking' }
  | { status: 'anonymous' }
  | {
      status: 'authed';
      token: string;
      username: string;
      expiresAt: number;
      mustChangePassword: boolean;
    };

const ANONYMOUS: AuthState = { status: 'anonymous' };

/** 管理后台登录态：演示站与内嵌在 Go 服务里的管理后台共用 */
export function useConsoleAuth() {
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

  /** 登录成功，或改密后换用新签发的会话（旧会话已全部失效） */
  const handleSession = useCallback((result: LoginResponse) => {
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

  return { auth, handleSession, handleLogout, handleSessionExpired };
}

/** 订阅 location.hash，两个入口各自解析路由 */
export function useLocationHash(): string {
  const [hash, setHash] = useState(() => window.location.hash);

  useEffect(() => {
    const sync = () => setHash(window.location.hash);
    window.addEventListener('hashchange', sync);
    return () => window.removeEventListener('hashchange', sync);
  }, []);

  return hash;
}
