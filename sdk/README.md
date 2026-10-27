## @aetherlib/g-captcha

采用 go-captcha 的 React SDK 封装，提供组件模式与 Hook 模式两种接入方式。

> `peerDependencies` 为 `react >= 18` 与 `react-dom >= 18`，演示站与测试基于 React 19。

## 推荐用法

全局配置通过 `CaptchaProvider` 注入，单次验证由 `Captcha` 或 `useCaptcha().verify()` 触发。

```jsx
import { useRef } from 'react';
import { Captcha, CaptchaProvider, useCaptcha } from '@aetherlib/g-captcha';

export default () => {
  const ref = useRef(null);
  const { verify, holder } = useCaptcha();

  return (
    <CaptchaProvider
      api={{ baseURL: 'https://captcha.example.com', appId: 'app_xxxxxxxxxxxx' }}
      theme={{ primaryColor: '#111827', borderRadius: 12 }}
    >
      {holder}

      <Captcha ref={ref} type="auto" onSuccess={(data) => console.log(data)}>
        <button onClick={() => ref.current?.open()}>组件模式</button>
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

`Captcha` 与 `useCaptcha()` 统一从最近的 `CaptchaProvider` 读取以上配置；局部需要不同配置时嵌套一层 `CaptchaProvider`，未指定的字段沿用外层。

`ApiConfig` 支持以下配置：

| 属性 | 说明 | 类型 | 默认值 |
|---|---|---|---|
| baseURL | 验证码服务的 origin，例如 `https://captcha.example.com`；只能是 origin，不带路径，不传表示与页面同源 | `string` | `''` |
| appId | 验证码服务管理后台「接入应用」里的应用 ID，必填；当前页面的 origin 必须在该应用的允许来源里 | `string` | - |

API 路径固定为 `/api/v1`。缺少 `appId` 或 `baseURL` 带路径时，出题直接以 `LOAD_FAILED` 失败；服务端拒绝本应用（应用不存在或已停用、来源不在允许名单内）时以 `APP_REJECTED` 通知。

**从 2.x 升级**：`api={{ basePath, prefix }}` 改为 `api={{ baseURL, appId }}`，并先在管理后台创建应用、填写允许来源。

SDK 会自动完成以下工作，无需业务侧干预：

1. 按 `devicePixelRatio` 请求 1x/2x/3x 高清题图，并把点击/拖动坐标换算回位图像素再提交
2. 在题目加载后于后台计算工作量证明（PoW），提交时一并携带
3. 按 `telemetry` 级别采集指针轨迹与环境信号，供服务端做人机判别

## Captcha

| 属性 | 说明 | 类型 | 默认值 |
|---|---|---|---|
| type | 验证码类型，支持 `auto`、`click-text`、`click-shape`、`slide-text`、`slide-region`、`rotate`；`auto` 由服务端按客户端风险等级选择题型 | `CaptchaType` | `auto` |
| maxAttempts | 连续答错的次数上限，达到后关闭弹窗并以 `MAX_ATTEMPTS` 触发 `onFail` | `number` | `5` |
| width | 题图渲染宽度（CSS 像素），高度按 3:2 推算 | `number` | `300` |
| onSuccess | 校验成功回调，`data.second_key` 用于二次验证 | `(data: CaptchaVerifySuccessData) => void` | - |
| onFail | 题目加载失败（`LOAD_FAILED`）、答错次数用完（`MAX_ATTEMPTS`）或服务端拒绝本应用（`APP_REJECTED`）时触发 | `(error: CaptchaError) => void` | - |
| onCancel | 用户关闭弹窗时触发，此时不会再触发 `onFail` | `() => void` | - |
| children | 原样渲染，通常放触发按钮 | `ReactNode` | - |

通过 `ref.current.open()` 打开弹窗并加载题目，结果只经由上面三个回调通知。

## useCaptcha

`useCaptcha()` 不再接收初始化参数，返回：

| 字段 | 说明 | 类型 |
|---|---|---|
| verify | 按次触发验证码，`options` 与 `Captcha` 属性相同（不含 `children`）；成功时 resolve，取消或失败时以 `CaptchaError` reject | `(options?: CaptchaVerifyOptions) => Promise<CaptchaVerifySuccessData>` |
| holder | 必须渲染在当前 React 树中的承载节点 | `ReactNode` |

```jsx
import { CaptchaError, CaptchaProvider, useCaptcha } from '@aetherlib/g-captcha';

export default () => {
  const { verify, holder } = useCaptcha();

  return (
    <CaptchaProvider api={{ baseURL: 'https://captcha.example.com', appId: 'app_xxxxxxxxxxxx' }}>
      {holder}
      <button
        onClick={async () => {
          try {
            const data = await verify({ type: 'auto', maxAttempts: 3 });
            console.log(data.second_key);
          } catch (e) {
            if (e instanceof CaptchaError && e.code === 'CANCELLED') return;
            console.log(e.code, e.message);
          }
        }}
      >
        verify
      </button>
    </CaptchaProvider>
  );
};
```

## CaptchaError

验证没有成功时统一给出 `CaptchaError`（继承自 `Error`），按 `code` 区分原因：

