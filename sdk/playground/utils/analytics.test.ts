import assert from 'node:assert/strict';
import { describe, it } from 'node:test';
import type { RiskTelemetryItem } from '../api';
import {
  bucketTelemetryTimeline,
  buildScoreHistogram,
  matchesRiskQuery,
  summarizeCaptchaTypes,
} from './analytics';

const SAMPLE_ITEMS: RiskTelemetryItem[] = [
  {
    kind: 'issue',
    time: '2026-05-01T10:00:00.000Z',
    app: 'app_1',
    type: 'slide-text',
    key: 'k1',
    ip: '10.0.*.*',
    ua: 'Mozilla/5.0 Chrome/120',
  },
  {
    kind: 'verify',
    time: '2026-05-01T10:04:00.000Z',
    app: 'app_1',
    type: 'slide-text',
    key: 'k1',
    score: 12,
    success: true,
    reasons: [],
  },
  {
    kind: 'verify',
    time: '2026-05-01T10:12:00.000Z',
    app: 'app_2',
    type: 'click-text',
    key: 'k2',
    score: 88,
    success: false,
    reasons: ['webdriver(+60)', 'no_trace(+25)'],
  },
  {
    kind: 'verify',
    time: '2026-05-01T10:20:00.000Z',
    app: 'app_2',
    type: 'click-text',
    key: 'k3',
    score: 100,
    success: false,
    reasons: ['untrusted_event(+50)'],
  },
];

describe('bucketTelemetryTimeline', () => {
  it('returns empty array when no valid timestamps exist', () => {
    assert.deepEqual(bucketTelemetryTimeline([]), []);
    assert.deepEqual(bucketTelemetryTimeline([{ kind: 'issue', app: 'app_1' }]), []);
  });

  it('orders buckets chronologically and computes passRate', () => {
    const buckets = bucketTelemetryTimeline(SAMPLE_ITEMS, 4);
    assert.equal(buckets.length, 4);

    const totalIssued = buckets.reduce((sum, b) => sum + b.issued, 0);
    const totalVerified = buckets.reduce((sum, b) => sum + b.verified, 0);
    const totalPassed = buckets.reduce((sum, b) => sum + b.passed, 0);
    const totalFailed = buckets.reduce((sum, b) => sum + b.failed, 0);

    assert.equal(totalIssued, 1);
    assert.equal(totalVerified, 3);
    assert.equal(totalPassed, 1);
    assert.equal(totalFailed, 2);
    assert.equal(buckets[0].issued, 1);
    assert.equal(buckets[0].passRate, 100);
    assert.equal(buckets[3].failed, 1);
    assert.equal(buckets[3].passRate, 0);
  });
});

describe('summarizeCaptchaTypes', () => {
  it('aggregates counts, localized labels, and shares sorted by total', () => {
    const breakdown = summarizeCaptchaTypes(SAMPLE_ITEMS);
    assert.equal(breakdown.length, 2);
    assert.equal(breakdown[0].type, 'click-text');
    assert.equal(breakdown[0].label, '点选文字');
    assert.equal(breakdown[0].total, 2);
    assert.equal(breakdown[1].type, 'slide-text');
    assert.equal(breakdown[1].label, '滑动文字');
    assert.equal(breakdown[1].issued, 1);
    assert.equal(breakdown[1].verified, 1);
    assert.equal(breakdown[1].passed, 1);
  });
});

describe('buildScoreHistogram', () => {
  it('creates 10 buckets from 0-9 to 90-100 including score 100 in last bucket', () => {
    const hist = buildScoreHistogram(SAMPLE_ITEMS);
    assert.equal(hist.length, 10);
    assert.equal(hist[1].label, '10-19');
    assert.equal(hist[1].pass, 1);
    assert.equal(hist[8].label, '80-89');
    assert.equal(hist[8].fail, 1);
    assert.equal(hist[9].label, '90-100');
    assert.equal(hist[9].fail, 1);
  });
});

describe('matchesRiskQuery', () => {
  const names = { app_1: '订单中心', app_2: '登录门户' };

  it('matches app display name, reasons, ip, ua, and key case-insensitively', () => {
    assert.equal(matchesRiskQuery(SAMPLE_ITEMS[0], '订单', names), true);
    assert.equal(matchesRiskQuery(SAMPLE_ITEMS[0], 'chrome/120', names), true);
    assert.equal(matchesRiskQuery(SAMPLE_ITEMS[2], 'webdriver', names), true);
    assert.equal(matchesRiskQuery(SAMPLE_ITEMS[2], 'k2', names), true);
    assert.equal(matchesRiskQuery(SAMPLE_ITEMS[2], 'nonexistent', names), false);
    assert.equal(matchesRiskQuery(SAMPLE_ITEMS[2], '   ', names), true);
  });
});
