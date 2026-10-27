import type { ApiConfig } from '../Captcha.types';

/** 验证码接口路径固定为 /api/v1，不支持自定义前缀 */
export const API_PREFIX = '/api/v1';

/** 规范化 baseURL：去掉首尾空白与末尾斜杠，空串表示同源 */
export function normalizeBaseURL(baseURL: string | undefined): string {
  return (baseURL || '').trim().replace(/\/+$/, '');
}

/** 检查接入参数，返回给接入方看的错误文案；参数有效时返回 null */
export function apiConfigError(api: Partial<ApiConfig>): string | null {
  if (!api.appId || !api.appId.trim()) {
    return '缺少 appId：请在 <CaptchaProvider api={{ appId }}> 中传入管理后台「接入应用」的应用 ID';
  }
  const baseURL = normalizeBaseURL(api.baseURL);
  if (baseURL && !/^https?:\/\/[^/?#\s]+$/.test(baseURL)) {
    return 'baseURL 只能是验证码服务的 origin（例如 https://captcha.example.com），不能带路径或查询串';
  }
  return null;
}

/** 用户侧接口（出题与提交）在查询参数里携带 appId */
export function withAppId(url: string, appId: string): string {
  return `${url}?appId=${encodeURIComponent(appId)}`;
}
