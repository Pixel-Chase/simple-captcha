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
| telemetry | 行为/环境采集级别：`off` 不采集，`basic` 轨迹 + 基础环境，`full` 额外采集 WebGL 渲染器与指纹哈希 | `TelemetryLevel` | `full` |

`ApiConfig.basePath` 用于指定验证码后端地址；`ApiConfig.prefix` 用于指定 API 路径前缀，默认 `/api/v1`，需与后端的 `API_PREFIX` 保持一致。

SDK 会自动完成以下工作，无需业务侧干预：

1. 按 `devicePixelRatio` 请求 1x/2x/3x 高清题图，并把点击/拖动坐标换算回位图像素再提交
2. 在题目加载后于后台计算工作量证明（PoW），提交时一并携带
3. 按 `telemetry` 级别采集指针轨迹与环境信号，供服务端做人机判别

## Captcha

| 属性 | 说明 | 类型 | 默认值 |
|---|---|---|---|
| onFail | 校验失败回调 | `(msg: string) => void` | - |
| onSuccess | 校验成功回调 | `(data: any) => void` | - |
| onCancel | 用户主动关闭回调 | `() => void` | - |
| maxRetryCount | 单次验证最多允许的校验失败次数，达到后才触发 `onFail` | `number` | `5` |
| type | 验证码类型；`auto` 由服务端按客户端风险等级选择题型 | `CaptchaType` | `auto` |
| telemetry | 当前实例的采集级别覆盖 | `TelemetryLevel` | - |
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
