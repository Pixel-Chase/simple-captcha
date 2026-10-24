import { useRef } from 'react';
import type { CSSProperties } from 'react';
import { Captcha, CaptchaProvider, useCaptcha } from '@aetherlib/g-captcha';
import type { CaptchaRef } from '@aetherlib/g-captcha';

const buttonStyle: CSSProperties = {
  border: 'none',
  color: '#fff',
  width: 100,
  height: 50,
  lineHeight: '50px',
  borderRadius: 5,
  cursor: 'pointer',
};

export default function App() {
  const ref = useRef<CaptchaRef>(null);
  const { verify, holder } = useCaptcha();

  return (
    <CaptchaProvider
      api={{ basePath: 'http://127.0.0.1:3321' }}
      locale={{ buttonText: '点击验证', userCancel: '你已取消本次验证' }}
      theme={{ primaryColor: '#1890ff', borderRadius: 12 }}
    >
      {holder}

      <Captcha ref={ref} type="auto" onSuccess={(data) => console.log(data)}>
        <button
          type="button"
          style={{ ...buttonStyle, background: '#1890ff' }}
          onClick={() => ref.current?.verify()}
        >
          组件模式
        </button>
      </Captcha>

      <button
        type="button"
        style={{ ...buttonStyle, marginLeft: 10, background: '#111827' }}
        onClick={async () => {
          try {
            const data = await verify({ type: 'auto', config: { width: 320 } });
            console.log(data);
          } catch (e) {
            console.log(e);
          }
        }}
      >
        Hook 模式
      </button>
    </CaptchaProvider>
  );
}
