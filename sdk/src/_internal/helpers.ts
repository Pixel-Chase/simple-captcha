/**
 * 获取 DOM 元素相对于文档的坐标
 */
export function getDomXY(dom: HTMLElement) {
  let x = 0;
  let y = 0;
  if (dom.getBoundingClientRect) {
    const box = dom.getBoundingClientRect();
    const D = document.documentElement;
    x = box.left + Math.max(D.scrollLeft, document.body.scrollLeft) - D.clientLeft;
    y = box.top + Math.max(D.scrollTop, document.body.scrollTop) - D.clientTop;
  } else {
    let el: HTMLElement | null = dom;
    while (el && el !== document.body) {
      x += el.offsetLeft;
      y += el.offsetTop;
      el = el.offsetParent as HTMLElement | null;
    }
  }
  return { domX: x, domY: y };
}

/**
 * 检查事件是否来自目标元素的子元素
 */
export function checkTargetFather(that: HTMLElement, e: MouseEvent | TouchEvent) {
  let parent = (e as MouseEvent).relatedTarget as HTMLElement | null;
  try {
    while (parent && parent !== that) {
      parent = parent.parentNode as HTMLElement | null;
    }
  } catch (err) {
    console.warn(err);
  }
  return parent !== that;
}
