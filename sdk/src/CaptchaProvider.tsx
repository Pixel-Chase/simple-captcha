import { createContext, useContext, useMemo } from 'react';
import type { CaptchaGlobalConfig, LocaleTexts, ThemeTokens, ApiConfig } from './Captcha.types';

export interface InternalConfig {
  locale: LocaleTexts;
  theme: ThemeTokens;
  api: ApiConfig;
  zIndex: number;
}

const defaultLocale: LocaleTexts = {
  userCancel: '用户取消',
  loading: '加载中...',
  clickTitle: '请在下图依次点击',
  slideTitle: '请拖动滑块完成拼图',
  confirmButton: '确认',
  buttonText: '点击按键进行验证',
};

const defaultTheme: ThemeTokens = {
  primaryColor: '#347eff',
  backgroundColor: '#ffffff',
  textColor: '#333333',
  borderColor: '#e2e2e2',
  borderRadius: 4,
};

const defaultConfig: InternalConfig = {
  locale: defaultLocale,
  theme: defaultTheme,
  api: {},
  zIndex: 9999,
};

const CaptchaConfigContext = createContext<InternalConfig>(defaultConfig);

export interface CaptchaProviderProps extends CaptchaGlobalConfig {
  children: React.ReactNode;
}

export const CaptchaProvider = ({ locale, theme, api, zIndex, children }: CaptchaProviderProps) => {
  const parentConfig = useContext(CaptchaConfigContext);

  const value = useMemo<InternalConfig>(() => ({
    locale: { ...parentConfig.locale, ...locale },
    theme: { ...parentConfig.theme, ...theme },
    api: { ...parentConfig.api, ...api },
    zIndex: zIndex ?? parentConfig.zIndex,
  }), [api, locale, parentConfig, theme, zIndex]);

  return <CaptchaConfigContext.Provider value={value}>{children}</CaptchaConfigContext.Provider>;
};

export const useCaptchaConfig = () => useContext(CaptchaConfigContext);
