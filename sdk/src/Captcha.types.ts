import type { ReactNode, CSSProperties } from 'react';

export type CaptchaType = 'auto' | 'click-text' | 'click-shape' | 'slide-text' | 'slide-region' | 'rotate';

export interface LocaleTexts {
  userCancel?: string;
  loading?: string;
  clickTitle?: string;
  slideTitle?: string;
  confirmButton?: string;
  buttonText?: string;
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
}

export interface CaptchaErrorResponse {
  code: number;
  error: string;
}

export interface CaptchaChallengeData {
  key: string;
  image: string;
  thumb?: string;
  thumbWidth?: number;
  thumbHeight?: number;
  thumbX?: number;
  thumbY?: number;
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
}

export interface CaptchaInstanceOptions {
  type?: CaptchaType;
  config?: CaptchaConfig;
  className?: string;
  style?: CSSProperties;
  locale?: LocaleTexts;
  theme?: ThemeTokens;
  api?: ApiConfig;
}

export interface CaptchaVerifyOptions extends CaptchaInstanceOptions {
  onCancel?: () => void;
  onFail?: (error: CaptchaErrorResponse | string) => void;
  onSuccess?: (data: CaptchaVerifySuccessData) => void;
}

export interface CaptchaRef {
  verify: () => Promise<void>;
}

export interface CaptchaProps extends CaptchaVerifyOptions {
  children?: ReactNode;
}
