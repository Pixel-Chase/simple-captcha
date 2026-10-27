import type { ReactNode } from 'react';
import type { CaptchaError } from './CaptchaError';

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
  clickTitle?: string;
  slideTitle?: string;
  confirmButton?: string;
  /** 滑动区域 / 旋转题型的标题（点选沿用 clickTitle，滑块拼图沿用 slideTitle） */
  regionTitle?: string;
  rotateTitle?: string;
  /** 各题型标题下的副标题 */
  clickSubtitle?: string;
  slideSubtitle?: string;
  regionSubtitle?: string;
  rotateSubtitle?: string;
  /** 各题型标题右侧的小标签，设为空字符串可隐藏 */
  clickTag?: string;
  slideTag?: string;
  regionTag?: string;
  rotateTag?: string;
  /** 点选：提示条文案 / 已点击计数（{n} 为数量） / 清除重选 */
  clickHint?: string;
  clickMeta?: string;
  clickClear?: string;
  /** 滑块 / 旋转：轨道内的待拖动提示，以及轨道下方的说明 */
  slideHint?: string;
  rotateHint?: string;
  slideProgress?: string;
  rotateProgress?: string;
  /** 滑块：轨道下方右侧的拖动进度，{p} 为百分比数值 */
  slidePercent?: string;
  /** 滑动区域：图片下方的操作提示与角标，以及图块复位按钮 */
  regionHint?: string;
  regionBadge?: string;
  regionReset?: string;
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
  /** 卡片底部的安全提示，设为空字符串可隐藏 */
  regionNote?: string;
  slideNote?: string;
  rotateNote?: string;
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

/**
 * 验证码服务接入参数
 * - baseURL：验证码服务的 origin，例如 https://captcha.example.com；不传表示与页面同源。
 *   API 路径固定为 /api/v1，验证码服务应部署在独立域名下，不支持挂在子路径
 * - appId：管理后台「接入应用」中的应用 ID；服务端只接受该应用允许来源发起的出题与提交
 */
export interface ApiConfig {
  baseURL?: string;
  appId: string;
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

/** 出题请求体（POST /captchas/:type）：出题前采集的环境信号随请求上报，服务端据此决定是否加难 */
export interface CaptchaIssuePayload {
  mode?: 'light' | 'dark';
  /** 取整并裁剪到 1-3 的 devicePixelRatio */
  dpr: number;
  /** telemetry='off' 时不携带 */
  env?: CaptchaEnv;
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
  /** 无头浏览器 / API 篡改痕迹标记（如 zero_outer_size、worker_ua_mismatch、getter_patched:webdriver），没有命中时不携带 */
  anomalies?: string[];
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

export interface CaptchaProps {
  /** 题型，auto 由服务端按风险等级选择 */
  type?: CaptchaType;
  /** 连续答错多少次后关闭弹窗，并以 MAX_ATTEMPTS 触发 onFail */
  maxAttempts?: number;
  /** 题图渲染宽度（CSS 像素），高度按 3:2 推算 */
  width?: number;
  onSuccess?: (data: CaptchaVerifySuccessData) => void;
  /** 题目加载失败或答错次数用完时触发；用户关闭弹窗只触发 onCancel */
  onFail?: (error: CaptchaError) => void;
  onCancel?: () => void;
  children?: ReactNode;
}

/** useCaptcha().verify 的单次参数 */
export type CaptchaVerifyOptions = Omit<CaptchaProps, 'children'>;

export interface CaptchaRef {
  /** 打开弹窗并加载题目，加载结束后 resolve；结果只经由 onSuccess / onFail / onCancel 通知 */
  open: () => Promise<void>;
}
