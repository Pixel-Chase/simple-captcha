import { createContext, useCallback, useContext, useEffect, useRef, useState } from 'react';
import type { ReactNode } from 'react';
import { createPortal } from 'react-dom';
import { Icon } from './Icon';

type ToastTone = 'success' | 'error' | 'info';

interface ToastItem {
  id: number;
  message: string;
  tone: ToastTone;
}

type ShowToast = (message: string, tone?: ToastTone) => void;

const TOAST_DURATION = 2600;

const ToastContext = createContext<ShowToast>(() => undefined);

/** 右下角轻提示：复制、保存成功等不打断操作的反馈 */
export function ToastProvider({ children }: { children: ReactNode }) {
  const [items, setItems] = useState<ToastItem[]>([]);
  const seq = useRef(0);
  const timers = useRef(new Set<ReturnType<typeof setTimeout>>());

  useEffect(() => {
    const pending = timers.current;
    return () => {
      for (const timer of pending) {
        clearTimeout(timer);
      }
    };
  }, []);

  const show = useCallback<ShowToast>((message, tone = 'success') => {
    seq.current += 1;
    const id = seq.current;
    setItems((prev) => [...prev.slice(-2), { id, message, tone }]);
    const timer = setTimeout(() => {
      timers.current.delete(timer);
      setItems((prev) => prev.filter((item) => item.id !== id));
    }, TOAST_DURATION);
    timers.current.add(timer);
  }, []);

  return (
    <ToastContext.Provider value={show}>
      {children}
      {createPortal(
        <div className="toast-stack" role="status" aria-live="polite">
          {items.map((item) => (
            <div key={item.id} className={`toast toast--${item.tone}`}>
              <Icon
                name={
                  item.tone === 'error' ? 'error' : item.tone === 'info' ? 'info' : 'check-circle'
                }
                size={16}
              />
              <span>{item.message}</span>
            </div>
          ))}
        </div>,
        document.body,
      )}
    </ToastContext.Provider>
  );
}

export const useToast = () => useContext(ToastContext);

/**
 * 复制到剪贴板并弹出提示。非安全上下文（例如通过内网 IP 以 http 访问）没有 clipboard API，
 * 此时提示手动复制
 */
export function useCopy() {
  const toast = useToast();
  return useCallback(
    async (text: string, done = '已复制') => {
      try {
        await navigator.clipboard.writeText(text);
        toast(done);
        return true;
      } catch {
        toast('无法访问剪贴板，请手动选择复制', 'error');
        return false;
      }
    },
    [toast],
  );
}
