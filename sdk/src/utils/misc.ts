import type { CaptchaType } from '../Captcha.types';

export function noop() {
}

const actions: CaptchaType[] = [ 'click-text', 'click-shape', 'slide-text', 'rotate' ];

export function getRandomType(): CaptchaType {
  return actions[Math.floor(Math.random() * actions.length)];
}
