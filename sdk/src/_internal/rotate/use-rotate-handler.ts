/* eslint-disable @typescript-eslint/no-explicit-any */
import { type RefObject, useCallback, useEffect, useState } from 'react';
import type { RotateData, RotateEvent, RotateConfig } from './rotate.types';
import { checkTargetFather } from '../helpers';

export const useRotateHandler = (
  data: RotateData,
  event: RotateEvent,
  config: RotateConfig,
  rootRef: RefObject<any>,
  dragBlockRef: RefObject<any>,
  dragBarRef: RefObject<any>,
  clearCbs: () => void,
) => {
  const [dragLeft, setDragLeft] = useState<number>(0);
  const [thumbAngle, setThumbAngle] = useState<number>(data.angle || 0);
  const [isFreeze, setIsFreeze] = useState<boolean>(false);

  useEffect(() => {
    if (!isFreeze) {
      setThumbAngle(data.angle || 0);
    }
  }, [data, setThumbAngle]);

  const resetData = useCallback(() => {
    setDragLeft(0);
    setThumbAngle(data.angle || 0);
  }, [data.angle, setDragLeft, setThumbAngle]);

  const dragEvent = useCallback((e: Event | any) => {
    if (!checkTargetFather(dragBarRef.current, e)) {
      return;
    }

    const touch = e.touches && e.touches[0];

    const offsetLeft = dragBlockRef.current.offsetLeft;
    const width = dragBarRef.current.offsetWidth;
    const blockWidth = dragBlockRef.current.offsetWidth;
    const maxWidth = width - blockWidth;
    const maxAngle = 360;
    const p = (maxAngle - data.angle! || 0) / maxWidth;

    let angle = 0;
    let isMoving = false;
    let tmpLeaveDragEvent: Event | any = null;
    let startX = 0;
    let currentAngle = 0;
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

      angle = (data.angle! || 0) + (left * p);

      if (left >= maxWidth) {
        setDragLeft(maxWidth);
        currentAngle = maxAngle;
        setThumbAngle(currentAngle);
        return;
      }

      if (left <= 0) {
        setDragLeft(0);
        currentAngle = data.angle || 0;
        setThumbAngle(currentAngle);
        return;
      }

      setDragLeft(left);
      currentAngle = angle;
      setThumbAngle(angle);

      event.rotate && event.rotate(angle);

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

      if (currentAngle < 0) {
        return;
      }

      event.confirm && event.confirm(parseInt(currentAngle.toString()), () => {
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
  }, [rootRef, dragBlockRef, dragBarRef, config, data, event, resetData]);

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

  const getState = useCallback(() => {
    return {
      dragLeft,
      thumbAngle,
    };
  }, [thumbAngle, dragLeft]);

  return {
    getState,
    thumbAngle,
    dragEvent,
    closeEvent,
    refreshEvent,
    resetData,
    clearData,
    close,
    refresh,
  };
};
