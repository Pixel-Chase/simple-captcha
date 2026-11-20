import crypto from 'node:crypto';
import fs from 'node:fs';
import path from 'node:path';

export interface BackgroundEntry {
  name: string;
  size: number;
  width: number;
  height: number;
  format: 'jpeg' | 'png';
  modifiedAt: string;
  mimeType: string;
  buffer: Buffer;
  dataUri: string;
}

export interface RuntimeSettings {
  captcha: {
    expireSeconds: number;
    secondVerifyExpireSeconds: number;
    maxAttempts: number;
    ipEscalatePerMinute: number;
    defaultDifficulty: number;
    minSolveTimeMs: number;
    bindClient: boolean;
    imageQuality: number;
    maxScale: number;
  };
  pow: {
    enabled: boolean;
    required: boolean;
    difficulty: number;
    maxDifficulty: number;
  };
  risk: {
    enabled: boolean;
    mode: 'enforce' | 'log';
    escalateThreshold: number;
    blockThreshold: number;
    stateTtlSeconds: number;
    telemetryRetention: number;
    envScoreCap: number;
    blockMinBehavior: number;
    fpReplayThreshold: number;
    fpReplayWindowSeconds: number;
  };
}

export function defaultRuntimeSettings(): RuntimeSettings {
  return {
    captcha: {
      expireSeconds: 300,
      secondVerifyExpireSeconds: 120,
      maxAttempts: 5,
      ipEscalatePerMinute: 30,
      defaultDifficulty: 3,
      minSolveTimeMs: 500,
      bindClient: true,
      imageQuality: 80,
      maxScale: 3,
    },
    pow: {
      enabled: true,
      required: true,
      difficulty: 14,
      maxDifficulty: 20,
    },
    risk: {
      enabled: true,
      mode: 'enforce',
      escalateThreshold: 55,
      blockThreshold: 85,
      stateTtlSeconds: 1800,
      telemetryRetention: 10000,
      envScoreCap: 60,
      blockMinBehavior: 20,
      fpReplayThreshold: 5,
      fpReplayWindowSeconds: 600,
    },
  };
}

interface DifficultyProfile {
  level: number;
  clickLen: [number, number];
  clickVerifyLen: [number, number];
  clickFontSize: [number, number];
  clickTolerance: number;
  slideGraphSize: [number, number];
  slideTolerance: number;
  rotateTolerance: number;
  powExtraBits: number;
}

const DIFFICULTY_PROFILES: Record<number, DifficultyProfile> = {
  1: {
    level: 1,
    clickLen: [3, 4],
    clickVerifyLen: [2, 2],
    clickFontSize: [28, 34],
    clickTolerance: 30,
    slideGraphSize: [54, 62],
    slideTolerance: 8,
    rotateTolerance: 8,
    powExtraBits: 0,
  },
  2: {
    level: 2,
    clickLen: [4, 5],
    clickVerifyLen: [2, 3],
    clickFontSize: [26, 32],
    clickTolerance: 27,
    slideGraphSize: [50, 58],
    slideTolerance: 7,
    rotateTolerance: 7,
    powExtraBits: 0,
  },
  3: {
    level: 3,
    clickLen: [4, 6],
    clickVerifyLen: [2, 4],
    clickFontSize: [26, 32],
    clickTolerance: 25,
    slideGraphSize: [46, 54],
    slideTolerance: 6,
    rotateTolerance: 6,
    powExtraBits: 1,
  },
  4: {
    level: 4,
    clickLen: [5, 7],
    clickVerifyLen: [3, 4],
    clickFontSize: [24, 30],
    clickTolerance: 20,
    slideGraphSize: [42, 50],
    slideTolerance: 5,
    rotateTolerance: 5,
    powExtraBits: 3,
  },
  5: {
    level: 5,
    clickLen: [6, 8],
    clickVerifyLen: [3, 5],
    clickFontSize: [22, 28],
    clickTolerance: 16,
    slideGraphSize: [38, 46],
    slideTolerance: 4,
    rotateTolerance: 4,
    powExtraBits: 4,
  },
};

