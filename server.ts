import crypto from 'node:crypto';
import path from 'node:path';
import bcrypt from 'bcryptjs';
import express from 'express';
import type { NextFunction, Request, Response } from 'express';
import multer from 'multer';
import {
  type BackgroundEntry,
  type ResolvedType,
  type RuntimeSettings,
  clampDifficulty,
  clampScale,
  defaultRuntimeSettings,
  generateChallenge,
  getProfile,
  loadBackgroundsFromDir,
  parseImageDimensions,
  verifyPow,
} from './server/captchaEngine';

const PORT = Number(process.env.PORT) || 3000;
const HOST = '0.0.0.0';
const CONSOLE_APP_ID = 'app_console';
const SESSION_TTL_MS = Number(process.env.ADMIN_SESSION_TTL) || 24 * 60 * 60 * 1000;
const MAX_UPLOAD_SIZE = Number(process.env.ADMIN_MAX_UPLOAD_SIZE) || 5 * 1024 * 1024;
const MIN_BG_WIDTH = 900;
const MIN_BG_HEIGHT = 600;
const REC_BG_WIDTH = 1800;
const REC_BG_HEIGHT = 1200;
const MAX_BG_PIXELS = 25_000_000;

const SETUP_ALPHABET = 'ABCDEFGHJKLMNPQRSTUVWXYZ23456789';

function generateSetupCode(): string {
  const groups: string[] = [];
  for (let g = 0; g < 3; g++) {
    let part = '';
    for (let i = 0; i < 4; i++) {
      part += SETUP_ALPHABET[crypto.randomInt(0, SETUP_ALPHABET.length)];
    }
    groups.push(part);
  }
  return groups.join('-');
}

function normalizeSetupCode(code: string): string {
  return code.replace(/[\s-]+/g, '').toUpperCase();
}

function sha256Hex(value: string): string {
  return crypto.createHash('sha256').update(value).digest('hex');
}

function randomHex(bytes: number): string {
  return crypto.randomBytes(bytes).toString('hex');
}

function getClientIp(req: Request): string {
  const fwd = req.headers['x-forwarded-for'];
  if (typeof fwd === 'string' && fwd.trim()) {
    return fwd.split(',')[0].trim();
  }
  return req.ip || req.socket.remoteAddress || '127.0.0.1';
}

function clientFingerprint(req: Request): string {
  const ip = getClientIp(req);
  const ua = typeof req.headers['user-agent'] === 'string' ? req.headers['user-agent'] : '';
  return sha256Hex(`${ip}|${ua}`);
}

function maskIp(ip: string): string {
  if (ip.includes('.')) {
    const parts = ip.split('.');
    if (parts.length >= 2) return `${parts[0]}.${parts[1]}.*.*`;
  }
  if (ip.includes(':')) {
    const parts = ip.split(':').slice(0, 2);
    return `${parts.join(':')}:*`;
  }
  return ip;
}

// ---------- In-Memory State ----------

interface AdminAccount {
  username: string;
  passwordHash: string;
  generation: string;
}

interface AdminSession {
  username: string;
  generation: string;
  expiresAt: number; // Unix seconds
}

interface AppRecord {
  id: string;
  name: string;
  secretHash: string;
  secretHint: string;
  enabled: boolean;
  createdAt: string;
  updatedAt: string;
  secretRotatedAt: string;
  lastUsedAt: string | null;
  origins: string[];
  isSystem: boolean;
}

interface ChallengeRecord {
  key: string;
  type: ResolvedType;
  appId: string;
  clientHash: string;
  scale: number;
  difficulty: number;
  maxAttempts: number;
  attempts: number;
  createdAt: number;
  expiresAt: number;
  powNonce: string;
  powBits: number;
  answerData: unknown;
}

interface SecondVerifyRecord {
  appId: string;
  type: ResolvedType;
  expiresAt: number;
}

interface TelemetryRecord {
  kind: 'issue' | 'verify';
  time: string;
  app: string;
  type?: string;
  key?: string;
  client?: string;
  ip?: string;
  ua?: string;
  level?: number;
  base?: number;
  boost?: number;
  envScore?: number;
  behaviorScore?: number;
  score?: number;
  success?: boolean;
  consistent?: boolean;
  reasons?: string[];
  anomalies?: string[] | null;
}

let setupCode = process.env.ADMIN_SETUP_CODE?.trim() || generateSetupCode();
let adminAccount: AdminAccount | null = null;
const adminSessions = new Map<string, AdminSession>();

let settings: RuntimeSettings = defaultRuntimeSettings();

const backgrounds: Map<string, BackgroundEntry> = loadBackgroundsFromDir(
  path.resolve(process.cwd(), 'backend/images'),
);

const apps = new Map<string, AppRecord>();
{
  const now = new Date().toISOString();
  apps.set(CONSOLE_APP_ID, {
    id: CONSOLE_APP_ID,
    name: '管理后台',
    secretHash: '',
    secretHint: 'system',
    enabled: true,
    createdAt: now,
    updatedAt: now,
    secretRotatedAt: now,
    lastUsedAt: null,
    origins: [],
    isSystem: true,
  });
}

const challenges = new Map<string, ChallengeRecord>();
const secondVerifyTokens = new Map<string, SecondVerifyRecord>();
const telemetryLog: TelemetryRecord[] = [];
const ipTimestamps = new Map<string, number[]>();

function pushTelemetry(entry: TelemetryRecord) {
  if (!settings.risk.enabled || settings.risk.telemetryRetention <= 0) {
    return;
  }
  telemetryLog.unshift(entry);
  const limit = Math.min(settings.risk.telemetryRetention, 10000);
  if (telemetryLog.length > limit) {
    telemetryLog.length = limit;
  }
}

