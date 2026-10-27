import type { CaptchaErrorResponse } from './Captcha.types';

/**
 * 验证未成功时的原因
 * - CANCELLED：用户关闭了弹窗
 * - LOAD_FAILED：题目获取失败（网络异常、限流、缺少 appId 或服务端拒绝）
 * - MAX_ATTEMPTS：连续答错达到 maxAttempts，弹窗已关闭
 * - ABORTED：useCaptcha 的本次 verify 被新的 verify 替换，或组件已卸载
 * - APP_REJECTED：服务端拒绝了本应用的请求——应用不存在或已停用，或当前页面来源不在应用允许名单内
 */
export type CaptchaErrorCode =
  | 'CANCELLED'
  | 'LOAD_FAILED'
  | 'MAX_ATTEMPTS'
  | 'ABORTED'
  | 'APP_REJECTED';

export class CaptchaError extends Error {
  readonly code: CaptchaErrorCode;
  /** 服务端返回的原始错误，LOAD_FAILED / MAX_ATTEMPTS 时存在 */
  readonly response?: CaptchaErrorResponse;

  constructor(code: CaptchaErrorCode, message: string, response?: CaptchaErrorResponse) {
    super(message);
    this.name = 'CaptchaError';
    this.code = code;
    this.response = response;
  }
}

/** 服务端拒绝应用请求的错误码：1010 应用不存在或已停用，1011 来源不在允许名单内 */
const APP_REJECTED_MESSAGES: Record<number, string> = {
  1010: '应用不存在或已停用',
  1011: '当前页面来源不在应用允许名单内',
};

/** 服务端返回应用被拒绝的错误时转换为 APP_REJECTED，其余错误返回 null */
export function appRejectedError(response: CaptchaErrorResponse | undefined): CaptchaError | null {
  const message = response ? APP_REJECTED_MESSAGES[response.code] : undefined;
  return message ? new CaptchaError('APP_REJECTED', message, response) : null;
}
