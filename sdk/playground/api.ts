import type { CaptchaErrorResponse } from '@aetherlib/g-captcha';

/** 与 backend/console 约定的占位符：服务端返回 index.html 时替换为实际的 API_PREFIX */
const API_PREFIX_PLACEHOLDER = '__CAPTCHA_API_PREFIX__';

/** 内嵌在 Go 服务中时由服务端写入的 API 前缀；本地演示站没有该 meta 或仍是占位符，返回 null */
const embeddedApiPrefix = (() => {
  const content = document.querySelector<HTMLMetaElement>(
    'meta[name="captcha-api-prefix"]',
  )?.content;
  return content === undefined || content === API_PREFIX_PLACEHOLDER ? null : content;
})();

/**
 * 验证码后端地址与 API 前缀，演示页与 SDK 共用同一个服务：
 * - 内嵌在 Go 服务中（/console/）：与页面同源，前缀取服务端配置的 API_PREFIX
 * - 本地演示站：VITE_API_BASE（见 .env.example），前缀固定 /api/v1
 */
export const API_BASE =
  embeddedApiPrefix !== null
    ? window.location.origin
    : (import.meta.env.VITE_API_BASE || 'http://127.0.0.1:3321').replace(/\/+$/, '');
export const API_PREFIX = embeddedApiPrefix ?? '/api/v1';
const ADMIN_ENDPOINT = `${API_BASE}${API_PREFIX}/admin`;

const TOKEN_STORAGE_KEY = 'g-captcha-admin-token';

export interface LoginResponse {
  token: string;
  username: string;
  /** Unix 秒 */
  expiresAt: number;
  /** 仍在使用随机初始密码：改密之前只开放会话查询、注销与改密 */
  mustChangePassword: boolean;
}

export interface SessionResponse {
  username: string;
  expiresAt: number;
  mustChangePassword: boolean;
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

/** 接入应用：业务后端凭应用密钥调用 S2S 接口，列表不含密钥本身 */
export interface AppItem {
  id: string;
  name: string;
  /** 密钥前若干位，仅用于辨认 */
  secretHint: string;
  enabled: boolean;
  createdAt: string;
  updatedAt: string;
  secretRotatedAt: string;
  lastUsedAt: string | null;
}

export interface AppListResponse {
  items: AppItem[];
  total: number;
}

/** 新建应用或重置密钥的响应：明文密钥只出现这一次 */
export interface AppSecretResponse {
  app: AppItem;
  secret: string;
}

/**
 * 风控观察：一条遥测记录（已由服务端脱敏）。
 * kind=issue 为出题评估（level / base / boost / envScore），kind=verify 为校验评分（score / success）；
 * 早期记录没有 kind 字段，按校验记录展示
 */
export interface RiskTelemetryItem {
  kind?: 'issue' | 'verify';
  time?: string;
  type?: string;
  key?: string;
  client?: string;
  ip?: string;
  ua?: string;
  level?: number;
  base?: number;
  boost?: number;
  envScore?: number;
  behaviorScore?: number;
  score?: number;
  success?: boolean;
  consistent?: boolean;
  viaPost?: boolean;
  reasons?: string[];
  anomalies?: string[] | null;
  tls?: string;
  h2?: string;
  [extra: string]: unknown;
}

export interface RiskTelemetryResponse {
  items: RiskTelemetryItem[];
  limit: number;
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
    throw new AdminApiError(0, 0, '无法连接验证码服务，请确认后端已启动');
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
  /** 改密成功后旧会话全部失效，响应里是新签发的会话 */
  changePassword(token: string, body: { currentPassword: string; newPassword: string }) {
    return request<LoginResponse>(
      '/password',
      {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body),
      },
      token,
    );
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
  listApps(token: string) {
    return request<AppListResponse>('/apps', {}, token);
  },
  /** 最近的出题评估与校验评分记录，limit 1-500 */
  riskTelemetry(token: string, limit: number) {
    return request<RiskTelemetryResponse>(
      `/risk/telemetry?limit=${encodeURIComponent(limit)}`,
      {},
      token,
    );
  },
  createApp(token: string, name: string) {
    return request<AppSecretResponse>(
      '/apps',
      {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ name }),
      },
      token,
    );
  },
  updateApp(token: string, id: string, body: { name: string; enabled: boolean }) {
    return request<AppItem>(
      `/apps/${encodeURIComponent(id)}`,
      {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body),
      },
      token,
    );
  },
  rotateAppSecret(token: string, id: string) {
    return request<AppSecretResponse>(
      `/apps/${encodeURIComponent(id)}/secret`,
      { method: 'POST' },
      token,
    );
  },
  deleteApp(token: string, id: string) {
    return request<void>(`/apps/${encodeURIComponent(id)}`, { method: 'DELETE' }, token);
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
