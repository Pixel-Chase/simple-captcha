## @aetherlib/g-captcha

采用 go-captcha 的 React SDK 封装，提供组件模式与 Hook 模式两种接入方式。

> `peerDependencies` 为 `react >= 18` 与 `react-dom >= 18`，演示站与测试基于 React 19。

## 推荐用法

全局配置通过 `CaptchaProvider` 注入，单次验证由 `Captcha` 或 `useCaptcha().verify()` 触发。

```jsx
import { useRef } from 'react';
import { Captcha, CaptchaProvider, useCaptcha } from '@aetherlib/g-captcha';

export default () => {
  const ref = useRef();
  const { verify, holder } = useCaptcha();

  return (
    <CaptchaProvider
      api={{ basePath: 'http://127.0.0.1:3321' }}
      locale={{ buttonText: '点击验证' }}
      theme={{ primaryColor: '#111827', borderRadius: 12 }}
    >
      {holder}

      <Captcha ref={ref} type="auto" onSuccess={(data) => console.log(data)}>
        <button onClick={() => ref.current?.verify()}>组件模式</button>
      </Captcha>

      <button
        onClick={async () => {
          const data = await verify({ type: 'slide-text' });
          console.log(data);
        }}
      >
        Hook 模式
      </button>
    </CaptchaProvider>
  );
};
```

## CaptchaProvider

| 属性 | 说明 | 类型 | 默认值 |
|---|---|---|---|
| api | 全局接口配置 | `ApiConfig` | - |
| locale | 全局多语言文案 | `LocaleTexts` | - |
| theme | 全局主题变量 | `ThemeTokens` | - |
| zIndex | 弹层层级 | `number` | `9999` |
| telemetry | 行为/环境采集级别：`off` 不采集，`basic` 轨迹 + 基础环境，`full` 额外采集 WebGL 渲染器与指纹哈希 | `TelemetryLevel` | `full` |

`ApiConfig` 支持以下配置：

| 属性 | 说明 | 类型 | 默认值 |
|---|---|---|---|
| basePath | 验证码后端服务地址 | `string` | `''` |
| prefix | API 路径前缀，需与后端 `API_PREFIX` 一致 | `string` | `/api/v1` |

后端使用自定义前缀时，可配置为 `api={{ basePath: 'http://foo.com', prefix: '/captcha/api' }}`。使用 `/` 或空字符串可将请求直接挂载到 `/captchas`。

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
| type | 验证码类型，支持 `auto`、`click-text`、`click-shape`、`slide-text`、`slide-region`、`rotate`；`auto` 由服务端按客户端风险等级选择题型 | `CaptchaType` | `auto` |
| telemetry | 当前实例的采集级别覆盖 | `TelemetryLevel` | - |
| api | 当前实例的接口配置覆盖 | `ApiConfig` | - |
| locale | 当前实例的文案覆盖 | `LocaleTexts` | - |
| theme | 当前实例的主题覆盖 | `ThemeTokens` | - |
| config | 当前实例的验证码配置 | `CaptchaConfig` | - |
| className | 容器类名 | `string` | - |
| style | 容器样式 | `CSSProperties` | - |

## useCaptcha

`useCaptcha()` 不再接收初始化参数，返回：

| 字段 | 说明 | 类型 |
|---|---|---|
| verify | 按次触发验证码，可覆盖当前调用的 `type/config/api/locale/theme` 与回调 | `(options?: CaptchaVerifyOptions) => Promise<any>` |
| holder | 必须渲染在当前 React 树中的承载节点 | `ReactNode` |

```jsx
import { CaptchaProvider, useCaptcha } from '@aetherlib/g-captcha';

export default () => {
  const { verify, holder } = useCaptcha();

  return (
    <CaptchaProvider api={{ basePath: 'http://foo.com' }}>
      {holder}
      <button
        onClick={async () => {
          const data = await verify({
            type: 'auto',
            maxRetryCount: 5,
            locale: { userCancel: 'Canceled by user' },
            theme: { primaryColor: '#111827' },
          });
          console.log(data);
        }}
      >
        verify
      </button>
    </CaptchaProvider>
  );
};
```

## CaptchaConfig

| 属性 | 说明 | 类型 | 默认值 |
|---|---|---|---|
| width | 宽度 | `number` | - |
| height | 高度 | `number` | - |
| thumbWidth | 缩略图宽度 | `number` | - |
| thumbHeight | 缩略图高度 | `number` | - |
| verticalPadding | 垂直方向内边距 | `number` | - |
| horizontalPadding | 水平方向内边距 | `number` | - |
| showTheme | 是否显示主题外框 | `boolean` | - |
| title | 标题 | `string` | - |
| iconSize | 图标大小 | `number` | - |
| scope | 滑动和旋转生效范围 | `number` | - |
| buttonText | 按钮文字 | `string` | - |
| dotSize | 点选模式区域大小 | `number` | - |

## LocaleTexts

| 属性 | 说明 |
|---|---|
| clickTitle | 点选模式标题，默认 `请在下图依次点击` |
| slideTitle | 滑块/旋转模式标题，默认 `请拖动滑块完成拼图` |
| confirmButton | 点选确认按钮，默认 `确认` |
| buttonText | 触发按钮文案，默认 `点击按键进行验证` |
| userCancel | 用户取消提示，默认 `用户取消` |
| loading | 加载中文案，默认 `加载中...` |

## ThemeTokens

| 属性 | 说明 | 默认值 |
|---|---|---|
| primaryColor | 主题色 | `#347eff` |
| backgroundColor | 弹窗背景色 | `#ffffff` |
| textColor | 文本颜色 | `#333333` |
| borderColor | 边框颜色 | `#e2e2e2` |
| borderRadius | 圆角大小 | `4` |

## 二次验证（S2S）

前端完成验证后，`onSuccess(data)` 回调中的 `data` 会包含一个 `second_key`。你应该将此参数随表单提交到业务后台，再由业务后台向验证码服务端发起二次验证：

```bash
curl -X GET \
  https://您的验证码域名/api/v1/captchas/states/{second_key} \
  -H "Authorization: Bearer <API_KEY>"
```

如果返回 `{"valid": true}` 则代表验证通过，可以放行业务逻辑。

若后端配置了自定义 `API_PREFIX`，此 S2S 地址中的 `/api/v1` 也需替换为相同前缀。

## 本地开发

```bash
pnpm install
pnpm start             # 启动 Vite 演示页（playground/，直接引用 src 源码，热更新）
pnpm build             # tsdown 打包：dist/index.js (ESM) + dist/index.cjs (CJS) + 类型声明
pnpm typecheck         # tsc 类型检查
pnpm lint              # Biome lint + 格式检查（pnpm lint:fix 自动修复）
pnpm test              # Vitest
```

演示页默认请求 `http://127.0.0.1:3321` 的后端服务，请先在仓库 `backend/` 目录启动服务。