function recordIpHit(ip: string): number {
  const now = Date.now();
  const cutoff = now - 60_000;
  const list = (ipTimestamps.get(ip) || []).filter((t) => t > cutoff);
  list.push(now);
  ipTimestamps.set(ip, list);
  return list.length;
}

function cleanExpiredChallenges() {
  const now = Date.now();
  for (const [k, v] of challenges.entries()) {
    if (v.expiresAt <= now) challenges.delete(k);
  }
  for (const [k, v] of secondVerifyTokens.entries()) {
    if (v.expiresAt <= now) secondVerifyTokens.delete(k);
  }
}
setInterval(cleanExpiredChallenges, 60_000).unref();

// ---------- Helpers ----------

function sendError(res: Response, status: number, code: number, error: string) {
  return res.status(status).json({ code, error });
}

function normalizeOrigin(raw: string): string | null {
  try {
    const trimmed = raw.trim();
    if (!trimmed) return null;
    const url = new URL(trimmed);
    if (url.protocol !== 'http:' && url.protocol !== 'https:') return null;
    return url.origin;
  } catch {
    return null;
  }
}

function validateAndNormalizeOrigins(origins: string[]): { ok: true; list: string[] } | { ok: false; error: string } {
  if (origins.length > 20) {
    return { ok: false, error: '每个应用最多配置 20 个允许来源' };
  }
  const seen = new Set<string>();
  const list: string[] = [];
  for (const item of origins) {
    const norm = normalizeOrigin(item);
    if (!norm) {
      return { ok: false, error: `来源格式无效：${item}` };
    }
    if (!seen.has(norm)) {
      seen.add(norm);
      list.push(norm);
    }
  }
  return { ok: true, list };
}

function toAppInfo(app: AppRecord) {
  return {
    id: app.id,
    name: app.name,
    secretHint: app.secretHint,
    enabled: app.enabled,
    createdAt: app.createdAt,
    updatedAt: app.updatedAt,
    secretRotatedAt: app.secretRotatedAt,
    lastUsedAt: app.lastUsedAt,
    origins: app.origins,
    isSystem: app.isSystem,
  };
}

function validateRuntimeSettings(candidate: unknown): { ok: true; value: RuntimeSettings } | { ok: false; error: string } {
  if (!candidate || typeof candidate !== 'object') {
    return { ok: false, error: '参数格式错误' };
  }
  const s = candidate as RuntimeSettings;
  const checks: Array<[boolean, string]> = [
    [Number.isInteger(s.captcha?.expireSeconds) && s.captcha.expireSeconds >= 30 && s.captcha.expireSeconds <= 3600, 'captcha.expireSeconds 必须在 30～3600 之间'],
    [Number.isInteger(s.captcha?.secondVerifyExpireSeconds) && s.captcha.secondVerifyExpireSeconds >= 10 && s.captcha.secondVerifyExpireSeconds <= 3600, 'captcha.secondVerifyExpireSeconds 必须在 10～3600 之间'],
    [Number.isInteger(s.captcha?.maxAttempts) && s.captcha.maxAttempts >= 1 && s.captcha.maxAttempts <= 20, 'captcha.maxAttempts 必须在 1～20 之间'],
    [Number.isInteger(s.captcha?.ipEscalatePerMinute) && s.captcha.ipEscalatePerMinute >= 0 && s.captcha.ipEscalatePerMinute <= 100000, 'captcha.ipEscalatePerMinute 必须在 0～100000 之间'],
    [Number.isInteger(s.captcha?.defaultDifficulty) && s.captcha.defaultDifficulty >= 1 && s.captcha.defaultDifficulty <= 5, 'captcha.defaultDifficulty 必须在 1～5 之间'],
    [Number.isInteger(s.captcha?.minSolveTimeMs) && s.captcha.minSolveTimeMs >= 0 && s.captcha.minSolveTimeMs <= 10000, 'captcha.minSolveTimeMs 必须在 0～10000 之间'],
    [typeof s.captcha?.bindClient === 'boolean', 'captcha.bindClient 必须是布尔值'],
    [Number.isInteger(s.captcha?.imageQuality) && s.captcha.imageQuality >= 55 && s.captcha.imageQuality <= 100, 'captcha.imageQuality 必须在 55～100 之间'],
    [Number.isInteger(s.captcha?.maxScale) && s.captcha.maxScale >= 1 && s.captcha.maxScale <= 3, 'captcha.maxScale 必须在 1～3 之间'],
    [typeof s.pow?.enabled === 'boolean', 'pow.enabled 必须是布尔值'],
    [typeof s.pow?.required === 'boolean', 'pow.required 必须是布尔值'],
    [Number.isInteger(s.pow?.difficulty) && s.pow.difficulty >= 1 && s.pow.difficulty <= 26, 'pow.difficulty 必须在 1～26 之间'],
    [Number.isInteger(s.pow?.maxDifficulty) && s.pow.maxDifficulty >= 1 && s.pow.maxDifficulty <= 26, 'pow.maxDifficulty 必须在 1～26 之间'],
    [s.pow?.maxDifficulty >= s.pow?.difficulty, `pow.maxDifficulty 不能小于 pow.difficulty（${s.pow?.difficulty}）`],
    [typeof s.risk?.enabled === 'boolean', 'risk.enabled 必须是布尔值'],
    [s.risk?.mode === 'enforce' || s.risk?.mode === 'log', 'risk.mode 只能是 enforce 或 log'],
    [Number.isInteger(s.risk?.escalateThreshold) && s.risk.escalateThreshold >= 0 && s.risk.escalateThreshold <= 100, 'risk.escalateThreshold 必须在 0～100 之间'],
    [Number.isInteger(s.risk?.blockThreshold) && s.risk.blockThreshold >= 0 && s.risk.blockThreshold <= 100, 'risk.blockThreshold 必须在 0～100 之间'],
    [s.risk?.escalateThreshold <= s.risk?.blockThreshold, `risk.escalateThreshold 不能大于 risk.blockThreshold（${s.risk?.blockThreshold}）`],
    [Number.isInteger(s.risk?.stateTtlSeconds) && s.risk.stateTtlSeconds >= 60 && s.risk.stateTtlSeconds <= 86400, 'risk.stateTtlSeconds 必须在 60～86400 之间'],
    [Number.isInteger(s.risk?.telemetryRetention) && s.risk.telemetryRetention >= 0 && s.risk.telemetryRetention <= 1000000, 'risk.telemetryRetention 必须在 0～1000000 之间'],
    [Number.isInteger(s.risk?.envScoreCap) && s.risk.envScoreCap >= 0 && s.risk.envScoreCap <= 100, 'risk.envScoreCap 必须在 0～100 之间'],
    [Number.isInteger(s.risk?.blockMinBehavior) && s.risk.blockMinBehavior >= 0 && s.risk.blockMinBehavior <= 100, 'risk.blockMinBehavior 必须在 0～100 之间'],
    [Number.isInteger(s.risk?.fpReplayThreshold) && s.risk.fpReplayThreshold >= 0 && s.risk.fpReplayThreshold <= 1000, 'risk.fpReplayThreshold 必须在 0～1000 之间'],
    [Number.isInteger(s.risk?.fpReplayWindowSeconds) && s.risk.fpReplayWindowSeconds >= 10 && s.risk.fpReplayWindowSeconds <= 86400, 'risk.fpReplayWindowSeconds 必须在 10～86400 之间'],
  ];

  const errs = checks.filter(([valid]) => !valid).map(([, msg]) => msg);
  if (errs.length > 0) {
    return { ok: false, error: errs.join('；') };
  }
  return { ok: true, value: s };
}

