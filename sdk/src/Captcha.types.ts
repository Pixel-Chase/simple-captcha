import type { ReactNode, CSSProperties } from 'react';

export type CaptchaType =
  | 'auto'
  | 'click-text'
  | 'click-shape'
  | 'slide-text'
  | 'slide-region'
  | 'rotate';

/** 服务端实际下发的具体题型（auto 解析后的结果） */
export type ResolvedCaptchaType = Exclude<CaptchaType, 'auto'>;

/**
 * 遥测采集级别
 * - off：不采集任何行为/环境信息（PoW 仍会计算）
 * - basic：采集交互轨迹与基础环境信息（UA、语言、屏幕、时区等）
 * - full：在 basic 基础上额外采集 WebGL 渲染器与设备指纹哈希
 */
export type TelemetryLevel = 'off' | 'basic' | 'full';

export interface LocaleTexts {
  userCancel?: string;
  loading?: string;
  clickTitle?: string;
  slideTitle?: string;
  confirmButton?: string;
  buttonText?: string;
  /** 滑动区域 / 旋转题型的标题（点选沿用 clickTitle，滑块拼图沿用 slideTitle） */
  regionTitle?: string;
  rotateTitle?: string;
  /** 各题型标题下的副标题 */
  clickSubtitle?: string;
  slideSubtitle?: string;
  regionSubtitle?: string;
  rotateSubtitle?: string;
  /** 点选：提示条文案 / 已点击计数（{n} 为数量） / 清除重选 */
  clickHint?: string;
  clickMeta?: string;
  clickClear?: string;
  /** 滑块 / 旋转：轨道内的待拖动提示，以及轨道下方的说明 */
  slideHint?: string;
  rotateHint?: string;
  slideProgress?: string;
  rotateProgress?: string;
  /** 滑动区域：图片底部的操作提示与角标 */
  regionHint?: string;
  regionBadge?: string;
  /** 旋转：角度角标前缀 */
  rotateAngle?: string;
  /** 校验结果态文案；elapsedText 中 {s} 为耗时秒数 */
  successText?: string;
  elapsedText?: string;
  doneText?: string;
  failTitle?: string;
  failBanner?: string;
  trackSuccess?: string;
  trackFail?: string;
  /** 滑动区域底部的安全提示，设为空字符串可隐藏 */
  poweredBy?: string;
  refreshLabel?: string;
  closeLabel?: string;
}

export interface ThemeTokens {
  primaryColor?: string;
  backgroundColor?: string;
  textColor?: string;
  borderColor?: string;
  borderRadius?: number;
}

export interface ApiConfig {
  basePath?: string;
  prefix?: string;
}

export interface CaptchaErrorResponse {
  code: number;
  error: string;
}

/** 服务端下发的工作量证明挑战 */
export interface CaptchaPowChallenge {
  nonce: string;
  /** SHA-256 前导零位数 */
  difficulty: number;
}

/** 提交给服务端的工作量证明解答 */
export interface CaptchaPowSolution {
  nonce: string;
  solution: string;
}

export interface CaptchaChallengeData {
  key: string;
  type?: ResolvedCaptchaType;
  image: string;
  thumb?: string;
  thumbWidth?: number;
  thumbHeight?: number;
  thumbX?: number;
  thumbY?: number;
  /** 主图位图尺寸，SDK 据此把 CSS 像素坐标换算回图像坐标 */
  imageWidth?: number;
  imageHeight?: number;
  /** 出图倍率（1-3） */
  scale?: number;
  /** 难度等级（1-5） */
  difficulty?: number;
  pow?: CaptchaPowChallenge;
}

/** 轨迹采样点 [x, y, t]，t 为相对题目加载完成的毫秒数 */
export type TracePoint = [number, number, number];

/** 验证过程中的交互轨迹 */
export interface CaptchaTrace {
  points: TracePoint[];
  clicks?: TracePoint[];
  pointerType?: string;
  startedAt: number;
  submittedAt: number;
  downs: number;
  ups: number;
  moves: number;
  untrusted: number;
}

/** 运行环境信号 */
export interface CaptchaEnv {
  ua?: string;
  platform?: string;
  languages?: string[];
  tz?: string;
  tzOffset?: number;
  screen?: [number, number];
  viewport?: [number, number];
  dpr?: number;
  touchPoints?: number;
  cores?: number;
  memory?: number;
  webdriver?: boolean;
  plugins?: number;
  webgl?: string;
  automation?: string[];
  fp?: string;
  visibilityChanges?: number;
  mobile?: boolean;
}

/** 提交答案时的完整请求体 */
export interface CaptchaVerifyPayload {
  dots?: string;
  point?: string;
  angle?: string;
  trace?: CaptchaTrace;
  env?: CaptchaEnv;
  pow?: CaptchaPowSolution;
}

export interface CaptchaVerifySuccessData {
  second_key: string;
}

export interface CaptchaStateData {
  valid: boolean;
}

export interface CaptchaConfig {
  width?: number;
  height?: number;
  /** 旋转验证码内圆区域尺寸 */
  size?: number;
  thumbWidth?: number;
  thumbHeight?: number;
  verticalPadding?: number;
  horizontalPadding?: number;
  showTheme?: boolean;
  title?: string;
  iconSize?: number;
  scope?: boolean;
  buttonText?: string;
  dotSize?: number;
}

export interface CaptchaGlobalConfig {
  locale?: LocaleTexts;
  theme?: ThemeTokens;
  api?: ApiConfig;
  zIndex?: number;
  telemetry?: TelemetryLevel;
}

export interface CaptchaInstanceOptions {
  type?: CaptchaType;
  config?: CaptchaConfig;
  className?: string;
  style?: CSSProperties;
  locale?: LocaleTexts;
  theme?: ThemeTokens;
  api?: ApiConfig;
  telemetry?: TelemetryLevel;
}

export interface CaptchaVerifyOptions extends CaptchaInstanceOptions {
  onCancel?: () => void;
  onFail?: (error: CaptchaErrorResponse | string) => void;
  onSuccess?: (data: CaptchaVerifySuccessData) => void;
  maxRetryCount?: number;
}

export interface CaptchaRef {
  verify: () => Promise<void>;
}

export interface CaptchaProps extends CaptchaVerifyOptions {
  children?: ReactNode;
  /** 弹窗彻底关闭后触发（含校验结果画面的展示时间），useCaptcha 据此延后卸载 */
  onClosed?: () => void;
}
