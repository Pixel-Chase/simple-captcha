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
  clickTitle: '安全点选验证',
  slideTitle: '拼图验证',
  regionTitle: '空间拼图',
  rotateTitle: '旋转验证',
  clickSubtitle: '请依序点击图中的指定汉字',
  slideSubtitle: '拖动滑块使拼图完全吻合',
  regionSubtitle: '自由拖动图块至缺口，松手提交',
  rotateSubtitle: '拖动滑块使图片角度摆正',
  clickTag: '风控防护',
  slideTag: '高精咬合',
  regionTag: '免滑条',
  rotateTag: '角度矫正',
  confirmButton: '确认点选',
  buttonText: '点击按键进行验证',
  clickHint: '请依序点击：',
  clickMeta: '已依序选择 {n} 处',
  clickClear: '重选',
  slideHint: '向右拖动滑块完成拼图',
  rotateHint: '向右拖动滑块使图片摆正',
  slideProgress: '拖动至与缺口精准咬合',
  rotateProgress: '松手自动校准判定',
  slidePercent: '当前进度 {p}%',
  regionHint: '按住图块自由拖拽入框',
  regionBadge: '松手即验',
  regionReset: '原位重置',
  rotateAngle: '当前偏移',
  successText: '验证通过',
  elapsedText: '耗时 {s}s',
  doneText: '已完成',
  failTitle: '验证未通过，请重试',
  failBanner: '正在重新生成验证码…',
  trackSuccess: '验证已通过',
  trackFail: '验证未通过，请重新拖动',
  poweredBy: '二维拖拽轨迹与运动特征分析',
  slideNote: '多维速度曲线与轨迹时序分析',
  rotateNote: '拖动速度与微抖动行为分析',
  refreshLabel: '刷新',
  closeLabel: '关闭',
};

const defaultTheme: ThemeTokens = {
  primaryColor: '#3904e7',
  backgroundColor: '#ffffff',
  textColor: '#1b1a25',
  borderColor: '#e3e8ee',
  borderRadius: 24,
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