function evaluateEnvRisk(env: Record<string, unknown> | undefined): { envScore: number; reasons: string[] } {
  if (!env || !settings.risk.enabled) return { envScore: 0, reasons: [] };
  let score = 0;
  const reasons: string[] = [];
  if (env.webdriver === true) {
    score += 60;
    reasons.push('webdriver(+60)');
  }
  if (Array.isArray(env.automation) && env.automation.length > 0) {
    score += 40;
    reasons.push('automation(+40)');
  }
  if (Array.isArray(env.anomalies) && env.anomalies.length > 0) {
    const pts = Math.min(45, env.anomalies.length * 15);
    score += pts;
    reasons.push(`anomalies(+${pts})`);
  }
  if (settings.risk.envScoreCap > 0) {
    score = Math.min(score, settings.risk.envScoreCap);
  }
  return { envScore: score, reasons };
}

function evaluateBehaviorRisk(trace: Record<string, unknown> | undefined): { behaviorScore: number; reasons: string[] } {
  if (!trace || !settings.risk.enabled) return { behaviorScore: 0, reasons: [] };
  let score = 0;
  const reasons: string[] = [];
  const points = Array.isArray(trace.points) ? trace.points : [];
  const untrusted = typeof trace.untrusted === 'number' ? trace.untrusted : 0;
  if (untrusted > 0) {
    score += 50;
    reasons.push('untrusted_event(+50)');
  }
  if (points.length === 0 && typeof trace.moves === 'number' && trace.moves === 0 && !Array.isArray(trace.clicks)) {
    score += 25;
    reasons.push('no_trace(+25)');
  }
  return { behaviorScore: score, reasons };
}

function consumeSecondVerify(key: string, appId: string): boolean {
  const rec = secondVerifyTokens.get(key);
  if (!rec) return false;
  if (rec.appId !== appId) return false;
  secondVerifyTokens.delete(key);
  return rec.expiresAt > Date.now();
}

function checkAppOrigin(req: Request, res: Response, next: NextFunction) {
  const appId = typeof req.query.appId === 'string' ? req.query.appId.trim() : '';
  if (!appId) {
    return sendError(res, 403, 1010, '应用不存在或已停用');
  }
  const app = apps.get(appId);
  if (!app || !app.enabled) {
    return sendError(res, 403, 1010, '应用不存在或已停用');
  }

  const origin = typeof req.headers.origin === 'string' ? req.headers.origin : '';
  if (origin) {
    res.setHeader('Vary', 'Origin');
    if (app.isSystem) {
      const host = req.headers.host || '';
      let originHost = '';
      try {
        originHost = new URL(origin).host;
      } catch {
        originHost = '';
      }
      if (host && originHost && originHost !== host) {
        return sendError(res, 403, 1011, '当前来源不允许访问该应用');
      }
      res.setHeader('Access-Control-Allow-Origin', origin);
    } else {
      if (!app.origins.includes(origin)) {
        return sendError(res, 403, 1011, '当前来源不允许访问该应用');
      }
      res.setHeader('Access-Control-Allow-Origin', origin);
    }
    res.setHeader('Access-Control-Allow-Methods', 'POST, OPTIONS');
    res.setHeader('Access-Control-Allow-Headers', 'Content-Type');
  } else if (!app.isSystem && app.origins.length === 0) {
    return sendError(res, 403, 1011, '当前来源不允许访问该应用');
  }

  if (req.method === 'OPTIONS') {
    return res.status(204).end();
  }
  (req as Request & { captchaApp?: AppRecord }).captchaApp = app;
  next();
}

