import { useCallback, useState } from 'react';
import type { ClickData, ClickDot, ClickEvent } from './click.types';
import { getDomXY } from '../helpers';

export const useClickHandler = (
  _: ClickData,
  event: ClickEvent,
  clearCbs: () => void,
) => {
  const [dots, setDots] = useState<Array<ClickDot>>([]);

  const resetData = useCallback(() => {
    setDots([]);
  }, [setDots]);

  const clickEvent = useCallback((e: React.MouseEvent<HTMLImageElement>) => {
    const dom = e.currentTarget;
    const xy = getDomXY(dom);

    const mouseX = e.pageX || e.clientX;
    const mouseY = e.pageY || e.clientY;

    const domX = xy.domX;
    const domY = xy.domY;

    const xPos = mouseX - domX;
    const yPos = mouseY - domY;

    const xx = parseInt(xPos.toString());
    const yy = parseInt(yPos.toString());
    const date = new Date();
    const index = dots.length;
    setDots([...dots, { key: date.getTime(), index: index + 1, x: xx, y: yy }]);

    event.click && event.click(xx, yy);
    e.stopPropagation();
    e.preventDefault();
    return false;
  }, [dots, event]);

  const confirmEvent = useCallback((e: React.MouseEvent<HTMLButtonElement>) => {
    event.confirm && event.confirm(dots, () => {
      resetData();
    });
    e.stopPropagation();
    e.preventDefault();
    return false;
  }, [dots, event, resetData]);

  const getDots = useCallback(() => {
    return dots;
  }, [dots]);

  const clearData = useCallback(() => {
    resetData();
    clearCbs && clearCbs();
  }, [resetData, clearCbs]);

  const close = useCallback(() => {
    event.close && event.close();
    resetData();
  }, [event, resetData]);

  const refresh = useCallback(() => {
    event.refresh && event.refresh();
    resetData();
  }, [resetData]);

  const closeEvent = useCallback((e: React.MouseEvent) => {
    close();
    e.stopPropagation();
    e.preventDefault();
    return false;
  }, [close]);

  const refreshEvent = useCallback((e: React.MouseEvent) => {
    refresh();
    e.stopPropagation();
    e.preventDefault();
    return false;
  }, [refresh]);

  return {
    setDots,
    getDots,
    clickEvent,
    confirmEvent,
    closeEvent,
    refreshEvent,
    resetData,
    clearData,
    close,
    refresh,
  };
};