export function clampDifficulty(level: number): number {
  if (!Number.isFinite(level) || level <= 0) return 3;
  return Math.min(5, Math.max(1, Math.round(level)));
}

export function clampScale(dpr: number, maxScale: number): number {
  const safeMax = Number.isFinite(maxScale) ? Math.min(3, Math.max(1, Math.round(maxScale))) : 3;
  const rounded = Number.isFinite(dpr) && dpr > 0 ? Math.round(dpr) : 1;
  const floor = Math.min(2, safeMax);
  return Math.min(safeMax, Math.max(floor, rounded));
}

export function getProfile(level: number): DifficultyProfile {
  return DIFFICULTY_PROFILES[clampDifficulty(level)];
}

/** Parse width/height from JPEG or PNG binary buffers without native addons */
export function parseImageDimensions(
  buf: Buffer,
): { format: 'jpeg' | 'png'; width: number; height: number } | null {
  if (buf.length < 24) return null;

  // PNG signature: 89 50 4E 47 0D 0A 1A 0A
  if (
    buf[0] === 0x89 &&
    buf[1] === 0x50 &&
    buf[2] === 0x4e &&
    buf[3] === 0x47 &&
    buf[4] === 0x0d &&
    buf[5] === 0x0a &&
    buf[6] === 0x1a &&
    buf[7] === 0x0a
  ) {
    const width = buf.readUInt32BE(16);
    const height = buf.readUInt32BE(20);
    if (width > 0 && height > 0) {
      return { format: 'png', width, height };
    }
    return null;
  }

  // JPEG signature: FF D8
  if (buf[0] === 0xff && buf[1] === 0xd8) {
    let offset = 2;
    while (offset + 9 < buf.length) {
      if (buf[offset] !== 0xff) {
        offset += 1;
        continue;
      }
      while (offset < buf.length && buf[offset] === 0xff) {
        offset += 1;
      }
      if (offset >= buf.length) break;
      const marker = buf[offset];
      offset += 1;
      if (marker === 0xd8 || marker === 0xd9 || marker === 0x01 || (marker >= 0xd0 && marker <= 0xd7)) {
        continue;
      }
      if (offset + 2 > buf.length) break;
      const segLen = buf.readUInt16BE(offset);
      if (segLen < 2 || offset + segLen > buf.length) break;
      const isSOF =
        (marker >= 0xc0 && marker <= 0xc3) ||
        (marker >= 0xc5 && marker <= 0xc7) ||
        (marker >= 0xc9 && marker <= 0xcb) ||
        (marker >= 0xcd && marker <= 0xcf);
      if (isSOF && offset + 7 < buf.length) {
        const height = buf.readUInt16BE(offset + 3);
        const width = buf.readUInt16BE(offset + 5);
        if (width > 0 && height > 0) {
          return { format: 'jpeg', width, height };
        }
        return null;
      }
      offset += segLen;
    }
  }

  return null;
}

export function loadBackgroundsFromDir(dir: string): Map<string, BackgroundEntry> {
  const store = new Map<string, BackgroundEntry>();
  if (!fs.existsSync(dir)) return store;
  const files = fs.readdirSync(dir).sort();
  for (const name of files) {
    if (!/\.(jpe?g|png)$/i.test(name)) continue;
    const full = path.join(dir, name);
    const stat = fs.statSync(full);
    if (!stat.isFile()) continue;
    const buffer = fs.readFileSync(full);
    const dims = parseImageDimensions(buffer);
    if (!dims) continue;
    const mimeType = dims.format === 'png' ? 'image/png' : 'image/jpeg';
    store.set(name, {
      name,
      size: stat.size,
      width: dims.width,
      height: dims.height,
      format: dims.format,
      modifiedAt: stat.mtime.toISOString(),
      mimeType,
      buffer,
      dataUri: `data:${mimeType};base64,${buffer.toString('base64')}`,
    });
  }
  return store;
}

function randInt(min: number, max: number): number {
  const lo = Math.ceil(min);
  const hi = Math.floor(max);
  if (hi <= lo) return lo;
  return crypto.randomInt(lo, hi + 1);
}

