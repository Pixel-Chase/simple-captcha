export { default as Click } from './click/Click';
export { default as Slide } from './slide/Slide';
export { default as Rotate } from './rotate/Rotate';
export { default as SlideRegion } from './slide-region/SlideRegion';
export { default as Button } from './button/Button';

// Re-export types that are needed by upper layers
export type { ClickDot } from './click/click.types';
export type { SlidePoint } from './slide/slide.types';
export type { SlideRegionPoint } from './slide-region/slide-region.types';
