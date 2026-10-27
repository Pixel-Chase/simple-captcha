import { describe, expect, it } from 'vitest';
import { appRejectedError, CaptchaError } from './CaptchaError';

describe('appRejectedError', () => {
  it('1010 / 1011 映射为 APP_REJECTED 并保留原始响应', () => {
    const disabled = { code: 1010, error: 'app invalid' };
    const origin = { code: 1011, error: 'origin not allowed' };

    const a = appRejectedError(disabled);
    expect(a).toBeInstanceOf(CaptchaError);
    expect(a?.code).toBe('APP_REJECTED');
    expect(a?.message).toBe('应用不存在或已停用');
    expect(a?.response).toBe(disabled);

    const b = appRejectedError(origin);
    expect(b?.code).toBe('APP_REJECTED');
    expect(b?.message).toBe('当前页面来源不在应用允许名单内');
    expect(b?.response).toBe(origin);
  });

  it('其他错误不映射', () => {
    expect(appRejectedError({ code: 1003, error: '验证失败' })).toBeNull();
    expect(appRejectedError({ code: 500, error: '验证码获取失败' })).toBeNull();
    expect(appRejectedError(undefined)).toBeNull();
  });
});
