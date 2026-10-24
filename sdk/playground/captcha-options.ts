import type { CaptchaType } from '@aetherlib/g-captcha';

/** 演示站里可选的题型，登录页与文档页的在线体验共用 */
export const CAPTCHA_TYPE_OPTIONS: ReadonlyArray<{ value: CaptchaType; label: string }> = [
  { value: 'auto', label: '智能选择（由服务端按风险决定）' },
  { value: 'click-text', label: '文字点选' },
  { value: 'click-shape', label: '图形点选' },
  { value: 'slide-text', label: '滑块拼图' },
  { value: 'slide-region', label: '滑动区域' },
  { value: 'rotate', label: '旋转还原' },
];

export const captchaTypeLabel = (type: CaptchaType) =>
  CAPTCHA_TYPE_OPTIONS.find((item) => item.value === type)?.label ?? type;
