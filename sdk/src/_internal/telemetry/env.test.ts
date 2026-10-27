import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

type EnvModule = typeof import('./env');

/** env.ts 按级别缓存采集结果，每个用例重新加载模块以拿到干净状态 */
async function loadEnvModule(): Promise<EnvModule> {
  vi.resetModules();
  return import('./env');
}

interface BrowserStubOptions {
  webdriver?: boolean;
  automationMarker?: string;
  touchPoints?: number;
}

function stubBrowser(options: BrowserStubOptions = {}) {
  const win: Record<string, unknown> = {
    innerWidth: 1440,
    innerHeight: 900,
    devicePixelRatio: 2,
    screen: { width: 1920, height: 1080 },
  };
  if (options.automationMarker) {
    win[options.automationMarker] = true;
  }
  vi.stubGlobal('window', win);
  vi.stubGlobal('navigator', {
    userAgent:
      'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 Chrome/130.0.0.0 Safari/537.36',
    platform: 'MacIntel',
    languages: ['zh-CN', 'en'],
    maxTouchPoints: options.touchPoints ?? 0,
    hardwareConcurrency: 8,
    deviceMemory: 8,
    webdriver: options.webdriver ?? false,
    plugins: { length: 5 },
  });
  vi.stubGlobal('document', {
    addEventListener: () => {},
    documentElement: { getAttribute: () => null },
    // 没有 createElement：WebGL 采集会静默失败，返回 undefined
  });
}

describe('collectEnv', () => {
  beforeEach(() => {
    vi.unstubAllGlobals();
  });
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('off 级别不采集', async () => {
    stubBrowser();
    const { collectEnv } = await loadEnvModule();
    expect(collectEnv('off')).toBeUndefined();
  });

  it('没有 window 时（SSR）返回 undefined', async () => {
    const { collectEnv } = await loadEnvModule();
    expect(collectEnv('full')).toBeUndefined();
  });

  it('basic 级别采集基础环境信号，不含指纹与 WebGL', async () => {
    stubBrowser();
    const { collectEnv } = await loadEnvModule();
    const env = collectEnv('basic');
    expect(env).toBeDefined();
    expect(env?.ua).toContain('Chrome/130');
    expect(env?.platform).toBe('MacIntel');
    expect(env?.languages).toEqual(['zh-CN', 'en']);
    expect(env?.screen).toEqual([1920, 1080]);
    expect(env?.viewport).toEqual([1440, 900]);
    expect(env?.dpr).toBe(2);
    expect(env?.cores).toBe(8);
    expect(env?.plugins).toBe(5);
    expect(env?.webdriver).toBe(false);
    expect(env?.automation).toEqual([]);
    expect(env?.fp).toBeUndefined();
    expect(env?.webgl).toBeUndefined();
  });

  it('检测 webdriver 与自动化框架痕迹', async () => {
    stubBrowser({ webdriver: true, automationMarker: '__playwright' });
    const { collectEnv } = await loadEnvModule();
    const env = collectEnv('basic');
    expect(env?.webdriver).toBe(true);
    expect(env?.automation).toContain('__playwright');
  });

  it('full 级别输出 32 位十六进制指纹，且对同样的环境稳定', async () => {
    stubBrowser();
    const first = (await loadEnvModule()).collectEnv('full');
    stubBrowser();
    const second = (await loadEnvModule()).collectEnv('full');
    expect(first?.fp).toMatch(/^[0-9a-f]{32}$/);
    expect(first?.fp).toBe(second?.fp);
  });

  it('同一级别重复采集复用缓存，但视口尺寸刷新', async () => {
    stubBrowser();
    const { collectEnv } = await loadEnvModule();
    const before = collectEnv('basic');
    (globalThis as unknown as { window: { innerWidth: number } }).window.innerWidth = 1024;
    const after = collectEnv('basic');
    expect(before?.viewport).toEqual([1440, 900]);
    expect(after?.viewport).toEqual([1024, 900]);
    expect(after?.ua).toBe(before?.ua);
  });
});
