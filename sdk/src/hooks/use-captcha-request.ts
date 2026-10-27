import { useState } from 'react';
import request from 'axios';
import type {
  ApiConfig,
  CaptchaChallengeData,
  CaptchaEnv,
  CaptchaErrorResponse,
  CaptchaIssuePayload,
  CaptchaType,
  CaptchaVerifyPayload,
  CaptchaVerifySuccessData,
} from '../Captcha.types';
import { useCaptchaConfig } from '../CaptchaProvider';
import { API_PREFIX, apiConfigError, normalizeBaseURL, withAppId } from '../utils/api-config';

export interface GetCaptchaOption {
  type: CaptchaType;
  mode?: 'light' | 'dark';
}

export interface GetCaptchaResult {
  /** 出题：env 为出题前采集的环境信号，服务端据此决定是否加难；不传则按默认难度出题 */
  get: (env?: CaptchaEnv) => Promise<CaptchaChallengeData>;
  loading: boolean;
  data: CaptchaChallengeData | null;
  error: CaptchaErrorResponse | null;
  check: (
    body: CaptchaVerifyPayload | Record<string, unknown>,
  ) => Promise<CaptchaVerifySuccessData>;
}

export function isCaptchaErrorResponse(value: unknown): value is CaptchaErrorResponse {
  return (
    !!value &&
    typeof value === 'object' &&
    'code' in value &&
    'error' in value &&
    typeof (value as CaptchaErrorResponse).code === 'number' &&
    typeof (value as CaptchaErrorResponse).error === 'string'
  );
}

/** 取整并裁剪到 1-3 的设备像素比，服务端据此选择出图倍率 */
export function getDevicePixelRatio(): number {
  if (typeof window === 'undefined') {
    return 1;
  }
  const dpr = Math.round(window.devicePixelRatio || 1);
  return Math.min(3, Math.max(1, dpr));
}

/** 拼接验证码接口根路径：`${baseURL}/api/v1/captchas` */
export function captchaEndpointFor(api: Partial<ApiConfig>): string {
  return `${normalizeBaseURL(api.baseURL)}${API_PREFIX}/captchas`;
}

/** 组装出题请求体；未提供的字段不写入，避免服务端把 undefined 当作显式值 */
export function buildIssuePayload(option: GetCaptchaOption, env?: CaptchaEnv): CaptchaIssuePayload {
  const payload: CaptchaIssuePayload = { dpr: getDevicePixelRatio() };
  if (option.mode) {
    payload.mode = option.mode;
  }
  if (env) {
    payload.env = env;
  }
  return payload;
}

export function useCaptchaRequest(option: GetCaptchaOption): GetCaptchaResult {
  const [loading, setLoading] = useState<boolean>(false);
  const [data, setData] = useState<CaptchaChallengeData | null>(null);
  const [error, setError] = useState<CaptchaErrorResponse | null>(null);
  const { api } = useCaptchaConfig();
  const captchaEndpoint = captchaEndpointFor(api);

  const get = async (env?: CaptchaEnv): Promise<CaptchaChallengeData> => {
    const configError = apiConfigError(api);
    if (configError) {
      const normalizedError = { code: 400, error: configError };
      setError(normalizedError);
      throw normalizedError;
    }
    setLoading(true);
    setError(null);
    try {
      // 出题用 POST：环境信号随请求体上报，服务端出题前据此加难
      const url = withAppId(`${captchaEndpoint}/${option.type}`, api.appId);
      const res = await request.post<CaptchaChallengeData | CaptchaErrorResponse>(
        url,
        buildIssuePayload(option, env),
      );
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

  const check = async (
    body: CaptchaVerifyPayload | Record<string, unknown>,
  ): Promise<CaptchaVerifySuccessData> => {
    setLoading(true);
    setError(null);
    try {
      // 提交路径使用服务端实际下发的题型（auto 已被服务端解析）
      const resolvedType = data?.type || option.type;
      const url = withAppId(`${captchaEndpoint}/${resolvedType}/${data?.key}/attempts`, api.appId);
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
