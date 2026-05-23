import { memo, useEffect, useState } from 'react';
import type { ButtonConfig, ButtonType } from './button.types';
import { defaultButtonConfig } from './button.types';
import { BtnDefaultIcon, BtnErrorIcon, BtnWarnIcon, BtnSuccessIcon } from '../icons';
import { useCaptchaConfig } from '../../CaptchaProvider';

export interface ButtonProps extends React.HTMLAttributes<HTMLElement> {
  config?: ButtonConfig;
  clickEvent?: () => void;
  disabled?: boolean;
  type?: ButtonType;
  title?: string;
}

const Button = (props: ButtonProps) => {
  const { locale } = useCaptchaConfig();
  const [localConfig, setLocalConfig] = useState<ButtonConfig>({ ...defaultButtonConfig(), ...(props.config || {}) });

  useEffect(() => {
    setLocalConfig((prev) => ({ ...prev, ...(props.config || {}) }));
  }, [props.config]);

  const type = props.type || 'default';

  let btnIcon = <BtnDefaultIcon />;
  let cn = `gc-btn-default`;
  if (type === 'warn') {
    btnIcon = <BtnWarnIcon />;
    cn = `gc-btn-warn`;
  } else if (type === 'error') {
    btnIcon = <BtnErrorIcon />;
    cn = `gc-btn-error`;
  } else if (type === 'success') {
    btnIcon = <BtnSuccessIcon />;
    cn = `gc-btn-success`;
  }

  return (
    <div
      className={`gc-btn ${cn} ${props.disabled ? `gc-btn-disabled` : ''}`}
      style={{
        width: localConfig.width + 'px',
        height: localConfig.height + 'px',
        paddingLeft: localConfig.verticalPadding + 'px',
        paddingRight: localConfig.verticalPadding + 'px',
        paddingTop: localConfig.verticalPadding + 'px',
        paddingBottom: localConfig.verticalPadding + 'px',
      }}
      onClick={props.clickEvent}
    >
      {type === 'default' ? <div className="gc-btn-ripple">{btnIcon}</div> : btnIcon}
      <span>{props.title || locale.buttonText}</span>
    </div>
  );
};

Button.displayName = 'Button';
export default memo(Button);