function pickRandom<T>(arr: readonly T[]): T {
  return arr[crypto.randomInt(0, arr.length)];
}

function shuffle<T>(arr: T[]): T[] {
  const copy = [...arr];
  for (let i = copy.length - 1; i > 0; i--) {
    const j = crypto.randomInt(0, i + 1);
    [copy[i], copy[j]] = [copy[j], copy[i]];
  }
  return copy;
}

function svgDataUri(svg: string): string {
  return `data:image/svg+xml;base64,${Buffer.from(svg, 'utf-8').toString('base64')}`;
}

export type ResolvedType = 'click-text' | 'click-shape' | 'slide-text' | 'slide-region' | 'rotate';

export interface GeneratedChallenge {
  type: ResolvedType;
  image: string;
  thumb?: string;
  thumbWidth?: number;
  thumbHeight?: number;
  thumbX?: number;
  thumbY?: number;
  imageWidth: number;
  imageHeight: number;
  scale: number;
  difficulty: number;
  answerData: unknown;
}

const CLICK_CHARS = Array.from(
  '天地玄黄宇宙洪荒日月盈昃辰宿列张寒来暑往秋收冬藏云智光慧信安和明远清风山川河海星辰松柏兰竹梅菊春华秋实金木水火土仁义礼智信',
);

const DARK_TEXT_COLORS = ['#fde98e', '#60c1ff', '#fcb08e', '#fb88ff', '#b4fed4', '#cbfaa9', '#78d6f8'];
const LIGHT_TEXT_COLORS = ['#1d4ed8', '#9333ea', '#047857', '#b91c1c', '#b45309', '#0f766e'];
const DARK_THUMB_COLORS = ['#60a5fa', '#c084fc', '#4ade80', '#f87171', '#fbbf24', '#2dd4bf'];
const LIGHT_THUMB_COLORS = ['#2563eb', '#9333ea', '#16a34a', '#dc2626', '#d97706', '#0d9488'];

type ShapeKind = 'triangle' | 'circle' | 'square' | 'diamond' | 'star' | 'hexagon';
const SHAPE_KINDS: readonly ShapeKind[] = ['triangle', 'circle', 'square', 'diamond', 'star', 'hexagon'];

function renderShapeSvg(
  shape: ShapeKind,
  cx: number,
  cy: number,
  size: number,
  color: string,
  angle: number,
  stroke: string,
  strokeWidth: number,
): string {
  const r = size * 0.44;
  let inner = '';
  switch (shape) {
    case 'circle':
      inner = `<circle cx="0" cy="0" r="${r.toFixed(1)}" fill="${color}" stroke="${stroke}" stroke-width="${strokeWidth}" />`;
      break;
    case 'square':
      inner = `<rect x="${(-r * 0.85).toFixed(1)}" y="${(-r * 0.85).toFixed(1)}" width="${(r * 1.7).toFixed(1)}" height="${(r * 1.7).toFixed(1)}" rx="${(r * 0.2).toFixed(1)}" fill="${color}" stroke="${stroke}" stroke-width="${strokeWidth}" />`;
      break;
    case 'diamond':
      inner = `<polygon points="0,${-r} ${r},0 0,${r} ${-r},0" fill="${color}" stroke="${stroke}" stroke-width="${strokeWidth}" stroke-linejoin="round" />`;
      break;
    case 'triangle': {
      const h = r * 0.95;
      const w = r * 0.95;
      inner = `<polygon points="0,${-h} ${w},${h * 0.85} ${-w},${h * 0.85}" fill="${color}" stroke="${stroke}" stroke-width="${strokeWidth}" stroke-linejoin="round" />`;
      break;
    }
    case 'hexagon': {
      const pts: string[] = [];
      for (let i = 0; i < 6; i++) {
        const a = (Math.PI / 3) * i - Math.PI / 6;
        pts.push(`${(Math.cos(a) * r).toFixed(1)},${(Math.sin(a) * r).toFixed(1)}`);
      }
      inner = `<polygon points="${pts.join(' ')}" fill="${color}" stroke="${stroke}" stroke-width="${strokeWidth}" stroke-linejoin="round" />`;
      break;
    }
    case 'star': {
      const pts: string[] = [];
      for (let i = 0; i < 10; i++) {
        const rad = i % 2 === 0 ? r : r * 0.45;
        const a = (Math.PI / 5) * i - Math.PI / 2;
        pts.push(`${(Math.cos(a) * rad).toFixed(1)},${(Math.sin(a) * rad).toFixed(1)}`);
      }
      inner = `<polygon points="${pts.join(' ')}" fill="${color}" stroke="${stroke}" stroke-width="${strokeWidth}" stroke-linejoin="round" />`;
      break;
    }
  }
  return `<g transform="translate(${cx.toFixed(1)},${cy.toFixed(1)}) rotate(${angle})">${inner}</g>`;
}

