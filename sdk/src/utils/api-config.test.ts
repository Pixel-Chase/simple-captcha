import { describe, expect, it } from 'vitest';
import { apiConfigError, withAppId } from './api-config';

describe('apiConfigError', () => {
  it('缺少 appId 时给出明确提示', () => {
    expect(apiConfigError({})).toContain('缺少 appId');
    expect(apiConfigError({ appId: '  ' })).toContain('缺少 appId');
  });

  it('baseURL 只接受 origin 形式', () => {
    expect(apiConfigError({ appId: 'app_x' })).toBeNull();
    expect(apiConfigError({ appId: 'app_x', baseURL: 'https://captcha.example.com' })).toBeNull();
    expect(apiConfigError({ appId: 'app_x', baseURL: 'http://127.0.0.1:3321/' })).toBeNull();
    for (const bad of [
      'https://example.com/captcha',
      'https://example.com?x=1',
      'captcha.example.com',
      'ftp://example.com',
    ]) {
      expect(apiConfigError({ appId: 'app_x', baseURL: bad })).toContain('origin');
    }
  });
});

describe('withAppId', () => {
  it('appId 写入查询参数并转义', () => {
    expect(withAppId('/api/v1/captchas/auto', 'app_console')).toBe(
      '/api/v1/captchas/auto?appId=app_console',
    );
    expect(withAppId('/x', 'a&b c')).toBe('/x?appId=a%26b%20c');
  });
});
