import { afterEach, describe, expect, it, vi } from 'vitest';
import type { CaptchaEnv } from '../Captcha.types';
import {
  buildIssuePayload,
  captchaEndpointFor,
  getDevicePixelRatio,
  isCaptchaErrorResponse,
} from './use-captcha-request';

const env: CaptchaEnv = {
  ua: 'Mozilla/5.0 test',
  languages: ['zh-CN'],
  screen: [1920, 1080],
  viewport: [1440, 900],
  dpr: 2,
  webdriver: false,
};

describe('captchaEndpointFor', () => {
  it('默认前缀 /api/v1', () => {
    expect(captchaEndpointFor({})).toBe('/api/v1/captchas');
  });

  it('规范化 basePath 与 prefix', () => {
    expect(
      captchaEndpointFor({ basePath: 'https://captcha.example.com/', prefix: '/captcha/api/' }),
    ).toBe('https://captcha.example.com/captcha/api/captchas');
  });

  it('空 prefix 挂在根路径', () => {
    expect(captchaEndpointFor({ prefix: '/' })).toBe('/captchas');
    expect(captchaEndpointFor({ basePath: 'http://localhost:8080', prefix: '' })).toBe(
      'http://localhost:8080/captchas',
    );
  });
});

describe('buildIssuePayload', () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('携带 mode、dpr 与 env', () => {
    vi.stubGlobal('window', { devicePixelRatio: 2 });
    const payload = buildIssuePayload({ type: 'auto', mode: 'dark' }, env);
    expect(payload).toEqual({ mode: 'dark', dpr: 2, env });
  });

  it('telemetry=off 时不写 env 键，未指定 mode 时不写 mode 键', () => {
    const payload = buildIssuePayload({ type: 'slide-text' }, undefined);
    expect(payload).toEqual({ dpr: 1 });
    expect(Object.keys(payload)).toEqual(['dpr']);
  });

  it('dpr 取整并裁剪到 1-3', () => {
    vi.stubGlobal('window', { devicePixelRatio: 3.7 });
    expect(getDevicePixelRatio()).toBe(3);
    vi.stubGlobal('window', { devicePixelRatio: 0.4 });
    expect(getDevicePixelRatio()).toBe(1);
    vi.stubGlobal('window', { devicePixelRatio: 1.5 });
    expect(getDevicePixelRatio()).toBe(2);
  });
});

describe('isCaptchaErrorResponse', () => {
  it('识别服务端错误体', () => {
    expect(isCaptchaErrorResponse({ code: 1003, error: '验证失败' })).toBe(true);
    expect(isCaptchaErrorResponse({ key: 'abc', image: 'data:' })).toBe(false);
    expect(isCaptchaErrorResponse(null)).toBe(false);
    expect(isCaptchaErrorResponse({ code: '400', error: 'x' })).toBe(false);
  });
});
