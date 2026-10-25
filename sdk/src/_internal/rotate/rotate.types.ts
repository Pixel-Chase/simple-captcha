export interface RotateConfig {
  width?: number;
  height?: number;
  size?: number;
  verticalPadding?: number;
  horizontalPadding?: number;
  showTheme?: boolean;
  scope?: boolean;
}

export const defaultRotateConfig = (): RotateConfig => ({
  width: 300,
  height: 220,
  size: 128,
  verticalPadding: 20,
  horizontalPadding: 20,
  showTheme: true,
  scope: true,
});

export interface RotateData {
  angle?: number;
  image: string;
  thumb: string;
  thumbSize: number;
}

export const defaultRotateData = (): RotateData => ({
  angle: 0,
  image: '',
  thumb: '',
  thumbSize: 0,
});

export interface RotateEvent {
  rotate?: (angle: number) => void;
  refresh?: () => void;
  close?: () => void;
  confirm?: (angle: number, reset: () => void) => void;
}
