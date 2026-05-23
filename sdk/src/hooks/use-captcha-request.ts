import { useState } from 'react';
import request from 'axios';
import type {
  CaptchaChallengeData,
  CaptchaErrorResponse,
  CaptchaType,
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
  check: (rawBody: Record<string, string>) => Promise<CaptchaVerifySuccessData>
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

export function useCaptchaRequest(option: GetCaptchaOption): GetCaptchaResult {
  const [loading, setLoading] = useState<boolean>(false);
  const [data, setData] = useState<CaptchaChallengeData | null>(null);
  const [error, setError] = useState<CaptchaErrorResponse | null>(null);
  const { api } = useCaptchaConfig();
  const basePath = api.basePath || '';

  const get = async (): Promise<CaptchaChallengeData> => {
    setLoading(true);
    setError(null);
    try {
      const url = `${basePath}/api/v1/captchas/${option.type}?mode=${option.mode || ''}`;
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

  const check = async (rawBody: Record<string, string>): Promise<CaptchaVerifySuccessData> => {
    setLoading(true);
    setError(null);
    try {
      const url = `${basePath}/api/v1/captchas/${option.type}/${data?.key}/attempts`;
      const res = await request.post<CaptchaVerifySuccessData | CaptchaErrorResponse>(url, rawBody);
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
