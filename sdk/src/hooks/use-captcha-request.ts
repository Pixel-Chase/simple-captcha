import { useState } from 'react';
import request from 'axios';
import type {
  CaptchaChallengeData,
  CaptchaErrorResponse,
  CaptchaType,
  CaptchaVerifyPayload,
  CaptchaVerifySuccessData,
} from '../Captcha.types';
import { useCaptchaConfig } from '../CaptchaProvider';

export interface GetCaptchaOption {
  type: CaptchaType
  mode?: 'light' | 'dark'
}

export interface GetCaptchaResult {
  get: () => Promise<CaptchaChallengeData>
  loading: boolean
  data: CaptchaChallengeData | null
  error: CaptchaErrorResponse | null
  check: (body: CaptchaVerifyPayload | Record<string, unknown>) => Promise<CaptchaVerifySuccessData>
}

export function isCaptchaErrorResponse(value: unknown): value is CaptchaErrorResponse {
  return !!value
    && typeof value === 'object'
    && 'code' in value
    && 'error' in value
    && typeof (value as CaptchaErrorResponse).code === 'number'
    && typeof (value as CaptchaErrorResponse).error === 'string';
}

export function getCaptchaErrorMessage(error: CaptchaErrorResponse | string): string {
  if (typeof error === 'string') {
    return error;
  }

  return error.error;
}

/** 取整并裁剪到 1-3 的设备像素比，服务端据此选择出图倍率 */
export function getDevicePixelRatio(): number {
  if (typeof window === 'undefined') {
    return 1;
  }
  const dpr = Math.round(window.devicePixelRatio || 1);
  return Math.min(3, Math.max(1, dpr));
}

export function useCaptchaRequest(option: GetCaptchaOption): GetCaptchaResult {
  const [loading, setLoading] = useState<boolean>(false);
  const [data, setData] = useState<CaptchaChallengeData | null>(null);
  const [error, setError] = useState<CaptchaErrorResponse | null>(null);
  const { api } = useCaptchaConfig();
  const basePath = (api.basePath || '').replace(/\/+$/, '');
  const normalizedPrefix = (api.prefix ?? '/api/v1').trim().replace(/^\/+|\/+$/g, '');
  const apiPrefix = normalizedPrefix ? `/${normalizedPrefix}` : '';
  const captchaEndpoint = `${basePath}${apiPrefix}/captchas`;

  const get = async (): Promise<CaptchaChallengeData> => {
    setLoading(true);
    setError(null);
    try {
      const params = new URLSearchParams();
      if (option.mode) {
        params.set('mode', option.mode);
      }
      params.set('dpr', String(getDevicePixelRatio()));

      const url = `${captchaEndpoint}/${option.type}?${params.toString()}`;
      const res = await request.get<CaptchaChallengeData | CaptchaErrorResponse>(url);
      if (isCaptchaErrorResponse(res.data)) {
        setError(res.data);
        throw res.data;
      }

      setData(res.data);
      return res.data;
    } catch (e: unknown) {
      const errResponse = (e as { response?: { data?: unknown } })?.response?.data || e;
      const normalizedError = isCaptchaErrorResponse(errResponse)
        ? errResponse
        : { code: 500, error: '验证码获取失败' };
      setError(normalizedError);
      throw normalizedError;
    } finally {
      setLoading(false);
    }
  };

  const check = async (body: CaptchaVerifyPayload | Record<string, unknown>): Promise<CaptchaVerifySuccessData> => {
    setLoading(true);
    setError(null);
    try {
      // 提交路径使用服务端实际下发的题型（auto 已被服务端解析）
      const resolvedType = data?.type || option.type;
      const url = `${captchaEndpoint}/${resolvedType}/${data?.key}/attempts`;
      const res = await request.post<CaptchaVerifySuccessData | CaptchaErrorResponse>(url, body);
      if (isCaptchaErrorResponse(res.data)) {
        setError(res.data);
        throw res.data;
      }

      return res.data;
    } catch (e: unknown) {
      const errResponse = (e as { response?: { data?: unknown } })?.response?.data || e;
      const normalizedError = isCaptchaErrorResponse(errResponse)
        ? errResponse
        : { code: 500, error: '验证码校验失败' };
      setError(normalizedError);
      throw normalizedError;
    } finally {
      setLoading(false);
    }
  };

  return { get, check, loading, data, error };
}
