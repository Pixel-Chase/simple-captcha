export interface SlideConfig {
  width?: number;
  height?: number;
  verticalPadding?: number;
  horizontalPadding?: number;
  showTheme?: boolean;
  scope?: boolean;
}

export const defaultSlideConfig = (): SlideConfig => ({
  width: 300,
  height: 200,
  verticalPadding: 20,
  horizontalPadding: 20,
  showTheme: true,
  scope: true,
});

export interface SlideData {
  thumbX: number;
  thumbY: number;
  thumbWidth: number;
  thumbHeight: number;
  image: string;
  thumb: string;
}

export const defaultSlideData = (): SlideData => ({
  thumbX: 0,
  thumbY: 0,
  thumbWidth: 0,
  thumbHeight: 0,
  image: '',
  thumb: '',
});

export interface SlidePoint {
  x: number;
  y: number;
}

export interface SlideEvent {
  move?: (x: number, y: number) => void;
  refresh?: () => void;
  close?: () => void;
  confirm?: (point: SlidePoint, reset: () => void) => void;
}
