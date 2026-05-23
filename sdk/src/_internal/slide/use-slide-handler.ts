/* eslint-disable @typescript-eslint/no-explicit-any */
import { type RefObject, useCallback, useEffect, useState } from 'react';
import type { SlideData, SlideEvent, SlideConfig } from './slide.types';
import { checkTargetFather } from '../helpers';

export const useSlideHandler = (
  data: SlideData,
  event: SlideEvent,
  config: SlideConfig,
  rootRef: RefObject<any>,
  containerRef: RefObject<any>,
  tileRef: RefObject<any>,
  dragBlockRef: RefObject<any>,
  dragBarRef: RefObject<any>,
  clearCbs: () => void,
) => {
  const [dragLeft, setDragLeft] = useState<number>(0);
  const [thumbLeft, setThumbLeft] = useState<number>(data.thumbX || 0);
  const [isFreeze, setIsFreeze] = useState<boolean>(false);

  useEffect(() => {
    if (!isFreeze) {
      setThumbLeft(data.thumbX || 0);
    }
  }, [data, setThumbLeft]);

  const resetData = useCallback(() => {
    setDragLeft(0);
    setThumbLeft(data.thumbX || 0);
  }, [setDragLeft, setThumbLeft, data.thumbX]);

  const dragEvent = useCallback((e: Event | any) => {
    if (!checkTargetFather(dragBarRef.current, e)) {
      return;
    }

    const touch = e.touches && e.touches[0];
    const offsetLeft = dragBlockRef.current.offsetLeft;
    const width = containerRef.current.offsetWidth;
    const blockWidth = dragBlockRef.current.offsetWidth;
    const maxWidth = width - blockWidth;

    const tileWith = tileRef.current.offsetWidth;
    const tileOffsetLeft = tileRef.current.offsetLeft;
    const containerMaxWidth = width - tileWith;
    const tileMaxWith = width - (tileWith + tileOffsetLeft);
    const ratio = tileMaxWith / maxWidth;

    let isMoving = false;
    let tmpLeaveDragEvent: Event | any = null;
    let startX = 0;
    let currentThumbX = 0;
    if (touch) {
      startX = touch.pageX - offsetLeft;
    } else {
      startX = e.clientX - offsetLeft;
    }

    const moveEvent = (e: Event | any) => {
      isMoving = true;
      const mTouche = e.touches && e.touches[0];

      let left = 0;
      if (mTouche) {
        left = mTouche.pageX - startX;
      } else {
        left = e.clientX - startX;
      }

      const ctX = tileOffsetLeft + (left * ratio);
      if (left >= maxWidth) {
        setDragLeft(maxWidth);
        currentThumbX = containerMaxWidth;
        setThumbLeft(currentThumbX);
        return;
      }

      if (left <= 0) {
        setDragLeft(0);
        currentThumbX = tileOffsetLeft;
        setThumbLeft(currentThumbX);
        return;
      }

      setDragLeft(left);
      currentThumbX = ctX;
      setThumbLeft(currentThumbX);

      event.move && event.move(currentThumbX, data.thumbY || 0);

      e.cancelBubble = true;
      e.preventDefault();
    };

    const upEvent = (e: Event | any) => {
      if (!checkTargetFather(dragBarRef.current, e)) {
        return;
      }

      clearEvent();

      if (!isMoving) {
        return;
      }
      isMoving = false;

      if (currentThumbX < 0) {
        return;
      }

      event.confirm && event.confirm({ x: parseInt(currentThumbX.toString()), y: data.thumbY || 0 }, () => {
        resetData();
      });

      e.cancelBubble = true;
      e.preventDefault();
    };

    const leaveDragBlockEvent = (e: Event | any) => {
      tmpLeaveDragEvent = e;
    };

    const enterDragBlockEvent = () => {
      tmpLeaveDragEvent = null;
    };

    const leaveUpEvent = (_: Event | any) => {
      if (!tmpLeaveDragEvent) {
        return;
      }

      upEvent(tmpLeaveDragEvent);
      clearEvent();
    };

    const scope = config.scope;
    const dragDom = scope ? rootRef.current : dragBarRef.current;
    const scopeDom = scope ? rootRef.current : document.body;

    const clearEvent = () => {
      scopeDom.removeEventListener('mousemove', moveEvent, false);
      scopeDom.removeEventListener('touchmove', moveEvent, { passive: false } as EventListenerOptions);

      dragDom.removeEventListener('mouseup', upEvent, false);
      dragDom.removeEventListener('mouseenter', enterDragBlockEvent, false);
      dragDom.removeEventListener('mouseleave', leaveDragBlockEvent, false);
      dragDom.removeEventListener('touchend', upEvent, false);

      scopeDom.removeEventListener('mouseleave', upEvent, false);
      scopeDom.removeEventListener('mouseup', leaveUpEvent, false);

      setIsFreeze(false);
    };
    setIsFreeze(true);

    scopeDom.addEventListener('mousemove', moveEvent, false);
    scopeDom.addEventListener('touchmove', moveEvent, { passive: false });

    dragDom.addEventListener('mouseup', upEvent, false);
    dragDom.addEventListener('mouseenter', enterDragBlockEvent, false);
    dragDom.addEventListener('mouseleave', leaveDragBlockEvent, false);
    dragDom.addEventListener('touchend', upEvent, false);

    scopeDom.addEventListener('mouseleave', upEvent, false);
    scopeDom.addEventListener('mouseup', leaveUpEvent, false);
  }, [rootRef, dragBlockRef, containerRef, config, data, tileRef, dragBarRef, event, resetData]);

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
  }, [event, resetData]);

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

  const getPoint = useCallback(() => {
    return {
      x: thumbLeft,
      y: data.thumbY || 0,
    };
  }, [data, thumbLeft]);

  const getState = useCallback(() => {
    return {
      dragLeft,
      thumbLeft,
    };
  }, [thumbLeft, dragLeft]);

  return {
    getState,
    getPoint,
    dragEvent,
    closeEvent,
    refreshEvent,
    resetData,
    clearData,
    close,
    refresh,
  };
};