| code | 触发场景 | 通知方式 |
|---|---|---|
| `CANCELLED` | 用户关闭弹窗 | 组件模式只触发 `onCancel`；Hook 模式同时以此 reject |
| `LOAD_FAILED` | 题目获取失败：网络异常、缺少 `appId`、`baseURL` 不是 origin，或服务端返回其他错误 | `onFail`，Hook 模式同时 reject |
| `MAX_ATTEMPTS` | 连续答错达到 `maxAttempts`，弹窗已关闭 | `onFail`，Hook 模式同时 reject |
| `ABORTED` | 上一次 `verify()` 被新的调用替换，或组件已卸载 | 仅 Hook 模式 reject |
| `APP_REJECTED` | 服务端拒绝了本应用：应用不存在或已停用（`1010`），或当前页面来源不在允许名单内（`1011`）；提交时出现不计入答错次数 | `onFail`，Hook 模式同时 reject |

`error.message` 是可直接展示的中文说明；`LOAD_FAILED`、`MAX_ATTEMPTS` 与 `APP_REJECTED` 还带有 `error.response`，即服务端返回的 `{ code, error }`。

## LocaleTexts

| 属性 | 说明 |
|---|---|
| clickTitle | 点选模式标题，默认 `安全点选验证` |
| slideTitle | 滑块拼图模式标题，默认 `拼图验证` |
| regionTitle | 滑动区域模式标题，默认 `空间拼图` |
| rotateTitle | 旋转模式标题，默认 `旋转验证` |
| clickSubtitle | 点选标题下的副标题，默认 `请依序点击图中的指定汉字` |
| slideSubtitle | 滑块拼图副标题，默认 `拖动滑块使拼图完全吻合` |
| regionSubtitle | 滑动区域副标题，默认 `自由拖动图块至缺口，松手提交` |
| rotateSubtitle | 旋转副标题，默认 `拖动滑块使图片角度摆正` |
| clickTag | 点选标题右侧的小标签，设为空字符串可隐藏，默认 `风控防护` |
| slideTag | 滑块拼图标题右侧的小标签，默认 `高精咬合` |
| regionTag | 滑动区域标题右侧的小标签，默认 `免滑条` |
| rotateTag | 旋转标题右侧的小标签，默认 `角度矫正` |
| confirmButton | 点选确认按钮，默认 `确认点选` |
| clickHint | 点选提示条文案，默认 `请依序点击：` |
| clickMeta | 点选计数，`{n}` 为已点击数量，默认 `已依序选择 {n} 处` |
| clickClear | 点选清除按钮，默认 `重选` |
| slideHint | 滑块轨道内的待拖动提示，默认 `向右拖动滑块完成拼图` |
| rotateHint | 旋转轨道内的待拖动提示，默认 `向右拖动滑块使图片摆正` |
| slideProgress | 滑块轨道下方左侧的说明，默认 `拖动至与缺口精准咬合` |
| slidePercent | 滑块轨道下方右侧的真实拖动进度，`{p}` 为百分比，默认 `当前进度 {p}%` |
| rotateProgress | 旋转轨道下方右侧的说明（左侧显示当前角度），默认 `松手自动校准判定` |
| regionHint | 滑动区域图片下方的操作提示，默认 `按住图块自由拖拽入框` |
| regionBadge | 滑动区域操作提示右侧的角标，默认 `松手即验` |
| regionReset | 滑动区域把图块移回初始位置的按钮，默认 `原位重置` |
| rotateAngle | 旋转图片下方的角度标签前缀，默认 `当前偏移` |
| successText | 校验通过画面的文案，默认 `验证通过` |
| elapsedText | 校验通过画面的耗时，`{s}` 为秒数，默认 `耗时 {s}s` |
| doneText | 点选校验通过后按钮文案，默认 `已完成` |
| failTitle | 校验未通过时替换副标题，默认 `验证未通过，请重试` |
| failBanner | 校验未通过时图片上的提示条，默认 `正在重新生成验证码…` |
| trackSuccess | 校验通过时轨道内的文案，默认 `验证已通过` |
| trackFail | 校验未通过时轨道内的文案，默认 `验证未通过，请重新拖动` |
| regionNote | 滑动区域底部的安全提示，设为空字符串可隐藏，默认 `二维拖拽轨迹与运动特征分析` |
| slideNote | 滑块拼图底部的安全提示，设为空字符串可隐藏，默认 `多维速度曲线与轨迹时序分析` |
| rotateNote | 旋转底部的安全提示，设为空字符串可隐藏，默认 `拖动速度与微抖动行为分析` |
| refreshLabel | 刷新按钮的无障碍标签，默认 `刷新` |
| closeLabel | 关闭按钮的无障碍标签，默认 `关闭` |

## ThemeTokens

| 属性 | 说明 | 默认值 |
|---|---|---|
| primaryColor | 主题色 | `#3904e7` |
| backgroundColor | 弹窗背景色 | `#ffffff` |
| textColor | 文本颜色 | `#1b1a25` |
| borderColor | 边框颜色 | `#e3e8ee` |
| borderRadius | 圆角大小 | `24` |

## 二次验证（S2S）

前端完成验证后，`onSuccess(data)` 回调中的 `data` 会包含一个 `second_key`。你应该将此参数随表单提交到业务后台，再由业务后台向验证码服务端发起二次验证：

```bash
curl -X GET \
  https://您的验证码域名/api/v1/captchas/states/{second_key} \
  -H "Authorization: Bearer <应用密钥>"
```

应用密钥在验证码服务的管理后台「接入应用」中创建，与前端使用的 `appId` 属于同一个应用，只在业务后端使用。
只有返回 `{"valid": true}` 才代表验证通过，可以放行业务逻辑；`second_key` 只能用出题应用自己的密钥查询，且只能查询一次。

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
