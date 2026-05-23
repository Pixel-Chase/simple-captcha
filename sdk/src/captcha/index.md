## @aetherlib/g-captcha

当前版本采用新的配置模型：

1. `CaptchaProvider` 负责全局 `api / locale / theme / zIndex`
2. `Captcha` 负责单个实例渲染与局部覆盖
3. `useCaptcha()` 负责按次触发验证，并返回 `holder`

## CaptchaProvider

| 属性 | 说明 | 类型 | 默认值 |
|---|---|---|---|
| api | 全局接口配置 | `ApiConfig` | - |
| locale | 全局多语言文案 | `LocaleTexts` | - |
| theme | 全局主题变量 | `ThemeTokens` | - |
| zIndex | 弹层层级 | `number` | `9999` |

## Captcha

| 属性 | 说明 | 类型 | 默认值 |
|---|---|---|---|
| onFail | 校验失败回调 | `(msg: string) => void` | - |
| onSuccess | 校验成功回调 | `(data: any) => void` | - |
| onCancel | 用户主动关闭回调 | `() => void` | - |
| type | 验证码类型 | `CaptchaType` | `auto` |
| api | 当前实例的接口配置覆盖 | `ApiConfig` | - |
| locale | 当前实例的文案覆盖 | `LocaleTexts` | - |
| theme | 当前实例的主题覆盖 | `ThemeTokens` | - |
| config | 当前实例的验证码配置 | `CaptchaConfig` | - |
| className | 容器类名 | `string` | - |
| style | 容器样式 | `CSSProperties` | - |

## useCaptcha

| 字段 | 说明 | 类型 |
|---|---|---|
| verify | 按次触发验证码，并支持局部覆盖配置 | `(options?: CaptchaVerifyOptions) => Promise<any>` |
| holder | 需要挂在当前 React 树中的承载节点 | `ReactNode` |

<code src='./demo/index.jsx'></code>
