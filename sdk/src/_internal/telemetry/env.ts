import type { CaptchaEnv, TelemetryLevel } from '../../Captcha.types';
import { encodeUtf8, sha256, toHex } from './sha256';

/** 常见自动化框架在 window / document 上留下的痕迹 */
const WINDOW_MARKERS = [
  '__webdriver_evaluate',
  '__selenium_evaluate',
  '__webdriver_script_function',
  '__webdriver_script_func',
  '__webdriver_script_fn',
  '__fxdriver_evaluate',
  '__driver_unwrapped',
  '__webdriver_unwrapped',
  '__driver_evaluate',
  '__selenium_unwrapped',
  '__fxdriver_unwrapped',
  '_Selenium_IDE_Recorder',
  '_selenium',
  'calledSelenium',
  '__nightmare',
  '_phantom',
  'callPhantom',
  '__phantomas',
  'domAutomation',
  'domAutomationController',
  '__playwright',
  '__pw_manual',
  '__puppeteer_evaluation_script__',
  'Cypress',
  'awesomium',
  '__lastWatirAlert',
  '__lastWatirConfirm',
  '__lastWatirPrompt',
];

const DOCUMENT_MARKERS = [
  '__webdriver_evaluate',
  '__selenium_evaluate',
  '__driver_evaluate',
  '__webdriver_script_fn',
  '$cdc_asdjflasutopfhvcZLmcfl_',
  '$chrome_asyncScriptInfo',
];

const CDC_PATTERN = /^\$?cdc_/;

let visibilityChanges = 0;
let visibilityHooked = false;
let cachedEnv: { level: TelemetryLevel; env: CaptchaEnv } | null = null;

function safe<T>(fn: () => T, fallback: T): T {
  try {
    return fn();
  } catch {
    return fallback;
  }
}

function hookVisibility() {
  if (visibilityHooked || typeof document === 'undefined') {
    return;
  }
  visibilityHooked = true;
  document.addEventListener('visibilitychange', () => {
    visibilityChanges += 1;
  });
}

/** 检测自动化框架留下的全局痕迹，返回命中的标记名称 */
export function detectAutomation(): string[] {
  const found: string[] = [];
  if (typeof window === 'undefined') {
    return found;
  }

  safe(() => {
    const win = window as unknown as Record<string, unknown>;
    for (const marker of WINDOW_MARKERS) {
      if (marker in win) {
        found.push(marker);
      }
    }
    for (const name of Object.getOwnPropertyNames(win)) {
      if (CDC_PATTERN.test(name)) {
        found.push(name);
      }
    }
  }, undefined);

  safe(() => {
    const doc = document as unknown as Record<string, unknown>;
    for (const marker of DOCUMENT_MARKERS) {
      if (marker in doc) {
        found.push(`document.${marker}`);
      }
    }
    const html = document.documentElement;
    for (const attr of ['webdriver', 'selenium', 'driver']) {
      if (html && html.getAttribute(attr)) {
        found.push(`html@${attr}`);
      }
    }
  }, undefined);

  return found.slice(0, 16);
}

function webglRenderer(): string | undefined {
  return safe(() => {
    const canvas = document.createElement('canvas');
    const gl = (canvas.getContext('webgl') ||
      canvas.getContext('experimental-webgl')) as WebGLRenderingContext | null;
    if (!gl) {
      return undefined;
    }
    const debugInfo = gl.getExtension('WEBGL_debug_renderer_info');
    const renderer = debugInfo
      ? gl.getParameter(debugInfo.UNMASKED_RENDERER_WEBGL)
      : gl.getParameter(gl.RENDERER);
    return typeof renderer === 'string' ? renderer.slice(0, 200) : undefined;
  }, undefined);
}

function fingerprint(env: CaptchaEnv): string {
  const raw = [
    env.ua,
    env.platform,
    (env.languages || []).join(','),
    (env.screen || []).join('x'),
    env.dpr,
    env.tz,
    env.cores,
    env.memory,
    env.webgl,
    env.touchPoints,
  ].join('|');
  return toHex(sha256(encodeUtf8(raw))).slice(0, 32);
}

type NavigatorWithExtras = Navigator & {
  deviceMemory?: number;
  userAgentData?: { mobile?: boolean; platform?: string };
};

/**
 * 采集运行环境信号。结果按级别缓存，仅视口尺寸与可见性切换次数每次刷新。
 * 不采集任何可直接识别个人的信息；full 级别的指纹只上报哈希。
 */
export function collectEnv(level: TelemetryLevel): CaptchaEnv | undefined {
  if (level === 'off' || typeof window === 'undefined' || typeof navigator === 'undefined') {
    return undefined;
  }
  hookVisibility();

  const viewport: [number, number] = [
    safe(() => window.innerWidth, 0),
    safe(() => window.innerHeight, 0),
  ];

  if (cachedEnv && cachedEnv.level === level) {
    return { ...cachedEnv.env, viewport, visibilityChanges };
  }

  const nav = navigator as NavigatorWithExtras;
  const env: CaptchaEnv = {
    ua: safe(() => nav.userAgent.slice(0, 400), undefined),
    platform: safe(() => nav.userAgentData?.platform || nav.platform, undefined),
    languages: safe(() => Array.from(nav.languages || []).slice(0, 8), []),
    tz: safe(() => Intl.DateTimeFormat().resolvedOptions().timeZone, undefined),
    tzOffset: safe(() => new Date().getTimezoneOffset(), 0),
    screen: [safe(() => window.screen.width, 0), safe(() => window.screen.height, 0)],
    viewport,
    dpr: safe(() => window.devicePixelRatio || 1, 1),
    touchPoints: safe(() => nav.maxTouchPoints || 0, 0),
    cores: safe(() => nav.hardwareConcurrency || 0, 0),
    memory: safe(() => nav.deviceMemory || 0, 0),
    webdriver: safe(() => !!nav.webdriver, false),
    plugins: safe(() => nav.plugins?.length ?? 0, 0),
    automation: detectAutomation(),
    mobile: safe(() => nav.userAgentData?.mobile, undefined),
  };

  if (level === 'full') {
    env.webgl = webglRenderer();
    env.fp = fingerprint(env);
  }

  cachedEnv = { level, env };
  return { ...env, visibilityChanges };
}
