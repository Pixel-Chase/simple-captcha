import { useMemo } from 'react';
import { useCaptchaConfig } from './CaptchaProvider';

const DEFAULT_PRIMARY = '#4f46e5';
const FONT_STACK =
  "'Plus Jakarta Sans', 'Noto Sans SC', -apple-system, BlinkMacSystemFont, 'Segoe UI', 'PingFang SC', 'Microsoft YaHei', sans-serif";
const MONO_STACK = 'ui-monospace, SFMono-Regular, Menlo, Consolas, monospace';

export const CaptchaStyle = () => {
  const { theme } = useCaptchaConfig();

  const css = useMemo(() => {
    const primary = theme.primaryColor || DEFAULT_PRIMARY;

    return `
    .gc-wrapper {
      --gc-ink: ${theme.textColor || '#0f172a'};
      --gc-sub: #64748b;
      --gc-mute: #94a3b8;
      --gc-line: ${theme.borderColor || '#e2e8f0'};
      --gc-soft: #f1f5f9;
      --gc-soft-2: #f8fafc;
      --gc-bg: ${theme.backgroundColor || '#fff'};
      --gc-primary: ${primary};
      --gc-primary-hover: color-mix(in srgb, var(--gc-primary) 85%, #000);
      --gc-primary-light: color-mix(in srgb, var(--gc-primary) 82%, #fff);
      --gc-primary-lighter: color-mix(in srgb, var(--gc-primary) 62%, #fff);
      --gc-tint-50: color-mix(in srgb, var(--gc-primary) 7%, #fff);
      --gc-tint-100: color-mix(in srgb, var(--gc-primary) 15%, #fff);
      --gc-tint-200: color-mix(in srgb, var(--gc-primary) 28%, #fff);
      --gc-radius: ${theme.borderRadius ?? 16}px;
      --gc-success: #059669;
      --gc-fail: #dc2626;

      --gc-handle: 36px;
      --gc-hgap: 5px;
      --gc-track-h: 44px;

      display: flex;
      flex-direction: column;
      gap: 16px;
      padding: var(--gc-py, 20px) var(--gc-px, 20px);
      color: var(--gc-ink);
      font-family: ${FONT_STACK};
      font-weight: 400;
      line-height: 1.5;
      -webkit-font-smoothing: antialiased;
      -webkit-touch-callout: none;
      user-select: none;
      box-sizing: border-box;
      -webkit-tap-highlight-color: transparent;
    }
    .gc-wrapper *, .gc-wrapper *::before, .gc-wrapper *::after { box-sizing: border-box; }
    .gc-wrapper button { font-family: inherit; }
    .gc-wrapper h3, .gc-wrapper p { margin: 0; }

    .gc-wrapper-theme {
      background-color: var(--gc-bg);
      border-radius: var(--gc-radius);
      box-shadow: 0 20px 48px -12px rgba(15, 23, 42, 0.18), 0 0 0 1px rgba(226, 232, 240, 0.9);
      animation: gc-card-in .65s cubic-bezier(.16, 1, .3, 1) both;
      transition: box-shadow .3s cubic-bezier(.16, 1, .3, 1);
    }
    .gc-wrapper-theme:hover {
      box-shadow:
        0 26px 60px -12px color-mix(in srgb, var(--gc-primary) 22%, transparent),
        0 0 0 1px color-mix(in srgb, var(--gc-primary-light) 40%, transparent),
        0 0 24px -2px color-mix(in srgb, var(--gc-primary-light) 15%, transparent);
    }
    .gc-wrapper-theme.gc-status-success { box-shadow: 0 20px 48px -12px rgba(15, 23, 42, 0.18), 0 0 0 1px #a7f3d0; }
    .gc-wrapper-theme.gc-status-fail { box-shadow: 0 20px 48px -12px rgba(15, 23, 42, 0.18), 0 0 0 1px #fecaca; }

    .gc-hide { visibility: hidden; }
    .gc-disabled { pointer-events: none; opacity: .5; }

    /* 结果态下题目区域不再响应操作；头部的关闭按钮仍可用 */
    .gc-status-success .gc-body, .gc-status-fail .gc-body,
    .gc-status-success .gc-track, .gc-status-fail .gc-track,
    .gc-status-success .gc-confirm, .gc-status-fail .gc-confirm,
    .gc-status-success .gc-clear, .gc-status-fail .gc-clear { pointer-events: none; }

    /* ---------- 头部 ---------- */
    .gc-header {
      display: flex;
      align-items: center;
      justify-content: space-between;
      gap: 8px;
      padding-bottom: 4px;
      user-select: none;
    }
    .gc-title { flex: 1; min-width: 0; display: flex; align-items: center; gap: 8px; }
    .gc-title-tile {
      flex: none;
      display: flex;
      align-items: center;
      justify-content: center;
      width: 28px;
      height: 28px;
      border-radius: 8px;
      color: var(--gc-primary);
      background: var(--gc-tint-50);
      border: 1px solid var(--gc-tint-100);
      box-shadow: 0 1px 2px rgba(15, 23, 42, 0.05);
    }
    .gc-title-tile svg { width: 14px; height: 14px; }
    .gc-title-text { min-width: 0; }
    .gc-title-main {
      font-size: 14px;
      font-weight: 700;
      line-height: 1.25;
      color: var(--gc-ink);
      white-space: nowrap;
    }
    .gc-subtitle {
      margin-top: 2px;
      font-size: 11px;
      font-weight: 500;
      line-height: 1;
      color: var(--gc-mute);
      white-space: nowrap;
      overflow: hidden;
      text-overflow: ellipsis;
    }
    .gc-header-fail .gc-subtitle { color: var(--gc-fail); }

    .gc-icon-block { flex: none; display: flex; align-items: center; gap: 4px; color: var(--gc-mute); }
    .gc-icon-block button {
      display: flex;
      align-items: center;
      justify-content: center;
      width: 28px;
      height: 28px;
      padding: 0;
      border: 0;
      border-radius: 8px;
      background: transparent;
      color: inherit;
      cursor: pointer;
    }
    .gc-icon-block button svg { width: 14px; height: 14px; }
    .gc-icon-block button:hover { background: var(--gc-soft); color: #334155; }
    .gc-icon-block button:focus-visible { outline: 2px solid var(--gc-primary-light); outline-offset: 1px; }
    .gc-btn-refresh { transition: transform .4s cubic-bezier(.34, 1.56, .64, 1), background-color .2s, color .2s; }
    .gc-btn-refresh:hover { transform: rotate(180deg); }
    .gc-btn-close { transition: transform .25s cubic-bezier(.34, 1.56, .64, 1), background-color .2s, color .2s; }
    .gc-btn-close:hover { transform: scale(1.15); }

    /* ---------- 图片区域 ---------- */
    .gc-body {
      position: relative;
      max-width: 100%;
      display: flex;
      background: var(--gc-soft);
      border-radius: 12px;
      overflow: hidden;
    }
    .gc-body::after {
      content: '';
      position: absolute;
      inset: 0;
      z-index: 4;
      pointer-events: none;
      border-radius: inherit;
      box-shadow: inset 0 0 0 1px var(--gc-line);
      transition: box-shadow .2s;
    }
    .gc-status-fail .gc-body::after { box-shadow: inset 0 0 0 1px rgba(220, 38, 38, 0.5); }
    .gc-picture { position: relative; z-index: 1; width: 100%; }
    .gc-body-click .gc-picture { filter: brightness(.88) contrast(1.05); }

    .gc-loading {
      position: absolute;
      z-index: 0;
      top: 50%;
      left: 50%;
      width: 68px;
      height: 68px;
      margin-left: -34px;
      margin-top: -34px;
      line-height: 68px;
      text-align: center;
      display: flex;
      align-content: center;
      justify-content: center;
    }
    .gc-loading svg, .gc-loading circle { color: var(--gc-primary); fill: var(--gc-primary); }

    /* ---------- 点选题 ---------- */
    .gc-hint-bar {
      align-items: center;
      justify-content: space-between;
      gap: 8px;
      padding: 8px 12px;
      background: var(--gc-tint-50);
      border: 1px solid var(--gc-tint-100);
      border-radius: 12px;
      font-size: 12px;
      font-weight: 500;
      color: #1e1b4b;
    }
    .gc-hint-bar img { display: block; max-width: 100%; border-radius: 6px; }

    .gc-dots { position: absolute; inset: 0; }
    .gc-dot {
      position: absolute;
      z-index: 3;
      color: #fff;
      font-size: 12px;
      font-weight: 700;
      background: var(--gc-primary);
      border: 2px solid #fff;
      display: flex;
      align-items: center;
      justify-content: center;
      border-radius: 50%;
      cursor: default;
      animation: gc-dot-in .25s ease-out, gc-radar 2.2s ease-out infinite;
    }
    .gc-dot:nth-child(even) { animation-delay: 0s, .8s; }

    .gc-status-row {
      display: flex;
      align-items: center;
      justify-content: space-between;
      padding: 0 2px;
      font-size: 12px;
      color: var(--gc-sub);
      user-select: none;
    }
    .gc-status-count { display: flex; align-items: center; gap: 6px; }
    .gc-status-count svg { color: var(--gc-primary-light); }
    .gc-status-count strong { margin: 0 3px; font-weight: 700; color: var(--gc-primary); }
    .gc-clear {
      padding: 0;
      border: 0;
      background: transparent;
      font-size: 11px;
      color: var(--gc-mute);
      cursor: pointer;
      transition: color .2s;
    }
    .gc-clear:hover { color: var(--gc-primary); }

    .gc-confirm {
      display: flex;
      align-items: center;
      justify-content: center;
      gap: 8px;
      width: 100%;
      height: 40px;
      padding: 0 16px;
      border: 0;
      border-radius: 12px;
      color: #fff;
      font-size: 12px;
      font-weight: 600;
      letter-spacing: .025em;
      line-height: 1;
      white-space: nowrap;
      cursor: pointer;
      background-size: 200% auto;
      background-image: linear-gradient(105deg, var(--gc-primary) 0%, var(--gc-primary-light) 35%, var(--gc-primary-lighter) 50%, var(--gc-primary-light) 65%, var(--gc-primary) 100%);
      animation: gc-shimmer 4s linear infinite;
      transition: transform .15s, box-shadow .2s;
      -webkit-appearance: none;
      outline: none;
    }
    .gc-confirm:hover { box-shadow: 0 6px 26px 4px color-mix(in srgb, var(--gc-primary-light) 55%, transparent); }
    .gc-confirm:active { transform: scale(.99); }
    .gc-confirm:focus-visible { outline: 2px solid var(--gc-primary-light); outline-offset: 2px; }
    .gc-status-success .gc-confirm { background-image: none; background-color: var(--gc-success); animation: none; }
    .gc-status-fail .gc-confirm { opacity: .5; animation: none; }

    /* ---------- 拼图块 ---------- */
    .gc-tile {
      position: absolute;
      z-index: 3;
      cursor: grab;
      filter: drop-shadow(0 0 2px color-mix(in srgb, var(--gc-primary-light) 90%, transparent)) drop-shadow(0 10px 14px rgba(30, 27, 75, 0.55));
    }
    .gc-tile:active { cursor: grabbing; }
    .gc-tile img { display: block; cursor: inherit; width: 100%; height: 100%; }
    .gc-tile-region { user-select: none; touch-action: none; animation: gc-float 3s ease-in-out infinite; }
    .gc-tile-region:active { animation: none; }

    /* 滑动区域：图片底部的操作提示 */
    .gc-region-tip {
      position: absolute;
      left: 8px;
      right: 8px;
      bottom: 8px;
      z-index: 4;
      display: flex;
      align-items: center;
      justify-content: space-between;
      padding: 6px 12px;
      border-radius: 8px;
      border: 1px solid rgba(255, 255, 255, 0.6);
      background: rgba(255, 255, 255, 0.92);
      backdrop-filter: blur(8px);
      box-shadow: 0 1px 2px rgba(15, 23, 42, 0.06);
      font-size: 11px;
      color: #334155;
      pointer-events: none;
    }
    .gc-region-tip span { display: flex; align-items: center; gap: 6px; font-weight: 500; }
    .gc-region-tip svg { color: var(--gc-primary); }
    .gc-region-tip em {
      padding: 2px 6px;
      border-radius: 4px;
      background: #ecfdf5;
      color: #059669;
      font-size: 10px;
      font-weight: 600;
      font-style: normal;
      animation: gc-pill 2.4s ease-in-out infinite;
    }
    .gc-secure-note {
      display: flex;
      align-items: center;
      justify-content: center;
      gap: 8px;
      padding-top: 4px;
      font-size: 11px;
      color: var(--gc-mute);
      user-select: none;
    }
    .gc-secure-icon { color: var(--gc-primary-lighter); animation: gc-shield 3.6s ease-in-out infinite; }

    /* ---------- 滑动轨道 ---------- */
    .gc-track { display: flex; flex-direction: column; gap: 6px; }
    .gc-footer { width: 100%; display: flex; align-items: center; user-select: none; }
    .gc-drag-slide-bar { width: 100%; height: var(--gc-track-h); position: relative; touch-action: none; }
    .gc-drag-line {
      position: absolute;
      inset: 0;
      background: var(--gc-soft);
      border: 1px solid rgba(226, 232, 240, 0.9);
      border-radius: 12px;
      overflow: hidden;
      transition: background-color .2s, border-color .2s;
    }
    .gc-drag-fill {
      position: absolute;
      left: 0; top: 0; bottom: 0;
      box-sizing: content-box;
      padding-right: calc(var(--gc-handle) + var(--gc-hgap) * 2);
      background: var(--gc-tint-50);
      border-right: 1px solid var(--gc-tint-200);
    }
    .gc-drag-hint {
      position: absolute;
      inset: 0;
      display: flex;
      align-items: center;
      justify-content: center;
      padding-left: 32px;
      font-size: 12px;
      font-weight: 500;
      color: var(--gc-mute);
      white-space: nowrap;
      pointer-events: none;
      transition: opacity .2s, color .2s;
    }
    .gc-drag-slide-bar:hover .gc-drag-hint { color: var(--gc-sub); }
    .gc-drag-hint-hidden { opacity: 0; }
    .gc-drag-block {
      position: absolute;
      left: 0;
      top: 0;
      width: calc(var(--gc-handle) + var(--gc-hgap) * 2);
      height: var(--gc-track-h);
      z-index: 2;
      display: flex;
      align-items: center;
      justify-content: center;
      cursor: grab;
      user-select: none;
    }
    .gc-drag-block:active { cursor: grabbing; }
    .gc-drag-block.gc-disabled { opacity: .6; }
    .gc-hide-block { display: none; }
    .gc-drag-block-inline {
      width: var(--gc-handle);
      height: var(--gc-handle);
      display: flex;
      align-items: center;
      justify-content: center;
      border-radius: 8px;
      background: var(--gc-primary);
      color: #fff;
      box-shadow: 0 4px 6px -1px color-mix(in srgb, var(--gc-primary) 30%, transparent), 0 2px 4px -2px color-mix(in srgb, var(--gc-primary) 30%, transparent);
      transition: background-color .15s;
    }
    .gc-drag-block:hover .gc-drag-block-inline { background: var(--gc-primary-hover); }
    .gc-drag-idle { animation: gc-nudge 3.2s ease-in-out infinite; }

    .gc-track-meta {
      display: flex;
      align-items: center;
      justify-content: space-between;
      padding: 0 4px;
      font-size: 11px;
      color: var(--gc-mute);
    }
    .gc-track-meta-value { font-family: ${MONO_STACK}; font-weight: 500; color: var(--gc-primary); }

    /* 轨道内的结果态 */
    .gc-drag-result {
      position: absolute;
      inset: 0;
      display: flex;
      align-items: center;
      justify-content: space-between;
      padding: 0 4px 0 14px;
      font-size: 12px;
      font-weight: 600;
    }
    .gc-drag-result-icon {
      display: flex;
      align-items: center;
      justify-content: center;
      width: calc(var(--gc-track-h) - 10px);
      height: calc(var(--gc-track-h) - 10px);
      border-radius: 8px;
      color: #fff;
    }
    .gc-status-success .gc-drag-line { background: #ecfdf5; border-color: #a7f3d0; }
    .gc-status-success .gc-drag-result { color: #047857; }
    .gc-status-success .gc-drag-result-icon { background: var(--gc-success); box-shadow: 0 2px 6px rgba(5, 150, 105, 0.35); }
    .gc-status-fail .gc-drag-line { background: #fef2f2; border-color: #fecaca; }
    .gc-status-fail .gc-drag-result { color: var(--gc-fail); }
    .gc-status-fail .gc-drag-result-icon { background: var(--gc-fail); box-shadow: 0 2px 6px rgba(220, 38, 38, 0.3); }

    /* ---------- 结果态覆盖层 ---------- */
    .gc-result {
      position: absolute;
      inset: 0;
      z-index: 5;
      display: flex;
      flex-direction: column;
      align-items: center;
      justify-content: center;
      gap: 8px;
      pointer-events: none;
      animation: gc-fade-in .2s ease-out;
    }
    .gc-result-success { background: rgba(6, 78, 59, 0.5); }
    .gc-result-badge {
      display: flex;
      align-items: center;
      justify-content: center;
      width: 44px;
      height: 44px;
      border-radius: 50%;
      background: #10b981;
      color: #fff;
      box-shadow: 0 4px 14px rgba(16, 185, 129, 0.45), 0 0 0 4px rgba(255, 255, 255, 0.25);
      animation: gc-pop .3s ease-out;
    }
    .gc-result-text { font-size: 13px; font-weight: 500; color: #fff; text-shadow: 0 2px 4px rgba(0, 0, 0, 0.6); }
    .gc-result-fail { justify-content: flex-end; }
    .gc-result-banner {
      width: 100%;
      display: flex;
      align-items: center;
      justify-content: center;
      gap: 6px;
      padding: 7px 12px;
      background: rgba(185, 28, 28, 0.95);
      color: #fff;
      font-size: 12px;
      font-weight: 500;
    }

    /* ---------- 旋转题 ---------- */
    .gc-body-rotate {
      align-items: center;
      justify-content: center;
      margin: 0 auto;
      background: var(--gc-soft-2);
    }
    .gc-dial {
      position: relative;
      flex: none;
      display: flex;
      align-items: center;
      justify-content: center;
      border-radius: 50%;
      border: 2px dashed var(--gc-tint-200);
    }
    .gc-dial-dot { position: absolute; width: 8px; height: 8px; border-radius: 50%; background: #cbd5e1; }
    .gc-dial-dot-top { top: -4px; left: 50%; margin-left: -4px; background: var(--gc-primary); animation: gc-dot-breath 2.6s ease-in-out infinite; }
    .gc-dial-dot-bottom { bottom: -4px; left: 50%; margin-left: -4px; }
    .gc-dial-dot-left { left: -4px; top: 50%; margin-top: -4px; }
    .gc-dial-dot-right { right: -4px; top: 50%; margin-top: -4px; }
    .gc-body-inner { position: relative; background: #fff; }
    .gc-body-inner-rotate {
      border-radius: 100%;
      overflow: hidden;
      box-shadow: 0 0 0 2px #fff, 0 0 0 3px var(--gc-line), 0 10px 15px -3px rgba(0, 0, 0, 0.1), 0 4px 6px -4px rgba(0, 0, 0, 0.1);
    }
    .gc-picture-rotate {
      position: relative;
      max-width: 100%;
      max-height: 100%;
      z-index: 1;
      border-radius: 100%;
      overflow: hidden;
      display: flex;
      justify-content: center;
      align-items: center;
    }
    .gc-picture-rotate img { max-width: 100%; max-height: 100%; }
    .gc-thumb { position: absolute; z-index: 3; inset: 0; display: flex; justify-content: center; align-items: center; }
    .gc-thumb img { max-width: 100%; max-height: 100%; }
    .gc-thumb-block { width: 100%; height: 100%; display: flex; justify-content: center; align-items: center; }
    .gc-dial-cross {
      position: absolute;
      left: 50%;
      top: 50%;
      z-index: 3;
      width: 24px;
      height: 24px;
      margin: -12px 0 0 -12px;
      opacity: .4;
      pointer-events: none;
      background:
        linear-gradient(#fff, #fff) center / 100% 1px no-repeat,
        linear-gradient(#fff, #fff) center / 1px 100% no-repeat;
    }
    .gc-angle-badge {
      position: absolute;
      z-index: 5;
      left: 50%;
      bottom: 8px;
      transform: translateX(-50%);
      padding: 2px 10px;
      border-radius: 999px;
      border: 1px solid var(--gc-line);
      background: rgba(255, 255, 255, 0.92);
      backdrop-filter: blur(8px);
      color: var(--gc-primary-hover);
      font-family: ${MONO_STACK};
      font-size: 10px;
      font-weight: 600;
      white-space: nowrap;
      pointer-events: none;
      box-shadow: 0 1px 2px rgba(15, 23, 42, 0.06);
      animation: gc-badge 3s ease-in-out infinite;
    }
    .gc-type-rotate .gc-result-banner { width: 76%; margin-bottom: 18%; border-radius: 999px; }

    /* ---------- 触发按钮 ---------- */
    .gc-btn {
      position: relative;
      box-sizing: border-box;
      font-size: 14px;
      border-radius: 12px;
      letter-spacing: .5px;
      line-height: 1;
      white-space: nowrap;
      -webkit-appearance: none;
      outline: none;
      margin: 0;
      transition: background-color .15s, box-shadow .15s;
      font-weight: 500;
      user-select: none;
      display: flex;
      align-items: center;
      justify-content: center;
      font-family: ${FONT_STACK};
      box-shadow: 0 2px 8px rgba(15, 23, 42, 0.08);
    }
    .gc-btn span { padding-left: 8px; }
    .gc-btn-disabled { pointer-events: none; }
    .gc-btn-default {
      --gc-primary: ${primary};
      color: var(--gc-primary);
      border: 1px solid color-mix(in srgb, var(--gc-primary) 40%, #fff);
      background-color: color-mix(in srgb, var(--gc-primary) 7%, #fff);
      cursor: pointer;
    }
    .gc-btn-default:hover {
      background-color: color-mix(in srgb, var(--gc-primary) 14%, #fff);
      box-shadow: 0 4px 14px color-mix(in srgb, var(--gc-primary) 22%, transparent);
    }
    .gc-btn-error { cursor: pointer; color: #b91c1c; background-color: #fef2f2; border: 1px solid #fecaca; }
    .gc-btn-warn { cursor: pointer; color: #b45309; background-color: #fffbeb; border: 1px solid #fde68a; }
    .gc-btn-success { color: #047857; background-color: #ecfdf5; border: 1px solid #a7f3d0; pointer-events: none; }
    .gc-btn-ripple { position: relative; display: flex; align-items: center; justify-content: center; }
    .gc-btn-ripple svg, .gc-btn-ripple > * { position: relative; z-index: 2; }
    .gc-btn-ripple::after {
      background-color: var(--gc-primary);
      border-radius: 50px;
      content: '';
      display: block;
      width: 21px; height: 21px;
      opacity: 0;
      position: absolute;
      top: 50%; left: 50%;
      margin-top: -11px; margin-left: -11px;
      z-index: 1;
      animation: gc-ripple 1.3s infinite;
      animation-delay: 2s;
    }

    /* ---------- 动效 ---------- */
    @keyframes gc-card-in {
      0% { opacity: 0; transform: translateY(18px) scale(.985); }
      100% { opacity: 1; transform: translateY(0) scale(1); }
    }
    @keyframes gc-fade-in { from { opacity: 0; } to { opacity: 1; } }
    @keyframes gc-pop { 0% { transform: scale(.5); } 70% { transform: scale(1.08); } 100% { transform: scale(1); } }
    @keyframes gc-dot-in { from { transform: scale(.5); opacity: 0; } to { transform: scale(1); opacity: 1; } }
    @keyframes gc-radar {
      0% { box-shadow: 0 0 0 0 color-mix(in srgb, var(--gc-primary-light) 60%, transparent), 0 4px 10px rgba(0, 0, 0, 0.3); }
      70% { box-shadow: 0 0 0 9px transparent, 0 4px 10px rgba(0, 0, 0, 0.3); }
      100% { box-shadow: 0 0 0 0 transparent, 0 4px 10px rgba(0, 0, 0, 0.3); }
    }
    @keyframes gc-shimmer {
      0% { background-position: -200% center; box-shadow: 0 4px 14px 0 color-mix(in srgb, var(--gc-primary) 28%, transparent); }
      50% { box-shadow: 0 4px 22px 2px color-mix(in srgb, var(--gc-primary-light) 48%, transparent); }
      100% { background-position: 200% center; box-shadow: 0 4px 14px 0 color-mix(in srgb, var(--gc-primary) 28%, transparent); }
    }
    @keyframes gc-nudge {
      0%, 65%, 100% { transform: translateX(0); }
      73% { transform: translateX(10px); }
      82% { transform: translateX(-2px); }
      90% { transform: translateX(5px); }
    }
    @keyframes gc-float { 0%, 100% { transform: translateY(0); } 50% { transform: translateY(-4px); } }
    @keyframes gc-pill {
      0%, 100% { box-shadow: 0 0 0 0 rgba(16, 185, 129, 0.4); }
      50% { box-shadow: 0 0 0 4px rgba(16, 185, 129, 0); }
    }
    @keyframes gc-shield {
      0%, 100% { opacity: .6; transform: scale(1); }
      50% { opacity: 1; transform: scale(1.08); filter: drop-shadow(0 0 5px color-mix(in srgb, var(--gc-primary-light) 60%, transparent)); }
    }
    @keyframes gc-dot-breath {
      0%, 100% { transform: scale(1); box-shadow: 0 0 0 2px var(--gc-tint-200), 0 0 0 0 color-mix(in srgb, var(--gc-primary-light) 50%, transparent); }
      50% { transform: scale(1.15); box-shadow: 0 0 0 3px var(--gc-tint-200), 0 0 8px 3px color-mix(in srgb, var(--gc-primary-light) 60%, transparent); }
    }
    @keyframes gc-badge {
      0%, 100% { border-color: var(--gc-line); box-shadow: 0 1px 3px rgba(0, 0, 0, 0.05); }
      50% { border-color: var(--gc-primary-lighter); box-shadow: 0 0 10px color-mix(in srgb, var(--gc-primary-light) 25%, transparent); }
    }
    @keyframes gc-ripple {
      0% { opacity: 0; }
      5% { opacity: 0.05; }
      20% { opacity: 0.35; }
      65% { opacity: 0.01; }
      100% { transform: scaleX(2) scaleY(2); opacity: 0; }
    }

    /* ---------- 窄屏：保持弹窗形态，仅放大触控区域 ---------- */
    @media (max-width: 480px) {
      .gc-wrapper { --gc-handle: 40px; --gc-track-h: 48px; }
      .gc-icon-block button { width: 32px; height: 32px; }
      .gc-confirm { height: 44px; }
    }
    @media (prefers-reduced-motion: reduce) {
      .gc-wrapper-theme, .gc-dot, .gc-confirm, .gc-drag-idle, .gc-tile-region, .gc-region-tip em,
      .gc-secure-icon, .gc-dial-dot-top, .gc-angle-badge, .gc-btn-ripple::after,
      .gc-result, .gc-result-badge { animation: none !important; }
      .gc-btn-refresh, .gc-btn-close { transition: none !important; }
    }
  `;
  }, [theme]);

  // Injecting directly as style block since they are small enough
  return <style>{css}</style>;
};
