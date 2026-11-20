import type { RuntimeSettings } from '../api';

export type PresetId = 'lenient' | 'balanced' | 'strict' | 'observe';

export interface SettingsPreset {
  id: PresetId;
  name: string;
  badge: string;
  desc: string;
  settings: RuntimeSettings;
}

export interface DeviationBadge {
  kind: 'higher' | 'lower' | 'changed';
  label: string;
}

function cloneSettings(s: RuntimeSettings): RuntimeSettings {
  return {
    captcha: { ...s.captcha },
    pow: { ...s.pow },
    risk: { ...s.risk },
  };
}

/** 基于服务端下发的默认参数构建四套典型场景预设 */
export function buildSettingsPresets(defaults: RuntimeSettings): SettingsPreset[] {
  const balanced = cloneSettings(defaults);

  const lenient = cloneSettings(defaults);
  lenient.captcha.defaultDifficulty = 1;
  lenient.captcha.maxAttempts = Math.min(20, Math.max(5, defaults.captcha.maxAttempts + 2));
  lenient.captcha.minSolveTimeMs = Math.min(defaults.captcha.minSolveTimeMs, 250);
  lenient.pow.enabled = false;
  lenient.pow.required = false;
  lenient.risk.enabled = true;
  lenient.risk.mode = 'enforce';
  lenient.risk.escalateThreshold = 55;
  lenient.risk.blockThreshold = 85;
  lenient.risk.blockMinBehavior = 35;

  const strict = cloneSettings(defaults);
  strict.captcha.defaultDifficulty = Math.min(
    5,
    Math.max(3, defaults.captcha.defaultDifficulty + 1),
  );
  strict.captcha.minSolveTimeMs = Math.max(600, defaults.captcha.minSolveTimeMs);
  strict.pow.enabled = true;
  strict.pow.required = true;
  strict.pow.difficulty = Math.min(26, Math.max(16, defaults.pow.difficulty + 2));
  strict.pow.maxDifficulty = Math.min(
    26,
    Math.max(strict.pow.difficulty + 4, defaults.pow.maxDifficulty),
  );
  strict.risk.enabled = true;
  strict.risk.mode = 'enforce';
  strict.risk.escalateThreshold = 25;
  strict.risk.blockThreshold = 55;
  strict.risk.blockMinBehavior = 15;

  const observe = cloneSettings(defaults);
  observe.pow.enabled = false;
  observe.pow.required = false;
  observe.risk.enabled = true;
  observe.risk.mode = 'log';
  observe.risk.telemetryRetention = Math.max(500, defaults.risk.telemetryRetention);

  return [
    {
      id: 'balanced',
      name: '平衡默认',
      badge: '推荐',
      desc: '恢复出厂推荐参数，兼顾真人通过率与基础脚本拦截。',
      settings: balanced,
    },
    {
      id: 'lenient',
      name: '宽松体验',
      badge: '低摩擦',
      desc: '关闭 PoW 并降低默认难度、提高拦截阈值，适合低风险前台页面。',
      settings: lenient,
    },
    {
      id: 'strict',
      name: '严格拦截',
      badge: '高防护',
      desc: '强制 PoW 校验并收紧风险分阈值，适合登录、领券或遭遇攻击时启用。',
      settings: strict,
    },
    {
      id: 'observe',
      name: '仅日志观察',
      badge: '灰度',
      desc: '保留风控评分但切换为 log 模式仅记录不拦截，方便调参前收集样本。',
      settings: observe,
    },
  ];
}

function settingsEqual(a: RuntimeSettings, b: RuntimeSettings): boolean {
  const keysA = Object.keys(a) as Array<keyof RuntimeSettings>;
  for (const group of keysA) {
    const groupA = a[group] as Record<string, unknown>;
    const groupB = b[group] as Record<string, unknown>;
    for (const key of Object.keys(groupA)) {
      if (groupA[key] !== groupB[key]) {
        return false;
      }
    }
  }
  return true;
}

/** 识别当前配置是否完整匹配某套预设 */
export function detectMatchingPreset(
  current: RuntimeSettings,
  presets: ReadonlyArray<SettingsPreset>,
): PresetId | null {
  for (const preset of presets) {
    if (settingsEqual(current, preset.settings)) {
      return preset.id;
    }
  }
  return null;
}

/** 对比表单项相对默认值的偏离方向 */
export function describeDeviation(
  kind: 'number' | 'bool' | 'mode',
  currentValue: string | boolean,
  defaultValue: string | boolean,
): DeviationBadge | null {
  if (kind === 'number') {
    const cur = Number(String(currentValue).trim());
    const def = Number(String(defaultValue).trim());
    if (!Number.isFinite(cur) || !Number.isFinite(def) || cur === def) {
      return null;
    }
    return cur > def ? { kind: 'higher', label: '高于默认' } : { kind: 'lower', label: '低于默认' };
  }
  if (String(currentValue) === String(defaultValue)) {
    return null;
  }
  return { kind: 'changed', label: '已调整' };
}
