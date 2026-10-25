import type { CaptchaErrorResponse } from '@aetherlib/g-captcha';

/** 验证码后端地址：演示页与 SDK 共用同一个服务，可用 VITE_API_BASE 覆盖（见 .env.example） */
export const API_BASE = (import.meta.env.VITE_API_BASE || 'http://127.0.0.1:3321').replace(
  /\/+$/,
  '',
);
const API_PREFIX = '/api/v1';
const ADMIN_ENDPOINT = `${API_BASE}${API_PREFIX}/admin`;

const TOKEN_STORAGE_KEY = 'g-captcha-admin-token';

export interface LoginResponse {
  token: string;
  username: string;
  /** Unix 秒 */
  expiresAt: number;
}

export interface SessionResponse {
  username: string;
  expiresAt: number;
}

export interface BackgroundItem {
  name: string;
  size: number;
  width: number;
  height: number;
  format: string;
  modifiedAt: string;
}

export interface UploadConstraints {
  minWidth: number;
  minHeight: number;
  /** 建议尺寸（3:2），仅用于提示 */
  recommendedWidth: number;
  recommendedHeight: number;
  maxUploadSize: number;
  maxPixels: number;
  allowedFormats: string[];
}

export interface BackgroundListResponse {
  items: BackgroundItem[];
  total: number;
  constraints: UploadConstraints;
}

/** 管理接口错误：status 为 HTTP 状态码，code 为服务端业务错误码 */
export class AdminApiError extends Error {
  readonly status: number;
  readonly code: number;

  constructor(status: number, code: number, message: string) {
    super(message);
    this.name = 'AdminApiError';
    this.status = status;
    this.code = code;
  }
}

export const isAdminApiError = (value: unknown): value is AdminApiError =>
  value instanceof AdminApiError;

const isErrorPayload = (value: unknown): value is CaptchaErrorResponse =>
  !!value &&
  typeof value === 'object' &&
  typeof (value as CaptchaErrorResponse).code === 'number' &&
  typeof (value as CaptchaErrorResponse).error === 'string';

/** 把 SDK 回调、管理接口与原生异常统一成可展示的文案 */
export function getErrorMessage(error: unknown, fallback = '操作失败，请稍后重试'): string {
  if (typeof error === 'string') {
    return error;
  }
  if (error instanceof Error) {
    return error.message || fallback;
  }
  if (isErrorPayload(error)) {
    return error.error;
  }
  return fallback;
}

async function request<T>(path: string, init: RequestInit = {}, token?: string): Promise<T> {
  const headers = new Headers(init.headers);
  if (token) {
    headers.set('Authorization', `Bearer ${token}`);
  }

  let response: Response;
  try {
    response = await fetch(`${ADMIN_ENDPOINT}${path}`, { ...init, headers });
  } catch {
    throw new AdminApiError(0, 0, '无法连接验证码服务，请确认后端已启动并开启 ADMIN_ENABLED');
  }

  const text = await response.text();
  let payload: unknown = null;
  if (text) {
    try {
      payload = JSON.parse(text);
    } catch {
      payload = null;
    }
  }

  if (!response.ok) {
    if (isErrorPayload(payload)) {
      throw new AdminApiError(response.status, payload.code, payload.error);
    }
    throw new AdminApiError(
      response.status,
      response.status,
      `请求失败（HTTP ${response.status}）`,
    );
  }
  return payload as T;
}

export const adminApi = {
  login(body: { username: string; password: string; captchaKey: string }) {
    return request<LoginResponse>('/login', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    });
  },
  session(token: string) {
    return request<SessionResponse>('/session', {}, token);
  },
  logout(token: string) {
    return request<void>('/session', { method: 'DELETE' }, token);
  },
  listBackgrounds(token: string) {
    return request<BackgroundListResponse>('/backgrounds', {}, token);
  },
  uploadBackground(token: string, file: File) {
    const form = new FormData();
    form.append('file', file, file.name);
    return request<BackgroundItem>('/backgrounds', { method: 'POST', body: form }, token);
  },
  deleteBackground(token: string, name: string) {
    return request<void>(`/backgrounds/${encodeURIComponent(name)}`, { method: 'DELETE' }, token);
  },
  /** 预览需要携带令牌，无法直接用 <img src>，这里取回 blob 生成对象 URL */
  async fetchPreview(token: string, name: string): Promise<string> {
    const response = await fetch(`${ADMIN_ENDPOINT}/backgrounds/${encodeURIComponent(name)}`, {
      headers: { Authorization: `Bearer ${token}` },
    });
    if (!response.ok) {
      throw new AdminApiError(response.status, response.status, '预览加载失败');
    }
    return URL.createObjectURL(await response.blob());
  },
};

/** 会话令牌放在 sessionStorage：关闭标签页即失效，刷新页面仍可恢复 */
export const tokenStorage = {
  load(): string | null {
    try {
      return sessionStorage.getItem(TOKEN_STORAGE_KEY);
    } catch {
      return null;
    }
  },
  save(token: string) {
    try {
      sessionStorage.setItem(TOKEN_STORAGE_KEY, token);
    } catch {
      // 隐私模式等场景下存储不可用，忽略即可：刷新后需要重新登录
    }
  },
  clear() {
    try {
      sessionStorage.removeItem(TOKEN_STORAGE_KEY);
    } catch {
      // 同上
    }
  },
};

export function formatBytes(bytes: number): string {
  if (bytes >= 1 << 20) {
    return `${(bytes / (1 << 20)).toFixed(1)} MB`;
  }
  if (bytes >= 1 << 10) {
    return `${Math.round(bytes / (1 << 10))} KB`;
  }
  return `${bytes} B`;
}

export function formatDateTime(iso: string): string {
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) {
    return iso;
  }
  return date.toLocaleString('zh-CN', { hour12: false });
}