interface PlacedItem {
  cx: number;
  cy: number;
  boxSize: number;
  dx: number;
  dy: number;
  width: number;
  height: number;
  angle: number;
}

function placeNonOverlapping(count: number, W: number, H: number, minSize: number, maxSize: number, scale: number): PlacedItem[] {
  const placed: PlacedItem[] = [];
  const marginX = 28 * scale;
  const marginY = 28 * scale;

  for (let i = 0; i < count; i++) {
    const boxSize = randInt(minSize, maxSize);
    let chosen: PlacedItem | null = null;
    for (let attempt = 0; attempt < 80; attempt++) {
      const cx = randInt(marginX + boxSize / 2, W - marginX - boxSize / 2);
      const cy = randInt(marginY + boxSize / 2, H - marginY - boxSize / 2);
      const minDist = boxSize * 1.15;
      const overlaps = placed.some((p) => Math.hypot(p.cx - cx, p.cy - cy) < Math.max(minDist, p.boxSize * 1.15));
      if (!overlaps) {
        chosen = {
          cx,
          cy,
          boxSize,
          dx: Math.round(cx - boxSize / 2),
          dy: Math.round(cy - boxSize / 2),
          width: boxSize,
          height: boxSize,
          angle: randInt(-32, 32),
        };
        break;
      }
    }
    if (!chosen) {
      // Fallback grid slot if random packing is tight
      const cols = Math.max(3, Math.ceil(Math.sqrt(count)));
      const col = i % cols;
      const row = Math.floor(i / cols);
      const cellW = (W - marginX * 2) / cols;
      const cellH = (H - marginY * 2) / Math.ceil(count / cols);
      const cx = Math.round(marginX + cellW * (col + 0.5));
      const cy = Math.round(marginY + cellH * (row + 0.5));
      chosen = {
        cx,
        cy,
        boxSize,
        dx: Math.round(cx - boxSize / 2),
        dy: Math.round(cy - boxSize / 2),
        width: boxSize,
        height: boxSize,
        angle: randInt(-25, 25),
      };
    }
    placed.push(chosen);
  }
  return placed;
}

function buildClickThumbNoise(thumbW: number, thumbH: number, scale: number, isLight: boolean): string {
  const elements: string[] = [];
  const circleCount = 12;
  for (let i = 0; i < circleCount; i++) {
    const cx = randInt(4, thumbW - 4);
    const cy = randInt(4, thumbH - 4);
    const r = randInt(2 * scale, 5 * scale);
    const opacity = (0.12 + Math.random() * 0.16).toFixed(2);
    const fill = isLight ? '#64748b' : '#94a3b8';
    elements.push(`<circle cx="${cx}" cy="${cy}" r="${r}" fill="${fill}" opacity="${opacity}" />`);
  }
  for (let i = 0; i < 3; i++) {
    const x1 = randInt(0, thumbW / 3);
    const y1 = randInt(2, thumbH - 2);
    const x2 = randInt((thumbW * 2) / 3, thumbW);
    const y2 = randInt(2, thumbH - 2);
    const stroke = isLight ? 'rgba(15,23,42,0.16)' : 'rgba(255,255,255,0.16)';
    elements.push(`<line x1="${x1}" y1="${y1}" x2="${x2}" y2="${y2}" stroke="${stroke}" stroke-width="${scale}" />`);
  }
  return elements.join('');
}

