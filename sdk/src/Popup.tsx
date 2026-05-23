import type { BaseTypeProps } from './types';
import type { ReactNode } from 'react';
import { useCaptchaConfig } from './CaptchaProvider';
export interface PopupProps extends BaseTypeProps {
  visible?: boolean;
  children?: ReactNode;
}

const Popup = ({ className, style, visible = false, children }: PopupProps) => {
  const { zIndex } = useCaptchaConfig();

  if (!visible) return null;

  return (
    <div 
      className={className} 
      style={{
        position: 'fixed',
        left: 0,
        top: 0,
        width: '100vw',
        height: '100vh',
        zIndex,
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        ...style
      }}
    >
      <div 
        style={{
          position: 'absolute',
          left: 0,
          top: 0,
          width: '100%',
          height: '100%',
          backgroundColor: 'rgba(0, 0, 0, 0.5)'
        }}
      />
      <div style={{ position: 'relative', zIndex: 1, backgroundColor: 'transparent' }}>
        {children}
      </div>
    </div>
  );
};

export default Popup;
