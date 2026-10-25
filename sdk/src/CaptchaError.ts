import type { CaptchaErrorResponse } from './Captcha.types';

/**
 * 验证未成功时的原因
 * - CANCELLED：用户关闭了弹窗
 * - LOAD_FAILED：题目获取失败（网络异常、限流或服务端拒绝）
 * - MAX_ATTEMPTS：连续答错达到 maxAttempts，弹窗已关闭
 * - ABORTED：useCaptcha 的本次 verify 被新的 verify 替换，或组件已卸载
 */
export type CaptchaErrorCode = 'CANCELLED' | 'LOAD_FAILED' | 'MAX_ATTEMPTS' | 'ABORTED';

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