function jigsawPath(w: number, h: number): string {
  // Normalized jigsaw puzzle piece fitting cleanly inside [0, w] x [0, h]
  const pad = Math.round(w * 0.14);
  const x0 = pad;
  const y0 = pad;
  const x1 = w - pad;
  const y1 = h - pad;
  const midX = (x0 + x1) / 2;
  const midY = (y0 + y1) / 2;
  const r = pad * 0.88;

  return [
    `M ${x0} ${y0}`,
    `L ${(midX - r).toFixed(1)} ${y0}`,
    `C ${(midX - r).toFixed(1)} ${(y0 - r * 1.15).toFixed(1)}, ${(midX + r).toFixed(1)} ${(y0 - r * 1.15).toFixed(1)}, ${(midX + r).toFixed(1)} ${y0}`,
    `L ${x1} ${y0}`,
    `L ${x1} ${(midY - r).toFixed(1)}`,
    `C ${(x1 + r * 1.15).toFixed(1)} ${(midY - r).toFixed(1)}, ${(x1 + r * 1.15).toFixed(1)} ${(midY + r).toFixed(1)}, ${x1} ${(midY + r).toFixed(1)}`,
    `L ${x1} ${y1}`,
    `L ${(midX + r).toFixed(1)} ${y1}`,
    `C ${(midX + r).toFixed(1)} ${(y1 - r * 0.95).toFixed(1)}, ${(midX - r).toFixed(1)} ${(y1 - r * 0.95).toFixed(1)}, ${(midX - r).toFixed(1)} ${y1}`,
    `L ${x0} ${y1}`,
    `L ${x0} ${(midY + r).toFixed(1)}`,
    `C ${(x0 + r * 0.95).toFixed(1)} ${(midY + r).toFixed(1)}, ${(x0 + r * 0.95).toFixed(1)} ${(midY - r).toFixed(1)}, ${x0} ${(midY - r).toFixed(1)}`,
    'Z',
  ].join(' ');
}

