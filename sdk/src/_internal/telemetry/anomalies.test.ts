import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

type AnomaliesModule = typeof import('./anomalies');

async function loadModule(): Promise<AnomaliesModule> {
  vi.resetModules();
  return import('./anomalies');
}

const desktopChromeUA =
  'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36';

interface StubOptions {
  outerWidth?: number;
  outerHeight?: number;
  chromeObject?: boolean;
  notificationPermission?: string;
  permissionState?: string;
  permissionNeverResolves?: boolean;
  mediaDevices?: number | null;
  ua?: string;
}

/** 伪造一个"干净"的桌面 Chrome，再按选项注入异常 */
function stubBrowser(options: StubOptions = {}) {
  // 原生 getter 的替身：__proto__ 的 getter 是真正的 [native code]
  const nativeGetter = Object.getOwnPropertyDescriptor(Object.prototype, '__proto__')?.get;
  class FakeNavigator {}
  for (const prop of ['webdriver', 'plugins', 'languages', 'hardwareConcurrency', 'userAgent']) {
    Object.defineProperty(FakeNavigator.prototype, prop, { get: nativeGetter, configurable: true });
  }

  const win: Record<string, unknown> = {
    outerWidth: options.outerWidth ?? 1440,
    outerHeight: options.outerHeight ?? 900,
    Navigator: FakeNavigator,
  };
  if (options.chromeObject ?? true) {
    win.chrome = { runtime: {} };
  }
  if (options.notificationPermission !== undefined) {
    win.Notification = { permission: options.notificationPermission };
  }
  vi.stubGlobal('window', win);

  // 真实浏览器里 navigator 的属性都来自原型链，实例上没有自有属性；
  // 这里把取值放在 Navigator.prototype 与实例之间的一层原型上，模拟同样的结构
  const values: Record<string, unknown> = {
    userAgent: options.ua ?? desktopChromeUA,
    platform: 'Win32',
    hardwareConcurrency: 8,
  };
  if (options.permissionState !== undefined || options.permissionNeverResolves) {
    values.permissions = {
      query: () =>
        options.permissionNeverResolves
          ? new Promise(() => {})
          : Promise.resolve({ state: options.permissionState }),
    };
  }
  if (options.mediaDevices !== null) {
    const count = options.mediaDevices ?? 2;
    values.mediaDevices = {
      enumerateDevices: () => Promise.resolve(Array.from({ length: count }, () => ({}))),
    };
  }
  const valuesProto = Object.create(FakeNavigator.prototype);
  for (const [key, value] of Object.entries(values)) {
    Object.defineProperty(valuesProto, key, { value, configurable: true, enumerable: true });
  }
  const nav: Record<string, unknown> = Object.create(valuesProto);
  vi.stubGlobal('navigator', nav);
  return { FakeNavigator, nav };
}

describe('collectAnomalies', () => {
  beforeEach(() => {
    vi.unstubAllGlobals();
  });
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('没有 window 时返回空数组', async () => {
    const { collectAnomalies } = await loadModule();
    expect(await collectAnomalies()).toEqual([]);
  });

  it('干净的桌面 Chrome 不产生任何标记（Worker 不可用时静默跳过）', async () => {
    stubBrowser({ notificationPermission: 'default', permissionState: 'prompt' });
    const { collectAnomalies } = await loadModule();
    expect(await collectAnomalies()).toEqual([]);
  });

  it('outerWidth 为 0 产生 zero_outer_size', async () => {
    stubBrowser({ outerWidth: 0 });
    const { collectAnomalies } = await loadModule();
    expect(await collectAnomalies()).toContain('zero_outer_size');
  });

  it('桌面 Chrome 缺少 window.chrome 产生 chrome_object_missing，移动端 UA 不检查', async () => {
    stubBrowser({ chromeObject: false });
    expect(await (await loadModule()).collectAnomalies()).toContain('chrome_object_missing');

    stubBrowser({
      chromeObject: false,
      ua: 'Mozilla/5.0 (Linux; Android 14) AppleWebKit/537.36 Chrome/130.0.0.0 Mobile Safari/537.36',
    });
    expect(await (await loadModule()).collectAnomalies()).not.toContain('chrome_object_missing');
  });

  it('被覆盖的 navigator.webdriver getter 产生 getter_patched，原生 getter 不触发', async () => {
    const { FakeNavigator } = stubBrowser();
    Object.defineProperty(FakeNavigator.prototype, 'webdriver', {
      get: () => false,
      configurable: true,
    });
    const anomalies = await (await loadModule()).collectAnomalies();
    expect(anomalies).toContain('getter_patched:webdriver');
    expect(anomalies).not.toContain('getter_patched:plugins');
  });

  it('实例上直接定义的同名属性同样视为被篡改', async () => {
    const { nav } = stubBrowser();
    Object.defineProperty(nav, 'languages', { value: ['zh-CN'], configurable: true });
    expect(await (await loadModule()).collectAnomalies()).toContain('getter_patched:languages');
  });

  it('Notification.permission 为 default 而 permissions 为 denied 产生 permission_mismatch', async () => {
    stubBrowser({ notificationPermission: 'default', permissionState: 'denied' });
    expect(await (await loadModule()).collectAnomalies()).toContain('permission_mismatch');

    stubBrowser({ notificationPermission: 'default', permissionState: 'granted' });
    expect(await (await loadModule()).collectAnomalies()).not.toContain('permission_mismatch');
  });

  it('没有任何媒体设备产生 no_media_devices，API 不存在时跳过', async () => {
    stubBrowser({ mediaDevices: 0 });
    expect(await (await loadModule()).collectAnomalies()).toContain('no_media_devices');

    stubBrowser({ mediaDevices: null });
    expect(await (await loadModule()).collectAnomalies()).not.toContain('no_media_devices');
  });

  it('超时的检测被放弃，其余结果仍在预算内返回', async () => {
    stubBrowser({
      outerWidth: 0,
      notificationPermission: 'default',
      permissionNeverResolves: true,
    });
    const { collectAnomalies } = await loadModule();
    const started = Date.now();
    const anomalies = await collectAnomalies(120);
    const elapsed = Date.now() - started;
    expect(anomalies).toContain('zero_outer_size');
    expect(anomalies).not.toContain('permission_mismatch');
    expect(elapsed).toBeLessThan(200);
  });

  it('结果按页面生命周期缓存，resetAnomaliesCache 后重新采集', async () => {
    stubBrowser({ outerWidth: 0 });
    const { collectAnomalies, resetAnomaliesCache } = await loadModule();
    expect(await collectAnomalies()).toContain('zero_outer_size');
    (globalThis as unknown as { window: { outerWidth: number } }).window.outerWidth = 1440;
    expect(await collectAnomalies()).toContain('zero_outer_size');
    resetAnomaliesCache();
    expect(await collectAnomalies()).not.toContain('zero_outer_size');
  });
});
