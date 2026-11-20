import assert from 'node:assert/strict';
import { describe, it } from 'node:test';
import type { RuntimeSettings } from '../api';
import { buildSettingsPresets, describeDeviation, detectMatchingPreset } from './presets';

const DEFAULTS: RuntimeSettings = {
  captcha: {
    expireSeconds: 180,
    secondVerifyExpireSeconds: 120,
    maxAttempts: 5,
    ipEscalatePerMinute: 20,
    defaultDifficulty: 2,
    minSolveTimeMs: 400,
    bindClient: true,
    imageQuality: 85,
    maxScale: 2,
  },
  pow: {
    enabled: true,
    required: false,
    difficulty: 12,
    maxDifficulty: 18,
  },
  risk: {
    enabled: true,
    mode: 'enforce',
    escalateThreshold: 35,
    blockThreshold: 70,
    stateTtlSeconds: 1800,
    telemetryRetention: 200,
    envScoreCap: 50,
    blockMinBehavior: 20,
    fpReplayThreshold: 5,
    fpReplayWindowSeconds: 300,
  },
};

describe('buildSettingsPresets', () => {
  it('builds 4 valid presets respecting server relational invariants', () => {
    const presets = buildSettingsPresets(DEFAULTS);
    assert.equal(presets.length, 4);
    for (const preset of presets) {
      assert.ok(preset.settings.pow.maxDifficulty >= preset.settings.pow.difficulty);
      assert.ok(preset.settings.risk.blockThreshold >= preset.settings.risk.escalateThreshold);
    }
    const observe = presets.find((p) => p.id === 'observe');
    assert.equal(observe?.settings.risk.mode, 'log');
    const strict = presets.find((p) => p.id === 'strict');
    assert.equal(strict?.settings.pow.required, true);
  });

  it('detects matching preset accurately', () => {
    const presets = buildSettingsPresets(DEFAULTS);
    assert.equal(detectMatchingPreset(DEFAULTS, presets), 'balanced');
    const modified: RuntimeSettings = {
      ...DEFAULTS,
      captcha: { ...DEFAULTS.captcha, maxAttempts: 9 },
    };
    assert.equal(detectMatchingPreset(modified, presets), null);
  });
});

describe('describeDeviation', () => {
  it('reports direction for numbers and changed for bool/mode', () => {
    assert.equal(describeDeviation('number', '2', '2'), null);
    assert.deepEqual(describeDeviation('number', '4', '2'), {
      kind: 'higher',
      label: '高于默认',
    });
    assert.deepEqual(describeDeviation('number', '1', '2'), {
      kind: 'lower',
      label: '低于默认',
    });
    assert.equal(describeDeviation('bool', true, true), null);
    assert.deepEqual(describeDeviation('bool', false, true), {
      kind: 'changed',
      label: '已调整',
    });
    assert.deepEqual(describeDeviation('mode', 'log', 'enforce'), {
      kind: 'changed',
      label: '已调整',
    });
  });
});
