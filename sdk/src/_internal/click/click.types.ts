export interface ClickConfig {
  width?: number;
  height?: number;
  thumbWidth?: number;
  thumbHeight?: number;
  verticalPadding?: number;
  horizontalPadding?: number;
  showTheme?: boolean;
  title?: string;
  buttonText?: string;
  iconSize?: number;
  dotSize?: number;
}

export const defaultClickConfig = (): ClickConfig => ({
  width: 300,
  height: 220,
  thumbWidth: 112,
  thumbHeight: 30,
  verticalPadding: 20,
  horizontalPadding: 20,
  showTheme: true,
  iconSize: 22,
  dotSize: 28,
});

export interface ClickData {
  image: string;
  thumb: string;
}

export interface ClickDot {
  key: number;
  index: number;
  x: number;
  y: number;
}

export interface ClickEvent {
  click?: (x: number, y: number) => void;
  callback?: () => void;
  refresh?: () => void;
  close?: () => void;
  confirm?: (dots: Array<ClickDot>, reset: () => void) => void;
}
