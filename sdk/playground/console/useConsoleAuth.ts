import { useCallback, useEffect, useState } from 'react';
import { adminApi, tokenStorage } from '../api';
import type { LoginResponse } from '../api';

export type AuthState =
  | { status: 'checking' }
  /** 尚未初始化：显示初始化页面 */
  | { status: 'setup' }
  | { status: 'anonymous' }
  | {
      status: 'authed';
      token: string;
      username: string;
      expiresAt: number;
    };

const ANONYMOUS: AuthState = { status: 'anonymous' };
const SETUP: AuthState = { status: 'setup' };

/** 管理后台登录态：演示站与内嵌在 Go 服务里的管理后台共用 */
export function useConsoleAuth() {
  const [auth, setAuth] = useState<AuthState>({ status: 'checking' });

  // 先确认是否已初始化，再用 sessionStorage 中的令牌恢复登录态；两次请求都在 checking 状态下完成，
  // 刷新时不会先闪一下登录页
  useEffect(() => {
    let cancelled = false;

    const restore = async (): Promise<AuthState> => {
      try {
        const { initialized } = await adminApi.setupStatus();
        if (!initialized) {
          tokenStorage.clear();
          return SETUP;
        }
      } catch {
        // 服务暂不可达时按已初始化处理：登录页提交时会给出连接错误
      }

      const token = tokenStorage.load();
      if (!token) {
        return ANONYMOUS;
      }
      try {
        const session = await adminApi.session(token);
        return { status: 'authed', token, ...session };
      } catch {
        tokenStorage.clear();
        return ANONYMOUS;
      }
    };

    void restore().then((next) => {
      if (!cancelled) {
        setAuth(next);
      }
    });
    return () => {
      cancelled = true;
    };
  }, []);

  /** 登录、初始化成功，或改密后换用新签发的会话（旧会话已全部失效） */
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

  /** 登录时服务端返回"尚未初始化"（例如执行了 reset-admin）：切到初始化页面 */
  const handleNotInitialized = useCallback(() => {
    tokenStorage.clear();
    setAuth(SETUP);
  }, []);

  /** 初始化时服务端返回"已初始化"（例如另一个浏览器先完成了初始化）：切回登录页 */
  const handleAlreadyInitialized = useCallback(() => {
    setAuth(ANONYMOUS);
  }, []);

  return {
    auth,
    handleSession,
    handleLogout,
    handleSessionExpired,
    handleNotInitialized,
    handleAlreadyInitialized,
  };
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
