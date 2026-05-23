import { useMemo } from 'react';
import { useCaptchaConfig } from './CaptchaProvider';

export const CaptchaStyle = () => {
  const { theme } = useCaptchaConfig();

  const css = useMemo(() => `
    .gc-wrapper {
      --gc-text: ${theme.textColor || '#333'};
      --gc-bg: ${theme.backgroundColor || '#fff'};
      --gc-primary: ${theme.primaryColor || '#4e87ff'};
      --gc-disabled: #749ff9;
      --gc-border: ${theme.borderColor || 'rgba(206, 223, 254, 0.5)'};
      --gc-icon: #3C3C3C;
      --gc-drag-bar: #e0e0e0;
      --gc-drag-bg: ${theme.primaryColor || '#3e7cff'};
      --gc-drag-icon: #ffffff;
      --gc-round: #e0e0e0;
      --gc-loading: ${theme.primaryColor || '#3e7cff'};
      --gc-body-bg: #34383e;
      --gc-dot-color: #cedffe;
      --gc-dot-bg: ${theme.primaryColor || '#3e7cff'};
      --gc-dot-border: #f7f9fb;
      --gc-btn-color: #ffffff;
      
      padding: 12px 16px;
      -webkit-touch-callout: none;
      user-select: none;
      box-sizing: border-box;
    }

    .gc-wrapper-theme {
      border: 1px solid var(--gc-border);
      border-radius: ${theme.borderRadius || 8}px;
      box-shadow: 0 0 20px rgba(100, 100, 100, 0.1);
      background-color: var(--gc-bg);
    }

    .gc-hide { visibility: hidden; }
    .gc-disabled {
      pointer-events: none;
      background-color: var(--gc-disabled);
      border-color: var(--gc-disabled);
    }

    .gc-header {
      height: 36px;
      width: 100%;
      font-size: 15px;
      color: var(--gc-text);
      display: flex;
      align-items: center;
      user-select: none;
    }
    .gc-header span {
      flex: 1;
      padding-right: 5px;
    }
    .gc-header em {
      padding: 0 3px;
      font-weight: bold;
      color: var(--gc-primary);
      font-style: normal;
    }
    .gc-header-center { text-align: center; }

    .gc-body {
      position: relative;
      width: 100%;
      margin-top: 10px;
      display: flex;
      background: var(--gc-body-bg);
      border-radius: 5px;
      overflow: hidden;
    }
    .gc-body-rotate {
      background: transparent !important;
      justify-content: center;
      align-items: center;
      margin: 10px auto 0;
    }
    .gc-body-inner {
      position: relative;
      background: var(--gc-body-bg);
    }
    .gc-body-inner-rotate { border-radius: 100%; }

    .gc-picture {
      position: relative;
      z-index: 2;
      width: 100%;
    }
    .gc-picture-rotate {
      position: relative;
      max-width: 100%;
      max-height: 100%;
      z-index: 2;
      border-radius: 100%;
      overflow: hidden;
      display: flex;
      justify-content: center;
      align-items: center;
    }
    .gc-picture-rotate img { max-width: 100%; max-height: 100%; }

    .gc-loading {
      position: absolute;
      z-index: 1;
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
    .gc-loading svg, .gc-loading circle {
      color: var(--gc-loading);
      fill: var(--gc-loading);
    }

    .gc-footer {
      width: 100%;
      height: 50px;
      color: #34383e;
      display: flex;
      align-items: center;
      padding-top: 10px;
      user-select: none;
    }

    .gc-icon-block {
      flex: 1;
      display: flex;
      align-items: center;
    }
    .gc-icon-block svg {
      color: var(--gc-icon);
      fill: var(--gc-icon);
      margin: 0 5px;
      cursor: pointer;
    }

    .gc-button-block {
      width: 120px;
      height: 40px;
    }
    .gc-button-block button {
      width: 100%;
      height: 40px;
      text-align: center;
      padding: 9px 15px;
      font-size: 15px;
      border-radius: 5px;
      display: inline-block;
      line-height: 1;
      white-space: nowrap;
      cursor: pointer;
      color: var(--gc-btn-color);
      background-color: var(--gc-primary);
      border: 1px solid var(--gc-primary);
      -webkit-appearance: none;
      box-sizing: border-box;
      outline: none;
      margin: 0;
      transition: .1s;
      font-weight: 500;
      user-select: none;
    }

    .gc-dots {
      position: absolute;
      top: 0; right: 0; left: 0; bottom: 0;
    }
    .gc-dot {
      position: absolute;
      z-index: 2;
      width: 22px;
      height: 22px;
      color: var(--gc-dot-color);
      background: var(--gc-dot-bg);
      border: 3px solid var(--gc-dot-border);
      display: flex;
      align-items: center;
      justify-content: center;
      border-radius: 22px;
      cursor: default;
    }

    .gc-tile {
      position: absolute;
      z-index: 2;
      cursor: pointer;
    }
    .gc-tile img {
      display: block;
      cursor: pointer;
      width: 100%;
      height: 100%;
    }
    .gc-tile-region { user-select: none; }

    .gc-drag-slide-bar {
      width: 100%;
      height: 100%;
      position: relative;
      touch-action: none;
    }
    .gc-drag-line {
      position: absolute;
      height: 14px;
      background-color: var(--gc-drag-bar);
      left: 0; right: 0; top: 50%;
      margin-top: -7px;
      border-radius: 7px;
    }
    .gc-drag-block {
      position: absolute;
      left: 0;
      top: 50%;
      margin-top: -20px;
      width: 82px;
      height: 40px;
      z-index: 2;
      background-color: var(--gc-drag-bg);
      cursor: pointer;
      display: flex;
      align-items: center;
      justify-content: center;
      user-select: none;
      border-radius: 24px;
      box-shadow: 0 0 20px rgba(100, 100, 100, 0.35);
    }
    .gc-drag-block svg {
      color: var(--gc-drag-icon);
      fill: var(--gc-drag-icon);
    }
    .gc-drag-block-inline {
      width: 100%;
      height: 100%;
      display: flex;
      align-items: center;
      justify-content: center;
    }

    .gc-round {
      position: absolute;
      top: 0; left: 0; right: 0; bottom: 0;
      border-radius: 100%;
      z-index: 2;
      border: 6px solid var(--gc-round);
    }
    .gc-thumb {
      position: absolute;
      z-index: 2;
      top: 0; left: 0; right: 0; bottom: 0;
      display: flex;
      justify-content: center;
      align-items: center;
    }
    .gc-thumb img { max-width: 100%; max-height: 100%; }
    .gc-thumb-block {
      width: 100%; height: 100%;
      display: flex; justify-content: center; align-items: center;
    }

    /* Button specific styles from original less */
    .gc-btn {
      position: relative;
      box-sizing: border-box;
      font-size: 13px;
      border-radius: 5px;
      letter-spacing: 1px;
      line-height: 1;
      white-space: nowrap;
      -webkit-appearance: none;
      outline: none;
      margin: 0;
      transition: .1s;
      font-weight: 500;
      user-select: none;
      display: flex;
      align-items: center;
      justify-content: center;
      box-shadow: 0 0 20px rgba(62, 124, 255, 0.1);
    }
    .gc-btn span { padding-left: 8px; }
    .gc-btn-disabled { pointer-events: none; }
    
    .gc-btn-default {
      color: var(--gc-primary);
      border: 1px solid var(--gc-primary);
      background-color: #ecf5ff;
      cursor: pointer;
    }
    .gc-btn-default:hover {
      background-color: #e0efff !important;
    }

    .gc-btn-error {
      cursor: pointer;
      color: #ed4630;
      background-color: #fef0f0;
      border: 1px solid #ff5a34;
    }

    .gc-btn-warn {
      cursor: pointer;
      color: #ffa000;
      background-color: #fdf6ec;
      border: 1px solid #ffbe09;
    }

    .gc-btn-success {
      color: #5eaa2f;
      background-color: #f0f9eb;
      border: 1px solid #8bc640;
      pointer-events: none;
    }

    .gc-btn-ripple {
      position: relative;
      display: flex;
      align-items: center;
      justify-content: center;
    }
    .gc-btn-ripple svg, .gc-btn-ripple > * {
      position: relative;
      z-index: 2;
    }
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

    @keyframes gc-ripple {
      0% { opacity: 0; }
      5% { opacity: 0.05; }
      20% { opacity: 0.35; }
      65% { opacity: 0.01; }
      100% {
        transform: scaleX(2) scaleY(2);
        opacity: 0;
      }
    }
  `, [theme]);

  // Injecting directly as style block since they are small enough
  return <style>{css}</style>;
}
