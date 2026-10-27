import type { CaptchaErrorResponse } from '@aetherlib/g-captcha';

/**
 * 验证码后端与页面同源，API 路径固定为 /api/v1：
 * - 内嵌在 Go 服务中（/console/）：本来就同源
 * - 本地演示站：Vite 开发服务器把 /api 代理到 VITE_API_PROXY_TARGET（见 vite.config.ts 与 .env.example）
 */
export const API_PREFIX = '/api/v1';
const ADMIN_ENDPOINT = `${API_PREFIX}/admin`;

/** 管理后台自己的内置系统应用：只接受同源请求，不可删除或修改（见 backend 的 app_console） */
export const CONSOLE_APP_ID = 'app_console';

const TOKEN_STORAGE_KEY = 'g-captcha-admin-token';

/** 登录、初始化与改密成功后签发的会话 */
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

/** 是否已完成首次初始化（已设置管理员账号） */
export interface SetupStatusResponse {
  initialized: boolean;
}

/** 管理接口中控制台需要区别处理的业务错误码 */
export const ADMIN_ERROR_CODE = {
  /** 尚未初始化：登录页据此切换到初始化页面 */
  notInitialized: 2012,
  /** 已初始化：初始化页面据此切回登录页 */
  alreadyInitialized: 2013,
  /** 初始化口令错误 */
  setupCodeInvalid: 2014,
} as const;

/** 初始化相关错误的中文提示，比服务端的通用文案多给一句处理办法 */
const ADMIN_ERROR_TEXT: Record<number, string> = {
  [ADMIN_ERROR_CODE.notInitialized]: '管理后台尚未初始化，请先完成初始化',
  [ADMIN_ERROR_CODE.alreadyInitialized]: '管理后台已完成初始化，请直接登录',
  [ADMIN_ERROR_CODE.setupCodeInvalid]: '初始化口令错误，请以服务最近一次启动日志中的口令为准',
};

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
  /** 允许发起出题与提交的来源（已规范化）；系统应用为空，只接受同源 */
  origins: string[];
  /** 内置系统应用（管理后台）：不可删除、停用、改名、轮换密钥或修改来源 */
  isSystem: boolean;
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

/** 验证码与风控策略（与 backend/config/runtime.go 的 RuntimeSettings 一致），保存后立即生效 */
export interface RuntimeSettings {
  captcha: {
    expireSeconds: number;
    secondVerifyExpireSeconds: number;
    maxAttempts: number;
    ipEscalatePerMinute: number;
    defaultDifficulty: number;
    minSolveTimeMs: number;
    bindClient: boolean;
    imageQuality: number;
    maxScale: number;
  };
  pow: {
    enabled: boolean;
    required: boolean;
    difficulty: number;
    maxDifficulty: number;
  };
  risk: {
    enabled: boolean;
    mode: 'enforce' | 'log';
    escalateThreshold: number;
    blockThreshold: number;
    stateTtlSeconds: number;
    telemetryRetention: number;
    envScoreCap: number;
    blockMinBehavior: number;
    fpReplayThreshold: number;
    fpReplayWindowSeconds: number;
  };
}

export interface SettingsResponse {
  settings: RuntimeSettings;
  defaults: RuntimeSettings;
}

/**
 * 风控观察：一条遥测记录（已由服务端脱敏）。
 * kind=issue 为出题评估（level / base / boost / envScore），kind=verify 为校验评分（score / success）
 */
export interface RiskTelemetryItem {
  kind: 'issue' | 'verify';
  time?: string;
  /** 请求所属的接入应用 ID */
  app: string;
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
  if (error instanceof AdminApiError && ADMIN_ERROR_TEXT[error.code]) {
    return ADMIN_ERROR_TEXT[error.code];
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
  setupStatus() {
    return request<SetupStatusResponse>('/setup');
  },
  /** 首次初始化：口令见服务启动日志；成功后直接签发会话 */
  setup(body: { setupCode: string; username: string; password: string }) {
    return request<LoginResponse>('/setup', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    });
  },
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
  createApp(token: string, name: string, origins: string[]) {
    return request<AppSecretResponse>(
      '/apps',
      {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ name, origins }),
      },
      token,
    );
  },
  /** origins 缺省时保持不变，提供时整体替换 */
  updateApp(
    token: string,
    id: string,
    body: { name: string; enabled: boolean; origins?: string[] },
  ) {
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
  getSettings(token: string) {
    return request<SettingsResponse>('/settings', {}, token);
  },
  /** 提交完整策略；校验失败返回 400，错误信息含字段名 */
  updateSettings(token: string, settings: RuntimeSettings) {
    return request<{ settings: RuntimeSettings }>(
      '/settings',
      {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(settings),
      },
      token,
    );
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
