import { useRef } from 'react';
import { Captcha, CaptchaProvider, useCaptcha } from '@aetherlib/g-captcha';

export default () => {
  const ref = useRef();
  const { verify, holder } = useCaptcha();

  const click = () => {
    ref.current?.verify();
  };

  return (
    <CaptchaProvider
      api={{ basePath: 'http://127.0.0.1:3321' }}
      locale={{ buttonText: '点击验证', userCancel: '你已取消本次验证' }}
      theme={{ primaryColor: '#1890ff', borderRadius: 12 }}
    >
      {holder}

      <Captcha
        onSuccess={(data) => console.log(data)}
        type="auto"
        ref={ref}
      >
        <button
          onClick={click}
          style={{
            border: 'none',
            color: '#fff',
            width: '100px',
            height: '50px',
            lineHeight: '50px',
            borderRadius: '5px',
            background: '#1890ff',
          }}
        >
          组件模式
        </button>
      </Captcha>

      <button
        onClick={async () => {
          try {
            const data = await verify({
              type: 'auto',
              config: { width: 320 },
            });
            console.log(data);
          } catch (e) {
            console.log(e);
          }
        }}
        style={{
          border: 'none',
          color: '#fff',
          width: '100px',
          height: '50px',
          borderRadius: '5px',
          marginLeft: '10px',
          lineHeight: '50px',
          background: '#111827',
        }}
      >
        Hook 模式
      </button>
    </CaptchaProvider>
  );
};
