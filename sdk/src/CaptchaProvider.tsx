import { createContext, useContext, useMemo } from 'react';
import type { ReactNode } from 'react';
import type {
  CaptchaGlobalConfig,
  LocaleTexts,
  ThemeTokens,
  ApiConfig,
  TelemetryLevel,
} from './Captcha.types';

export interface InternalConfig {
  locale: LocaleTexts;
  theme: ThemeTokens;
  api: ApiConfig;
  zIndex: number;
  telemetry: TelemetryLevel;
}

const defaultLocale: LocaleTexts = {
  userCancel: '用户取消',
  loading: '加载中...',
  clickTitle: '安全验证',
  slideTitle: '拼图验证',
  regionTitle: '空间拼图',
  rotateTitle: '旋转验证',
  clickSubtitle: '请依次点击图中的文字',
  slideSubtitle: '拖动滑块使拼图吻合',
  regionSubtitle: '直接拖动图块至缺口，松手提交',
  rotateSubtitle: '拖动滑块使图片角度摆正',
  confirmButton: '确认提交',
  buttonText: '点击按键进行验证',
  clickHint: '请按顺序点选：',
  clickMeta: '已点击 {n} 处',
  clickClear: '清除重选',
  slideHint: '向右拖动滑块完成拼图',
  rotateHint: '向右拖动滑块使图片摆正',
  slideProgress: '拖动至与缺口精准咬合',
  rotateProgress: '松手自动校准判定',
  regionHint: '按住图块拖入缺口',
  regionBadge: '松手即验',
  rotateAngle: '当前偏移',
  successText: '验证通过',
  elapsedText: '耗时 {s}s',
  doneText: '已完成',
  failTitle: '验证未通过，请重试',
  failBanner: '正在重新生成验证码…',
  trackSuccess: '验证已通过',
  trackFail: '验证未通过，请重新拖动',
  poweredBy: '行为验证保护中',
  refreshLabel: '刷新',
  closeLabel: '关闭',
};

const defaultTheme: ThemeTokens = {
  primaryColor: '#4f46e5',
  backgroundColor: '#ffffff',
  textColor: '#0f172a',
  borderColor: '#e2e8f0',
  borderRadius: 16,
};

const defaultConfig: InternalConfig = {
  locale: defaultLocale,
  theme: defaultTheme,
  api: { prefix: '/api/v1' },
  zIndex: 9999,
  telemetry: 'full',
};

const CaptchaConfigContext = createContext<InternalConfig>(defaultConfig);

export interface CaptchaProviderProps extends CaptchaGlobalConfig {
  children: ReactNode;
}

export const CaptchaProvider = ({
  locale,
  theme,
  api,
  zIndex,
  telemetry,
  children,
}: CaptchaProviderProps) => {
  const parentConfig = useContext(CaptchaConfigContext);

  const value = useMemo<InternalConfig>(
    () => ({
      locale: { ...parentConfig.locale, ...locale },
      theme: { ...parentConfig.theme, ...theme },
      api: { ...parentConfig.api, ...api },
      zIndex: zIndex ?? parentConfig.zIndex,
      telemetry: telemetry ?? parentConfig.telemetry,
    }),
    [api, locale, parentConfig, telemetry, theme, zIndex],
  );

  return <CaptchaConfigContext.Provider value={value}>{children}</CaptchaConfigContext.Provider>;
};

export const useCaptchaConfig = () => useContext(CaptchaConfigContext);