export function generateChallenge(
  type: ResolvedType,
  backgrounds: Map<string, BackgroundEntry>,
  difficulty: number,
  scale: number,
  mode: 'light' | 'dark',
): GeneratedChallenge {
  const list = Array.from(backgrounds.values());
  if (list.length === 0) {
    throw new Error('没有可用的背景图素材');
  }
  const bg = pickRandom(list);
  const profile = getProfile(difficulty);
  const isLight = mode === 'light';

  if (type === 'click-text') {
    const W = 300 * scale;
    const H = 200 * scale;
    const thumbW = 150 * scale;
    const thumbH = 40 * scale;

    const totalLen = randInt(profile.clickLen[0], profile.clickLen[1]);
    const verifyLen = Math.min(totalLen, randInt(profile.clickVerifyLen[0], profile.clickVerifyLen[1]));
    const chars = shuffle([...CLICK_CHARS]).slice(0, totalLen);
    const items = placeNonOverlapping(
      totalLen,
      W,
      H,
      profile.clickFontSize[0] * scale,
      profile.clickFontSize[1] * scale,
      scale,
    );

    const colors = isLight ? LIGHT_TEXT_COLORS : DARK_TEXT_COLORS;
    const thumbColors = isLight ? LIGHT_THUMB_COLORS : DARK_THUMB_COLORS;

    const textNodes = items
      .map((item, idx) => {
        const ch = chars[idx];
        const color = colors[idx % colors.length];
        const stroke = isLight ? 'rgba(255,255,255,0.88)' : 'rgba(9,13,22,0.85)';
        return `<text x="${item.cx}" y="${item.cy}" text-anchor="middle" dominant-baseline="central" font-size="${item.boxSize}" font-weight="700" font-family="PingFang SC, Microsoft YaHei, Noto Sans SC, sans-serif" fill="${color}" stroke="${stroke}" stroke-width="${Math.max(2, scale * 1.8)}" paint-order="stroke" transform="rotate(${item.angle} ${item.cx} ${item.cy})">${ch}</text>`;
      })
      .join('');

    const masterSvg = `<svg xmlns="http://www.w3.org/2000/svg" width="${W}" height="${H}" viewBox="0 0 ${W} ${H}">
      <image href="${bg.dataUri}" x="0" y="0" width="${W}" height="${H}" preserveAspectRatio="xMidYMid slice" />
      <rect x="0" y="0" width="${W}" height="${H}" fill="${isLight ? 'rgba(255,255,255,0.08)' : 'rgba(9,13,22,0.22)'}" />
      ${textNodes}
    </svg>`;

    const stepX = thumbW / (verifyLen + 1);
    const thumbFontSize = Math.round(22 * scale);
    const thumbTextNodes = items
      .slice(0, verifyLen)
      .map((_, idx) => {
        const ch = chars[idx];
        const tx = Math.round(stepX * (idx + 1));
        const ty = Math.round(thumbH / 2);
        const angle = randInt(-14, 14);
        const color = thumbColors[idx % thumbColors.length];
        return `<text x="${tx}" y="${ty}" text-anchor="middle" dominant-baseline="central" font-size="${thumbFontSize}" font-weight="700" font-family="PingFang SC, Microsoft YaHei, Noto Sans SC, sans-serif" fill="${color}" transform="rotate(${angle} ${tx} ${ty})">${ch}</text>`;
      })
      .join('');

    const thumbBg = isLight ? '#f8fafc' : '#111827';
    const thumbSvg = `<svg xmlns="http://www.w3.org/2000/svg" width="${thumbW}" height="${thumbH}" viewBox="0 0 ${thumbW} ${thumbH}">
      <rect x="0" y="0" width="${thumbW}" height="${thumbH}" fill="${thumbBg}" />
      ${buildClickThumbNoise(thumbW, thumbH, scale, isLight)}
      ${thumbTextNodes}
    </svg>`;

    const answerData: Record<string, { x: number; y: number; width: number; height: number }> = {};
    for (let i = 0; i < verifyLen; i++) {
      answerData[String(i)] = {
        x: items[i].dx,
        y: items[i].dy,
        width: items[i].width,
        height: items[i].height,
      };
    }

    return {
      type,
      image: svgDataUri(masterSvg),
      thumb: svgDataUri(thumbSvg),
      thumbWidth: thumbW,
      thumbHeight: thumbH,
      imageWidth: W,
      imageHeight: H,
      scale,
      difficulty,
      answerData,
    };
  }

  if (type === 'click-shape') {
    const W = 300 * scale;
    const H = 200 * scale;
    const thumbW = 150 * scale;
    const thumbH = 40 * scale;

    const totalLen = Math.min(SHAPE_KINDS.length, randInt(profile.clickLen[0], profile.clickLen[1]));
    const verifyLen = Math.min(totalLen, randInt(profile.clickVerifyLen[0], profile.clickVerifyLen[1]));
    const shapes = shuffle([...SHAPE_KINDS]).slice(0, totalLen);
    const items = placeNonOverlapping(
      totalLen,
      W,
      H,
      profile.clickFontSize[0] * scale,
      profile.clickFontSize[1] * scale,
      scale,
    );

    const colors = DARK_TEXT_COLORS;
    const thumbColors = isLight ? LIGHT_THUMB_COLORS : DARK_THUMB_COLORS;

    const shapeNodes = items
      .map((item, idx) =>
        renderShapeSvg(
          shapes[idx],
          item.cx,
          item.cy,
          item.boxSize,
          colors[idx % colors.length],
          item.angle,
          'rgba(10,14,24,0.85)',
          Math.max(1.5, scale * 1.4),
        ),
      )
      .join('');

    const masterSvg = `<svg xmlns="http://www.w3.org/2000/svg" width="${W}" height="${H}" viewBox="0 0 ${W} ${H}">
      <image href="${bg.dataUri}" x="0" y="0" width="${W}" height="${H}" preserveAspectRatio="xMidYMid slice" />
      <rect x="0" y="0" width="${W}" height="${H}" fill="rgba(9,13,22,0.18)" />
      ${shapeNodes}
    </svg>`;

    const stepX = thumbW / (verifyLen + 1);
    const thumbShapeSize = Math.round(22 * scale);
    const thumbShapeNodes = items
      .slice(0, verifyLen)
      .map((_, idx) =>
        renderShapeSvg(
          shapes[idx],
          Math.round(stepX * (idx + 1)),
          Math.round(thumbH / 2),
          thumbShapeSize,
          thumbColors[idx % thumbColors.length],
          0,
          isLight ? 'rgba(15,23,42,0.35)' : 'rgba(255,255,255,0.45)',
          scale,
        ),
      )
      .join('');

    const thumbBg = isLight ? '#f8fafc' : '#111827';
    const thumbSvg = `<svg xmlns="http://www.w3.org/2000/svg" width="${thumbW}" height="${thumbH}" viewBox="0 0 ${thumbW} ${thumbH}">
      <rect x="0" y="0" width="${thumbW}" height="${thumbH}" fill="${thumbBg}" />
      ${buildClickThumbNoise(thumbW, thumbH, scale, isLight)}
      ${thumbShapeNodes}
    </svg>`;

    const answerData: Record<string, { x: number; y: number; width: number; height: number }> = {};
    for (let i = 0; i < verifyLen; i++) {
      answerData[String(i)] = {
        x: items[i].dx,
        y: items[i].dy,
        width: items[i].width,
        height: items[i].height,
      };
    }

    return {
      type,
      image: svgDataUri(masterSvg),
      thumb: svgDataUri(thumbSvg),
      thumbWidth: thumbW,
      thumbHeight: thumbH,
      imageWidth: W,
      imageHeight: H,
      scale,
      difficulty,
      answerData,
    };
  }

  if (type === 'slide-text' || type === 'slide-region') {
    const W = 300 * scale;
    const H = 200 * scale;
    const tileSize = randInt(profile.slideGraphSize[0], profile.slideGraphSize[1]) * scale;
    const dx = randInt(Math.round(W * 0.44), W - tileSize - 18 * scale);
    const dy = randInt(16 * scale, H - tileSize - 16 * scale);

    const startX = randInt(10 * scale, 26 * scale);
    const startY =
      type === 'slide-text'
        ? dy
        : dy > H / 2
          ? randInt(14 * scale, Math.max(18 * scale, Math.round(H * 0.35) - tileSize))
          : randInt(Math.round(H * 0.6), H - tileSize - 14 * scale);

    const pathD = jigsawPath(tileSize, tileSize);

    let decoySlot = '';
    if (type === 'slide-region') {
      const decoyX = randInt(Math.round(W * 0.28), W - tileSize - 18 * scale);
      const decoyY = dy > H / 2 ? randInt(16 * scale, Math.round(H * 0.35)) : randInt(Math.round(H * 0.55), H - tileSize - 16 * scale);
      if (Math.hypot(decoyX - dx, decoyY - dy) > tileSize * 1.1) {
        decoySlot = `<g transform="translate(${decoyX}, ${decoyY}) rotate(90 ${tileSize / 2} ${tileSize / 2})">
          <path d="${pathD}" fill="rgba(10,14,24,0.45)" stroke="rgba(255,255,255,0.4)" stroke-width="${Math.max(1.5, scale * 1.1)}" />
        </g>`;
      }
    }

    const masterSvg = `<svg xmlns="http://www.w3.org/2000/svg" width="${W}" height="${H}" viewBox="0 0 ${W} ${H}">
      <image href="${bg.dataUri}" x="0" y="0" width="${W}" height="${H}" preserveAspectRatio="xMidYMid slice" />
      ${decoySlot}
      <g transform="translate(${dx}, ${dy})">
        <path d="${pathD}" fill="rgba(9,13,22,0.68)" stroke="rgba(255,255,255,0.72)" stroke-width="${Math.max(1.5, scale * 1.25)}" />
      </g>
    </svg>`;

    const clipId = `tile-${crypto.randomBytes(4).toString('hex')}`;
    const thumbSvg = `<svg xmlns="http://www.w3.org/2000/svg" width="${tileSize}" height="${tileSize}" viewBox="0 0 ${tileSize} ${tileSize}">
      <defs>
        <clipPath id="${clipId}">
          <path d="${pathD}" />
        </clipPath>
      </defs>
      <g clip-path="url(#${clipId})">
        <image href="${bg.dataUri}" x="${-dx}" y="${-dy}" width="${W}" height="${H}" preserveAspectRatio="xMidYMid slice" />
      </g>
      <path d="${pathD}" fill="none" stroke="rgba(255,255,255,0.92)" stroke-width="${Math.max(2, scale * 1.5)}" />
    </svg>`;

    return {
      type,
      image: svgDataUri(masterSvg),
      thumb: svgDataUri(thumbSvg),
      thumbWidth: tileSize,
      thumbHeight: tileSize,
      thumbX: startX,
      thumbY: startY,
      imageWidth: W,
      imageHeight: H,
      scale,
      difficulty,
      answerData: { x: dx, y: dy },
    };
  }

  // rotate
  const square = 128 * scale;
  const thumbSize = pickRandom([84, 88, 92, 96]) * scale;
  const dAngle = randInt(40, 320);
  const delta = (square - thumbSize) / 2;
  const ringMaskId = `ring-${crypto.randomBytes(4).toString('hex')}`;
  const innerClipId = `inner-${crypto.randomBytes(4).toString('hex')}`;

  const masterSvg = `<svg xmlns="http://www.w3.org/2000/svg" width="${square}" height="${square}" viewBox="0 0 ${square} ${square}">
    <defs>
      <mask id="${ringMaskId}">
        <rect x="0" y="0" width="${square}" height="${square}" fill="black" />
        <circle cx="${square / 2}" cy="${square / 2}" r="${square / 2}" fill="white" />
        <circle cx="${square / 2}" cy="${square / 2}" r="${thumbSize / 2}" fill="black" />
      </mask>
    </defs>
    <g mask="url(#${ringMaskId})">
      <image href="${bg.dataUri}" x="0" y="0" width="${square}" height="${square}" preserveAspectRatio="xMidYMid slice" />
    </g>
    <circle cx="${square / 2}" cy="${square / 2}" r="${thumbSize / 2}" fill="none" stroke="rgba(255,255,255,0.45)" stroke-width="${scale}" />
  </svg>`;

  const thumbSvg = `<svg xmlns="http://www.w3.org/2000/svg" width="${thumbSize}" height="${thumbSize}" viewBox="0 0 ${thumbSize} ${thumbSize}">
    <defs>
      <clipPath id="${innerClipId}">
        <circle cx="${thumbSize / 2}" cy="${thumbSize / 2}" r="${thumbSize / 2}" />
      </clipPath>
    </defs>
    <g clip-path="url(#${innerClipId})">
      <g transform="rotate(${dAngle} ${thumbSize / 2} ${thumbSize / 2})">
        <image href="${bg.dataUri}" x="${-delta}" y="${-delta}" width="${square}" height="${square}" preserveAspectRatio="xMidYMid slice" />
      </g>
    </g>
    <circle cx="${thumbSize / 2}" cy="${thumbSize / 2}" r="${thumbSize / 2 - scale * 0.5}" fill="none" stroke="rgba(255,255,255,0.65)" stroke-width="${scale}" />
  </svg>`;

  return {
    type: 'rotate',
    image: svgDataUri(masterSvg),
    thumb: svgDataUri(thumbSvg),
    thumbWidth: thumbSize,
    thumbHeight: thumbSize,
    imageWidth: square,
    imageHeight: square,
    scale,
    difficulty,
    answerData: { angle: dAngle },
  };
}

export function verifyPow(nonce: string, key: string, solution: string, difficulty: number): boolean {
  if (difficulty <= 0) return true;
  if (!solution || solution.length > 32) return false;
  const digest = crypto.createHash('sha256').update(`${nonce}:${key}:${solution}`).digest();
  let leadingZeros = 0;
  for (const byte of digest) {
    if (byte === 0) {
      leadingZeros += 8;
      continue;
    }
    leadingZeros += Math.clz32(byte) - 24;
    break;
  }
  return leadingZeros >= difficulty;
}
