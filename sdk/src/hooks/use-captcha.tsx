import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import type { ReactNode } from 'react';
import { CaptchaInternal } from '../Captcha';
import { CaptchaError } from '../CaptchaError';
import type { CaptchaRef, CaptchaVerifyOptions, CaptchaVerifySuccessData } from '../Captcha.types';

type PendingRequest = {
  resolve: (value: CaptchaVerifySuccessData) => void;
  reject: (reason: CaptchaError) => void;
};

export interface UseCaptchaReturn {
  verify: (options?: CaptchaVerifyOptions) => Promise<CaptchaVerifySuccessData>;
  holder: ReactNode;
}

export function useCaptcha(): UseCaptchaReturn {
  const captchaRef = useRef<CaptchaRef | null>(null);
  const pendingRef = useRef<PendingRequest | null>(null);
  const [activeOptions, setActiveOptions] = useState<CaptchaVerifyOptions | null>(null);
  const [requestId, setRequestId] = useState(0);

  // 结果先交给调用方，holder 留到弹窗自己关闭（onClosed）后再卸载，
  // 这样校验通过 / 未通过的结果画面才有机会展示
  const handleSuccess = useCallback(
    (data: CaptchaVerifySuccessData) => {
      activeOptions?.onSuccess?.(data);
      pendingRef.current?.resolve(data);
      pendingRef.current = null;
    },
    [activeOptions],
  );

  const handleFail = useCallback(
    (error: CaptchaError) => {
      activeOptions?.onFail?.(error);
      pendingRef.current?.reject(error);
      pendingRef.current = null;
    },
    [activeOptions],
  );

  const handleClosed = useCallback(() => {
    setActiveOptions(null);
  }, []);

  const handleCancel = useCallback(() => {
    activeOptions?.onCancel?.();
    pendingRef.current?.reject(new CaptchaError('CANCELLED', '用户取消'));
    pendingRef.current = null;
  }, [activeOptions]);

  const verify = useCallback((options?: CaptchaVerifyOptions) => {
    if (pendingRef.current) {
      pendingRef.current.reject(new CaptchaError('ABORTED', '验证码流程已被新的请求替换'));
    }

    return new Promise<CaptchaVerifySuccessData>((resolve, reject) => {
      pendingRef.current = { resolve, reject };
      setActiveOptions(options || {});
      setRequestId((value) => value + 1);
    });
  }, []);

  useEffect(() => {
    if (requestId === 0 || !activeOptions) {
      return;
    }

    // 加载失败经由 onFail（handleFail）结束本次 Promise，open 本身不会 reject
    void captchaRef.current?.open();
  }, [activeOptions, requestId]);

  useEffect(() => {
    return () => {
      if (pendingRef.current) {
        pendingRef.current.reject(new CaptchaError('ABORTED', '验证码实例已卸载'));
      }
    };
  }, []);

  const holder = useMemo(() => {
    if (!activeOptions) {
      return null;
    }

    const {
      onCancel: _onCancel,
      onFail: _onFail,
      onSuccess: _onSuccess,
      ...captchaProps
    } = activeOptions;

    return (
      <CaptchaInternal
        {...captchaProps}
        onCancel={handleCancel}
        onFail={handleFail}
        onSuccess={handleSuccess}
        onClosed={handleClosed}
        ref={captchaRef}
      />
    );
  }, [activeOptions, handleCancel, handleClosed, handleFail, handleSuccess]);

  return {
    verify,
    holder,
  };
}