function requireAdminAuth(req: Request, res: Response, next: NextFunction) {
  const auth = req.headers.authorization || '';
  const match = /^Bearer\s+(.+)$/i.exec(auth);
  if (!match) {
    return sendError(res, 401, 2001, '未登录或会话已过期');
  }
  const token = match[1].trim();
  const session = adminSessions.get(token);
  if (
    !session ||
    session.expiresAt * 1000 <= Date.now() ||
    !adminAccount ||
    session.generation !== adminAccount.generation
  ) {
    adminSessions.delete(token);
    return sendError(res, 401, 2001, '未登录或会话已过期');
  }
  (req as Request & { adminSession?: AdminSession; adminToken?: string }).adminSession = session;
  (req as Request & { adminToken?: string }).adminToken = token;
  next();
}

function issueSession(acc: AdminAccount): { token: string; username: string; expiresAt: number } {
  const token = randomHex(32);
  const expiresAt = Math.floor((Date.now() + SESSION_TTL_MS) / 1000);
  adminSessions.set(token, {
    username: acc.username,
    generation: acc.generation,
    expiresAt,
  });
  return { token, username: acc.username, expiresAt };
}

// ---------- Express App ----------

async function startServer() {
  const app = express();
  app.set('trust proxy', true);
  app.use(express.json({ limit: '1mb' }));

  const upload = multer({
    storage: multer.memoryStorage(),
    limits: { fileSize: MAX_UPLOAD_SIZE + (1 << 20) },
  });

  // Health & Stats
  const healthHandler = (_req: Request, res: Response) => {
    res.json({
      status: 'ok',
      service: 'simple-captcha',
      version: '3.0.0',
      runtime: 'nodejs',
      timestamp: new Date().toISOString(),
    });
  };
  app.get('/health', healthHandler);
  app.head('/health', healthHandler);

  app.get('/stats', (_req: Request, res: Response) => {
    res.json({
      service: { name: 'simple-captcha', version: '3.0.0', runtime: 'nodejs' },
      challenges: challenges.size,
      backgrounds: backgrounds.size,
      apps: apps.size,
      timestamp: new Date().toISOString(),
    });
  });

  // ---------- Captcha API (/api/v1/captchas) ----------

  const captchaRouter = express.Router();
  captchaRouter.use((_req, res, next) => {
    res.setHeader('Cache-Control', 'no-store, no-cache, must-revalidate');
    res.setHeader('Pragma', 'no-cache');
    res.setHeader('Expires', '0');
    next();
  });

  // S2S state check
  captchaRouter.get('/states/:key', (req, res) => {
    const auth = req.headers.authorization || '';
    const match = /^Bearer\s+(.+)$/i.exec(auth);
    if (!match) {
      return sendError(res, 401, 3001, '应用密钥缺失或无效');
    }
    const secret = match[1].trim();
    const secretHash = sha256Hex(secret);
    let matchedApp: AppRecord | null = null;
    for (const candidate of apps.values()) {
      if (!candidate.isSystem && candidate.enabled && candidate.secretHash === secretHash) {
        matchedApp = candidate;
        break;
      }
    }
    if (!matchedApp) {
      return sendError(res, 401, 3001, '应用密钥缺失或无效');
    }
    matchedApp.lastUsedAt = new Date().toISOString();

    const key = String(req.params.key || '');
    if (!consumeSecondVerify(key, matchedApp.id)) {
      return sendError(res, 200, 1004, '验证码已失效');
    }
    return res.json({ valid: true });
  });

  captchaRouter.options('/:type', checkAppOrigin);
  captchaRouter.post('/:type', checkAppOrigin, (req, res) => {
    const requestedType = String(req.params.type || '');
    const validTypes = ['auto', 'click-text', 'click-shape', 'slide-text', 'slide-region', 'rotate'];
    if (!validTypes.includes(requestedType)) {
      return sendError(res, 400, 400, '不支持的验证码类型');
    }

    const body = (req.body && typeof req.body === 'object' ? req.body : {}) as {
      mode?: 'light' | 'dark';
      dpr?: number;
      env?: Record<string, unknown>;
    };

    const mode: 'light' | 'dark' = body.mode === 'light' ? 'light' : 'dark';
    const scale = clampScale(Number(body.dpr) || 1, settings.captcha.maxScale);
    const ip = getClientIp(req);
    const clientHash = clientFingerprint(req);
    const ipHits = recordIpHit(ip);

    const { envScore, reasons } = evaluateEnvRisk(body.env);
    let boost = 0;
    if (settings.captcha.ipEscalatePerMinute > 0) {
      if (ipHits > settings.captcha.ipEscalatePerMinute * 2) boost += 2;
      else if (ipHits > settings.captcha.ipEscalatePerMinute) boost += 1;
    }
    if (settings.risk.enabled && envScore >= settings.risk.escalateThreshold) {
      boost += 1;
    }

    const base = clampDifficulty(settings.captcha.defaultDifficulty);
    const difficulty = clampDifficulty(base + boost);

    let resolvedType: ResolvedType;
    if (requestedType === 'auto') {
      if (difficulty >= 4) resolvedType = 'click-text';
      else if (difficulty >= 3) resolvedType = 'slide-region';
      else resolvedType = 'slide-text';
    } else {
      resolvedType = requestedType as ResolvedType;
    }

    try {
      const challenge = generateChallenge(resolvedType, backgrounds, difficulty, scale, mode);
      const key = `${Date.now().toString(36)}_${randomHex(8)}`;
      const profile = getProfile(difficulty);
      const powBits = settings.pow.enabled
        ? Math.min(settings.pow.maxDifficulty, settings.pow.difficulty + profile.powExtraBits)
        : 0;
      const powNonce = powBits > 0 ? randomHex(8) : '';
      const captchaApp = (req as Request & { captchaApp: AppRecord }).captchaApp;

      challenges.set(key, {
        key,
        type: resolvedType,
        appId: captchaApp.id,
        clientHash: settings.captcha.bindClient ? clientHash : '',
        scale,
        difficulty,
        maxAttempts: settings.captcha.maxAttempts,
        attempts: 0,
        createdAt: Date.now(),
        expiresAt: Date.now() + settings.captcha.expireSeconds * 1000,
        powNonce,
        powBits,
        answerData: challenge.answerData,
      });

      pushTelemetry({
        kind: 'issue',
        time: new Date().toISOString(),
        app: captchaApp.id,
        type: resolvedType,
        key: key.slice(0, 12),
        client: clientHash.slice(0, 10),
        ip: maskIp(ip),
        ua: typeof req.headers['user-agent'] === 'string' ? req.headers['user-agent'].slice(0, 64) : '',
        level: difficulty,
        base,
        boost,
        envScore,
        reasons,
        anomalies: Array.isArray(body.env?.anomalies) ? (body.env.anomalies as string[]) : null,
      });

      return res.json({
        key,
        type: resolvedType,
        image: challenge.image,
        thumb: challenge.thumb,
        thumbWidth: challenge.thumbWidth,
        thumbHeight: challenge.thumbHeight,
        thumbX: challenge.thumbX,
        thumbY: challenge.thumbY,
        imageWidth: challenge.imageWidth,
        imageHeight: challenge.imageHeight,
        scale: challenge.scale,
        difficulty: challenge.difficulty,
        ...(powBits > 0 ? { pow: { nonce: powNonce, difficulty: powBits } } : {}),
      });
    } catch (err) {
      return sendError(res, 500, 1002, err instanceof Error ? err.message : '验证码生成失败');
    }
  });

  captchaRouter.options('/:type/:key/attempts', checkAppOrigin);
  captchaRouter.post('/:type/:key/attempts', checkAppOrigin, (req, res) => {
    const type = String(req.params.type || '');
    const key = String(req.params.key || '');
    const captchaApp = (req as Request & { captchaApp: AppRecord }).captchaApp;

    const record = challenges.get(key);
    if (!record || record.expiresAt <= Date.now() || record.appId !== captchaApp.id || record.type !== type) {
      return sendError(res, 200, 1004, '验证码已失效');
    }

    if (record.attempts >= record.maxAttempts) {
      challenges.delete(key);
      return sendError(res, 200, 1005, '尝试次数过多，请重新获取验证码');
    }

    record.attempts += 1;
    const ip = getClientIp(req);
    const clientHash = clientFingerprint(req);

    const failAttempt = (code: number, message: string) => {
      if (record.maxAttempts - record.attempts <= 0) {
        challenges.delete(key);
        return sendError(res, 200, 1005, '尝试次数过多，请重新获取验证码');
      }
      return sendError(res, 200, code, message);
    };

    if (settings.captcha.bindClient && record.clientHash && record.clientHash !== clientHash) {
      challenges.delete(key);
      return sendError(res, 200, 1007, '验证码已失效，请重新获取');
    }

    if (settings.captcha.minSolveTimeMs > 0 && Date.now() - record.createdAt < settings.captcha.minSolveTimeMs) {
      return failAttempt(1006, '操作过快，请重试');
    }

    const body = (req.body && typeof req.body === 'object' ? req.body : {}) as {
      dots?: string;
      point?: string;
      angle?: string;
      trace?: Record<string, unknown>;
      env?: Record<string, unknown>;
      pow?: { nonce?: string; solution?: string };
    };

    if (record.powBits > 0) {
      const pow = body.pow;
      if (!pow || !pow.solution) {
        if (settings.pow.required) {
          return failAttempt(1009, '验证失败，请刷新后重试');
        }
      } else if (pow.nonce !== record.powNonce || !verifyPow(record.powNonce, key, pow.solution, record.powBits)) {
        return failAttempt(1009, '验证失败，请刷新后重试');
      }
    }

    const { envScore, reasons: envReasons } = evaluateEnvRisk(body.env);
    const { behaviorScore, reasons: behReasons } = evaluateBehaviorRisk(body.trace);
    const totalScore = envScore + behaviorScore;
    const allReasons = [...envReasons, ...behReasons];

    const isBlocked =
      settings.risk.enabled &&
      settings.risk.mode === 'enforce' &&
      totalScore >= settings.risk.blockThreshold &&
      (behaviorScore >= settings.risk.blockMinBehavior || envReasons.some((r) => r.startsWith('webdriver')));

    const profile = getProfile(record.difficulty);
    let matched = false;

    if (type === 'click-text' || type === 'click-shape') {
      const coords = String(body.dots || '')
        .split(',')
        .map((s) => Number(s.trim()));
      const correctMap = record.answerData as Record<string, { x: number; y: number; width: number; height: number }>;
      const targetCount = Object.keys(correctMap).length;
      if (coords.length === targetCount * 2 && coords.every((n) => Number.isFinite(n))) {
        const tol = profile.clickTolerance * record.scale;
        matched = true;
        for (let i = 0; i < targetCount; i++) {
          const sx = coords[i * 2];
          const sy = coords[i * 2 + 1];
          const box = correctMap[String(i)];
          if (
            sx < box.x - tol ||
            sx > box.x + box.width + tol ||
            sy < box.y - tol ||
            sy > box.y + box.height + tol
          ) {
            matched = false;
            break;
          }
        }
      }
    } else if (type === 'slide-text' || type === 'slide-region') {
      const coords = String(body.point || '')
        .split(',')
        .map((s) => Number(s.trim()));
      const target = record.answerData as { x: number; y: number };
      if (coords.length === 2 && coords.every((n) => Number.isFinite(n))) {
        const tol = profile.slideTolerance * record.scale;
        matched = Math.abs(coords[0] - target.x) <= tol && Math.abs(coords[1] - target.y) <= tol;
      }
    } else if (type === 'rotate') {
      const userAngle = Number(String(body.angle || '').trim());
      const target = record.answerData as { angle: number };
      if (Number.isFinite(userAngle) && userAngle >= 0 && userAngle <= 360) {
        const tol = profile.rotateTolerance;
        const total = userAngle + target.angle;
        matched = total >= 360 - tol && total <= 360 + tol;
      }
    }

    const success = matched && !isBlocked;

    pushTelemetry({
      kind: 'verify',
      time: new Date().toISOString(),
      app: captchaApp.id,
      type,
      key: key.slice(0, 12),
      client: clientHash.slice(0, 10),
      ip: maskIp(ip),
      ua: typeof req.headers['user-agent'] === 'string' ? req.headers['user-agent'].slice(0, 64) : '',
      envScore,
      behaviorScore,
      score: totalScore,
      success,
      consistent: true,
      reasons: allReasons,
      anomalies: Array.isArray(body.env?.anomalies) ? (body.env.anomalies as string[]) : null,
    });

    if (!success) {
      return failAttempt(1003, '验证失败');
    }

    challenges.delete(key);
    const secondKey = `${Date.now().toString(36)}_${randomHex(12)}`;
    secondVerifyTokens.set(secondKey, {
      appId: captchaApp.id,
      type: record.type,
      expiresAt: Date.now() + settings.captcha.secondVerifyExpireSeconds * 1000,
    });

    return res.json({ second_key: secondKey });
  });

  app.use('/api/v1/captchas', captchaRouter);

  // ---------- Admin API (/api/v1/admin) ----------

  const adminRouter = express.Router();
  adminRouter.use((_req, res, next) => {
    res.setHeader('Cache-Control', 'no-store, no-cache, must-revalidate');
    res.setHeader('Pragma', 'no-cache');
    res.setHeader('Expires', '0');
    next();
  });

  adminRouter.get('/setup', (_req, res) => {
    res.json({ initialized: adminAccount !== null });
  });

  adminRouter.post('/setup', (req, res) => {
    if (adminAccount !== null) {
      return sendError(res, 409, 2013, '管理后台已初始化');
    }
    const { setupCode: rawCode, username: rawUsername, password } = (req.body || {}) as {
      setupCode?: string;
      username?: string;
      password?: string;
    };
    if (!rawCode || normalizeSetupCode(rawCode) !== normalizeSetupCode(setupCode)) {
      return sendError(res, 403, 2014, '初始化口令错误');
    }
    const username = typeof rawUsername === 'string' ? rawUsername.trim() : '';
    if (!username || username.length > 64) {
      return sendError(res, 400, 400, '用户名不能为空且不超过 64 个字符');
    }
    if (typeof password !== 'string' || Array.from(password).length < 10 || Buffer.byteLength(password) > 72) {
      return sendError(res, 400, 400, '密码至少需要 10 个字符，且不超过 72 字节');
    }

    adminAccount = {
      username,
      passwordHash: bcrypt.hashSync(password, 10),
      generation: randomHex(16),
    };
    setupCode = '';
    const session = issueSession(adminAccount);
    return res.json(session);
  });

  adminRouter.post('/login', (req, res) => {
    if (!adminAccount) {
      return sendError(res, 409, 2012, '管理后台尚未初始化');
    }
    const { username, password, captchaKey } = (req.body || {}) as {
      username?: string;
      password?: string;
      captchaKey?: string;
    };
    if (!captchaKey || !consumeSecondVerify(captchaKey, CONSOLE_APP_ID)) {
      return sendError(res, 400, 2003, '请先完成行为验证');
    }
    if (
      typeof username !== 'string' ||
      typeof password !== 'string' ||
      username !== adminAccount.username ||
      !bcrypt.compareSync(password, adminAccount.passwordHash)
    ) {
      return sendError(res, 401, 2002, '用户名或密码错误');
    }
    return res.json(issueSession(adminAccount));
  });

  adminRouter.get('/session', requireAdminAuth, (req, res) => {
    const session = (req as Request & { adminSession: AdminSession }).adminSession;
    return res.json({ username: session.username, expiresAt: session.expiresAt });
  });

  adminRouter.delete('/session', requireAdminAuth, (req, res) => {
    const token = (req as Request & { adminToken: string }).adminToken;
    adminSessions.delete(token);
    return res.status(204).end();
  });

  adminRouter.put('/password', requireAdminAuth, (req, res) => {
    if (!adminAccount) {
      return sendError(res, 401, 2001, '未登录或会话已过期');
    }
    const { currentPassword, newPassword } = (req.body || {}) as {
      currentPassword?: string;
      newPassword?: string;
    };
    if (typeof currentPassword !== 'string' || !bcrypt.compareSync(currentPassword, adminAccount.passwordHash)) {
      return sendError(res, 400, 2009, '当前密码错误');
    }
    if (
      typeof newPassword !== 'string' ||
      Array.from(newPassword).length < 10 ||
      Buffer.byteLength(newPassword) > 72
    ) {
      return sendError(res, 400, 400, '新密码至少需要 10 个字符，且不超过 72 字节');
    }
    if (newPassword === currentPassword) {
      return sendError(res, 400, 400, '新密码不能与当前密码相同');
    }
    adminAccount = {
      username: adminAccount.username,
      passwordHash: bcrypt.hashSync(newPassword, 10),
      generation: randomHex(16),
    };
    adminSessions.clear();
    return res.json(issueSession(adminAccount));
  });

  // Backgrounds
  adminRouter.get('/backgrounds', requireAdminAuth, (_req, res) => {
    const items = Array.from(backgrounds.values())
      .map(({ name, size, width, height, format, modifiedAt }) => ({
        name,
        size,
        width,
        height,
        format,
        modifiedAt,
      }))
      .sort((a, b) => b.modifiedAt.localeCompare(a.modifiedAt) || a.name.localeCompare(b.name));

    res.json({
      items,
      total: items.length,
      constraints: {
        minWidth: MIN_BG_WIDTH,
        minHeight: MIN_BG_HEIGHT,
        recommendedWidth: REC_BG_WIDTH,
        recommendedHeight: REC_BG_HEIGHT,
        maxUploadSize: MAX_UPLOAD_SIZE,
        maxPixels: MAX_BG_PIXELS,
        allowedFormats: ['jpeg', 'png'],
      },
    });
  });

  adminRouter.post('/backgrounds', requireAdminAuth, upload.single('file'), (req, res) => {
    const file = req.file;
    if (!file) {
      return sendError(res, 400, 400, '缺少 file 字段或表单格式错误');
    }
    if (file.size > MAX_UPLOAD_SIZE) {
      return sendError(res, 413, 2005, '图片体积超出限制');
    }
    if (file.size === 0) {
      return sendError(res, 400, 2004, '上传文件为空');
    }
    const dims = parseImageDimensions(file.buffer);
    if (!dims) {
      return sendError(res, 400, 2004, '仅支持 JPEG 或 PNG 图片');
    }
    if (dims.width < MIN_BG_WIDTH || dims.height < MIN_BG_HEIGHT) {
      return sendError(
        res,
        400,
        2004,
        `图片尺寸至少需要 ${MIN_BG_WIDTH}x${MIN_BG_HEIGHT}，当前为 ${dims.width}x${dims.height}`,
      );
    }
    if (dims.width * dims.height > MAX_BG_PIXELS) {
      return sendError(res, 400, 2004, `图片像素总数不能超过 ${MAX_BG_PIXELS}`);
    }

    const base = path.basename(file.originalname, path.extname(file.originalname));
    const stem = base.replace(/[^A-Za-z0-9_-]+/g, '-').replace(/^[-_]+|[-_]+$/g, '').slice(0, 40) || 'bg';
    const ext = dims.format === 'jpeg' ? '.jpg' : '.png';
    const name = `${stem}-${randomHex(4)}${ext}`;
    const mimeType = dims.format === 'png' ? 'image/png' : 'image/jpeg';
    const entry: BackgroundEntry = {
      name,
      size: file.size,
      width: dims.width,
      height: dims.height,
      format: dims.format,
      modifiedAt: new Date().toISOString(),
      mimeType,
      buffer: file.buffer,
      dataUri: `data:${mimeType};base64,${file.buffer.toString('base64')}`,
    };
    backgrounds.set(name, entry);

    return res.json({
      name: entry.name,
      size: entry.size,
      width: entry.width,
      height: entry.height,
      format: entry.format,
      modifiedAt: entry.modifiedAt,
    });
  });

  adminRouter.get('/backgrounds/:name', requireAdminAuth, (req, res) => {
    const name = String(req.params.name || '');
    const entry = backgrounds.get(name);
    if (!entry) {
      return sendError(res, 404, 2007, '背景图不存在');
    }
    res.setHeader('Cache-Control', 'private, max-age=300');
    res.removeHeader('Pragma');
    res.removeHeader('Expires');
    res.setHeader('Content-Type', entry.mimeType);
    res.setHeader('Content-Disposition', 'inline');
    return res.send(entry.buffer);
  });

  adminRouter.delete('/backgrounds/:name', requireAdminAuth, (req, res) => {
    const name = String(req.params.name || '');
    if (!backgrounds.has(name)) {
      return sendError(res, 404, 2007, '背景图不存在');
    }
    if (backgrounds.size <= 1) {
      return sendError(res, 409, 2006, '至少需要保留一张背景图');
    }
    backgrounds.delete(name);
    return res.status(204).end();
  });

  // Risk Telemetry
  adminRouter.get('/risk/telemetry', requireAdminAuth, (req, res) => {
    const rawLimit = Number(req.query.limit) || 100;
    const limit = Math.min(500, Math.max(1, Math.round(rawLimit)));
    return res.json({
      items: telemetryLog.slice(0, limit),
      limit,
    });
  });

  // Apps CRUD
  adminRouter.get('/apps', requireAdminAuth, (_req, res) => {
    const list = Array.from(apps.values())
      .sort((a, b) => Number(b.isSystem) - Number(a.isSystem) || a.createdAt.localeCompare(b.createdAt))
      .map(toAppInfo);
    return res.json({ items: list, total: list.length });
  });

  adminRouter.post('/apps', requireAdminAuth, (req, res) => {
    const { name: rawName, origins: rawOrigins } = (req.body || {}) as {
      name?: string;
      origins?: string[];
    };
    const name = typeof rawName === 'string' ? rawName.trim() : '';
    if (!name || Array.from(name).length > 40) {
      return sendError(res, 400, 400, '应用名称不能为空且不超过 40 个字符');
    }
    for (const existing of apps.values()) {
      if (existing.name.toLowerCase() === name.toLowerCase()) {
        return sendError(res, 409, 2011, '已存在同名应用');
      }
    }
    const originsRes = validateAndNormalizeOrigins(Array.isArray(rawOrigins) ? rawOrigins : []);
    if (!originsRes.ok) {
      return sendError(res, 400, 400, originsRes.error);
    }

    const id = `app_${randomHex(6)}`;
    const secret = `sk_${randomHex(24)}`;
    const now = new Date().toISOString();
    const record: AppRecord = {
      id,
      name,
      secretHash: sha256Hex(secret),
      secretHint: secret.slice(0, 10),
      enabled: true,
      createdAt: now,
      updatedAt: now,
      secretRotatedAt: now,
      lastUsedAt: null,
      origins: originsRes.list,
      isSystem: false,
    };
    apps.set(id, record);
    return res.json({ app: toAppInfo(record), secret });
  });

  adminRouter.put('/apps/:id', requireAdminAuth, (req, res) => {
    const id = String(req.params.id || '');
    const record = apps.get(id);
    if (!record) {
      return sendError(res, 404, 2010, '应用不存在');
    }
    if (record.isSystem) {
      return sendError(res, 400, 2015, '系统应用不可修改');
    }
    const { name: rawName, enabled, origins: rawOrigins } = (req.body || {}) as {
      name?: string;
      enabled?: boolean;
      origins?: string[];
    };
    const name = typeof rawName === 'string' ? rawName.trim() : '';
    if (!name || Array.from(name).length > 40 || typeof enabled !== 'boolean') {
      return sendError(res, 400, 400, '参数格式错误');
    }
    for (const existing of apps.values()) {
      if (existing.id !== id && existing.name.toLowerCase() === name.toLowerCase()) {
        return sendError(res, 409, 2011, '已存在同名应用');
      }
    }
    let nextOrigins = record.origins;
    if (rawOrigins !== undefined) {
      const originsRes = validateAndNormalizeOrigins(Array.isArray(rawOrigins) ? rawOrigins : []);
      if (!originsRes.ok) {
        return sendError(res, 400, 400, originsRes.error);
      }
      nextOrigins = originsRes.list;
    }

    record.name = name;
    record.enabled = enabled;
    record.origins = nextOrigins;
    record.updatedAt = new Date().toISOString();
    return res.json(toAppInfo(record));
  });

  adminRouter.post('/apps/:id/secret', requireAdminAuth, (req, res) => {
    const id = String(req.params.id || '');
    const record = apps.get(id);
    if (!record) {
      return sendError(res, 404, 2010, '应用不存在');
    }
    if (record.isSystem) {
      return sendError(res, 400, 2015, '系统应用不可修改');
    }
    const secret = `sk_${randomHex(24)}`;
    const now = new Date().toISOString();
    record.secretHash = sha256Hex(secret);
    record.secretHint = secret.slice(0, 10);
    record.secretRotatedAt = now;
    record.updatedAt = now;
    return res.json({ app: toAppInfo(record), secret });
  });

  adminRouter.delete('/apps/:id', requireAdminAuth, (req, res) => {
    const id = String(req.params.id || '');
    const record = apps.get(id);
    if (!record) {
      return sendError(res, 404, 2010, '应用不存在');
    }
    if (record.isSystem) {
      return sendError(res, 400, 2015, '系统应用不可修改');
    }
    apps.delete(id);
    return res.status(204).end();
  });

  // Settings
  adminRouter.get('/settings', requireAdminAuth, (_req, res) => {
    return res.json({
      settings,
      defaults: defaultRuntimeSettings(),
    });
  });

  adminRouter.put('/settings', requireAdminAuth, (req, res) => {
    const result = validateRuntimeSettings(req.body);
    if (!result.ok) {
      return sendError(res, 400, 400, result.error);
    }
    settings = result.value;
    return res.json({ settings });
  });

  app.use('/api/v1/admin', adminRouter);

  // ---------- Frontend Serving (Vite dev middleware or built static) ----------

  if (process.env.NODE_ENV === 'production') {
    const distDir = path.resolve(process.cwd(), 'sdk/playground-dist');
    app.use(express.static(distDir));
    app.get('*', (_req, res) => {
      res.sendFile(path.join(distDir, 'index.html'));
    });
  } else {
    const { createServer: createViteServer } = await import('vite');
    const vite = await createViteServer({
      configFile: path.resolve(process.cwd(), 'sdk/vite.config.ts'),
      server: {
        middlewareMode: true,
        hmr: false,
      },
    });
    app.use(vite.middlewares);
  }

  app.listen(PORT, HOST, () => {
    console.log(`simple-captcha server listening on http://${HOST}:${PORT}`);
    console.log(`Loaded ${backgrounds.size} background images from backend/images`);
    if (setupCode) {
      console.log('====================================================================');
      console.log('  管理后台尚未初始化');
      console.log(`      打开控制台:   http://${HOST}:${PORT}/`);
      console.log(`      初始化口令:   ${setupCode}`);
      console.log('====================================================================');
    }
  });
}

startServer().catch((err) => {
  console.error('Failed to start server:', err);
  process.exit(1);
});
