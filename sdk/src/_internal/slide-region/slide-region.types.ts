export interface SlideRegionConfig {
  width?: number;
  height?: number;
  verticalPadding?: number;
  horizontalPadding?: number;
  showTheme?: boolean;
  title?: string;
  iconSize?: number;
  scope?: boolean;
}

export const defaultSlideRegionConfig = (): SlideRegionConfig => ({
  width: 300,
  height: 220,
  verticalPadding: 16,
  horizontalPadding: 12,
  showTheme: true,
  iconSize: 22,
  scope: true,
});

export interface SlideRegionData {
  thumbX: number;
  thumbY: number;
  thumbWidth: number;
  thumbHeight: number;
  image: string;
  thumb: string;
}

export const defaultSlideRegionData = (): SlideRegionData => ({
  thumbX: 0,
  thumbY: 0,
  thumbWidth: 0,
  thumbHeight: 0,
  image: '',
  thumb: '',
});

export interface SlideRegionPoint {
  x: number;
  y: number;
}

export interface SlideRegionEvent {
  move?: (x: number, y: number) => void;
  refresh?: () => void;
  close?: () => void;
  confirm?: (point: SlideRegionPoint, reset: () => void) => void;
}
