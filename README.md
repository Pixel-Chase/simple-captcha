# Simple Captcha

开箱即用的行为验证码：Go 后端服务 + React 前端 SDK，支持 **文字点选**、**图形点选**、**滑块拼图**、**滑块区域**、**旋转验证** 五种模式。后端以 Docker 镜像部署，前端通过 npm 包接入。

## 目录

- [架构概览](#架构概览)
- [验证流程](#验证流程)
- [后端部署](#后端部署)
- [前端接入](#前端接入)
- [API 参考](#api-参考)
- [安全机制](#安全机制)
- [二次验证（S2S）](#二次验证s2s)
- [管理后台](#管理后台)
- [环境变量参考](#环境变量参考)
- [本地开发](#本地开发)

## 架构概览

```
┌──────────────────┐         ┌──────────────────┐         ┌───────────┐
│   前端应用        │◄───────►│  Simple Captcha  │◄───────►│   Redis   │
│  @aetherlib/     │  HTTP   │  Backend (Go)    │         │  (外部)    │
│  g-captcha       │         │  :3321           │         │           │
└──────────────────┘         └──────────────────┘         └───────────┘
                                      ▲
                                      │ Authorization: Bearer <应用密钥>
                              ┌───────┴────────┐
                              │  业务后端 (S2S)  │
                              │  二次验证查询     │
                              └────────────────┘
```

## 验证流程

```
用户操作                    前端 SDK                     验证码后端                    业务后端

  │  点击验证                  │                            │                           │
  ├────────────────────────►│                            │                           │
  │                          │  GET /captchas/:type       │                           │
  │                          ├───────────────────────────►│                           │
  │                          │  ◄── 返回图片+key           │                           │
  │  ◄── 显示验证码弹窗       │                            │                           │
  │                          │                            │                           │
  │  完成验证操作              │                            │                           │
  ├────────────────────────►│                            │                           │
  │                          │  POST /captchas/:type/     │                           │
  │                          │       :key/attempts        │                           │
  │                          ├───────────────────────────►│                           │
  │                          │  ◄── 返回 second_key       │                           │
  │  ◄── onSuccess(data)     │                            │                           │
  │                          │                            │                           │
  │  提交表单(含 second_key)   │                            │                           │
  ├─────────────────────────────────────────────────────────────────────────────────►│
  │                          │                            │  GET /captchas/states/    │
  │                          │                            │      :key                │
  │                          │                            │  ◄────────────────────────┤
  │                          │                            │  ── { valid: true }      │
  │                          │                            │  ────────────────────────►│
  │  ◄── 业务处理结果          │                            │                           │
```

## 后端部署

### 快速启动

需要 Docker Compose 与一个外部 Redis（6.0+）。

**1. 准备 `.env`**

最少只填 Redis，完整模板见 [`backend/.env.example`](backend/.env.example)。应用密钥与管理员密码都不走配置。

```dotenv
REDIS_HOST=your-redis-host
REDIS_PORT=6379
REDIS_PASSWORD=your-redis-password
REDIS_DB=2

# 允许的前端来源，逗号分隔；其余可选项见「环境变量参考」
CORS_ALLOW_ORIGINS=https://example.com,https://admin.example.com
```

> 含 `$` 的值必须用单引号包裹，否则会被当作变量展开。

**2. 创建 `docker-compose.yml`**

```yaml
services:
  captcha:
    image: ghcr.io/pixel-chase/simple-captcha:latest
    container_name: simple-captcha
    ports:
      - "3321:3321"
    env_file:
      - .env
    volumes:
      # 管理员凭据与接入应用，必须持久化：丢失后管理员密码重新生成，全部应用密钥失效
      - captcha-data:/app/data
      # 背景图素材，管理后台直接读写；新卷首次挂载时带上镜像内置素材
      - captcha-images:/app/images
    restart: unless-stopped
    healthcheck:
      test: ["CMD", "wget", "--no-verbose", "--tries=1", "--spider", "http://localhost:3321/health"]
      interval: 30s
      timeout: 10s
      retries: 3

volumes:
  captcha-data:
  captcha-images:
```

**3. 启动并检查**

```bash
docker compose up -d
curl http://localhost:3321/health   # {"status":"ok","redis":{"status":"ok"}, ...}
```

**4. 登录管理后台，创建接入应用**

管理员初始密码在首次启动时随机生成，只在日志里打印一次：

```bash
docker compose logs captcha | grep -A6 管理后台初始密码
```

用它登录 `http://<服务地址>:3321/console/` 并改密，再在「接入应用」里为每个业务后端创建应用、配置密钥。创建第一个应用之前，S2S 状态查询一律返回 401。详见 [管理后台](#管理后台)。

### 镜像发布

镜像由 GitHub Actions 构建并发布到 `ghcr.io/pixel-chase/simple-captcha`（`linux/amd64` / `linux/arm64`）：推送默认分支生成 `latest`、分支名与 `sha-*` 标签，推送 `v*` 标签生成语义化版本并更新 `latest`。私有 Package 拉取前需先 `docker login ghcr.io`。

本地构建以仓库根目录为上下文（镜像内先构建管理后台前端，再内嵌进 Go 二进制）：`docker build -f backend/Dockerfile .`

### RESTful API 路由

鉴权均为 `Authorization: Bearer <…>`：S2S 查询用应用密钥，`/admin/*` 用登录返回的会话令牌。

| 方法 | 路径 | 说明 | 鉴权 |
|------|------|------|------|
| `GET` | `/api/v1/captchas/:type` | 获取验证码 | — |
| `POST` | `/api/v1/captchas/:type/:key/attempts` | 提交答案 | — |
| `GET` | `/api/v1/captchas/states/:key` | 查询验证状态（S2S） | 应用密钥 |
| `GET` | `/health` | 健康检查 | — |
| `POST` | `/api/v1/admin/login` | 管理员登录（需携带 `second_key`，叠加 IP 限流） | — |
| `GET` / `DELETE` | `/api/v1/admin/session` | 查询 / 注销会话 | 会话令牌 |
| `PUT` | `/api/v1/admin/password` | 修改密码（旧会话全部失效，返回新会话） | 会话令牌 |
| `GET` / `POST` | `/api/v1/admin/backgrounds` | 列出 / 上传背景图 | 会话令牌 |
| `GET` / `DELETE` | `/api/v1/admin/backgrounds/:name` | 预览 / 删除背景图 | 会话令牌 |
| `GET` / `POST` | `/api/v1/admin/apps` | 列出 / 新建接入应用 | 会话令牌 |
| `PUT` / `DELETE` | `/api/v1/admin/apps/:id` | 修改名称与启用状态 / 删除应用 | 会话令牌 |
| `POST` | `/api/v1/admin/apps/:id/secret` | 重置应用密钥 | 会话令牌 |

- 路径前缀由 `API_PREFIX` 决定（默认 `/api/v1`）：配置 `/captcha/api` 后为 `/captcha/api/captchas/:type`，配置 `/` 则挂在根路径。
- 仍在使用初始密码时，`/admin/*` 只开放会话与改密接口，其余返回 HTTP 403（`code: 2008`）。

**验证码类型 (`:type`)**

| 值 | 验证方式 |
|---|---|
| `click-text` | 文字点选 |
| `click-shape` | 图形点选 |
| `slide-text` | 滑块验证 |
| `slide-region` | 区域滑块 |
| `rotate` | 旋转验证 |
| `auto` | 服务端按客户端风险等级选题：低风险可能是滑块 / 旋转，高风险只给点选类 |

### 接口字段说明

**`GET /captchas/:type?mode=dark&dpr=2`**

| 查询参数 | 说明 |
|---|---|
| `mode` | `light` / `dark`，影响点选文字配色，可选 |
| `dpr` | 前端 `devicePixelRatio`（1-3），服务端按此倍率出图（至少 2 倍，但不超过 `CAPTCHA_MAX_SCALE`），可选 |

响应（HTTP 200）：

```json
{
  "key": "f3c1…（32 位随机十六进制）",
  "type": "slide-text",
  "image": "data:image/jpeg;base64,…",
  "thumb": "data:image/png;base64,…",
  "thumbWidth": 110, "thumbHeight": 110, "thumbX": 20, "thumbY": 60,
  "imageWidth": 600, "imageHeight": 400,
  "scale": 2,
  "difficulty": 3,
  "pow": { "nonce": "…", "difficulty": 14 }
}
```

- 尺寸与坐标均为位图像素。SDK 按 `imageWidth / 渲染宽度` 把坐标换算回位图空间再提交，服务端容差也按倍率放大。
- `pow` 为工作量证明挑战：客户端需找到 `solution` 使 `SHA-256(nonce + ":" + key + ":" + solution)` 的前导零位数 ≥ `difficulty`。

**`POST /captchas/:type/:key/attempts`**

```json
{
  "point": "123,45",
  "trace": { "points": [[x, y, t], …], "clicks": [[x, y, t]], "pointerType": "mouse", "startedAt": 820, "submittedAt": 2410, "downs": 1, "ups": 1, "moves": 96, "untrusted": 0 },
  "env":   { "ua": "…", "languages": ["zh-CN"], "screen": [1920, 1080], "viewport": [1440, 900], "dpr": 2, "webdriver": false, "automation": [], "fp": "…" },
  "pow":   { "nonce": "…", "solution": "48213" }
}
```

答案字段按题型取其一：点选 `dots`（`"x1,y1,x2,y2"`）、滑块 `point`（`"x,y"`）、旋转 `angle`。`trace` / `env` / `pow` 由 SDK 自动填充；旧版 SDK 不携带也能验证，但会计入风险分。

**业务错误码**（HTTP 200 或 4xx，响应体为 `{ "code": …, "error": "…" }`）

| code | 含义 |
|---|---|
| `400` | 参数错误 |
| `429` | 触发频率限制 |
| `1003` | 答案错误 |
| `1004` | 验证码不存在 / 已过期 / 已失效 |
| `1005` | 尝试次数过多 |
| `1006` | 出题到提交间隔过短（疑似脚本） |
| `1007` | 提交客户端与出题客户端不一致 |
| `1009` | 工作量证明缺失或无效 |

> 风控评分超过拦截阈值时，对外同样返回 `1003` 并消耗一次尝试机会；真实原因（内部码 `1008` 与评分理由）只记在服务端日志，避免成为脚本调参的探针。

---

## 前端接入

### 安装

```bash
pnpm add @aetherlib/g-captcha   # 或 npm install / yarn add
```

要求 React 与 React DOM ≥ 18。

### 使用

`CaptchaProvider` 提供全局的 `api / locale / theme / zIndex`，`Captcha` 渲染单个实例，`useCaptcha()` 按次触发验证并返回需要渲染的 `holder`：

```tsx
import { useRef } from 'react';
import { Captcha, CaptchaProvider, useCaptcha } from '@aetherlib/g-captcha';

function App() {
  const ref = useRef();
  const { verify, holder } = useCaptcha();

  return (
    <CaptchaProvider
      api={{ basePath: 'https://captcha.yourdomain.com' }}
      locale={{ buttonText: '点击验证' }}
      theme={{ primaryColor: '#111827', borderRadius: 12 }}
    >
      {holder}

      <Captcha
        type="auto"
        ref={ref}
        onSuccess={(data) => {
          console.log('验证成功', data);
          submitForm(data.second_key);
        }}
        onFail={(msg) => console.log('验证失败', msg)}
      >
        <button onClick={() => ref.current?.verify()}>
          组件模式
        </button>
      </Captcha>

      <button
        onClick={async () => {
          try {
            const data = await verify({ type: 'auto' });
            console.log('验证成功', data);
          } catch (e) {
            console.log('验证失败或取消', e);
          }
        }}
      >
        Hook 模式
      </button>
    </CaptchaProvider>
  );
}
```

---

## API 参考

### `<CaptchaProvider>` Props

| 属性 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `api` | `ApiConfig` | — | 全局接口配置，`basePath` 指定后端地址，`prefix` 指定 API 路径前缀 |
| `locale` | `LocaleTexts` | — | 全局文案覆盖 |
| `theme` | `ThemeTokens` | — | 全局主题配置 |
| `zIndex` | `number` | `9999` | 弹窗层级 |
| `telemetry` | `'off' \| 'basic' \| 'full'` | `'full'` | 行为/环境采集级别，见[安全机制](#安全机制) |

### `<Captcha>` Props

| 属性 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `type` | `CaptchaType` | `'auto'` | 验证码类型，`auto` 由服务端按风险等级选择题型（需要后端 ≥ 1.3） |
| `telemetry` | `TelemetryLevel` | — | 当前实例的采集级别覆盖 |
| `onSuccess` | `(data) => void` | — | 验证成功回调，`data` 包含 `second_key` |
| `onFail` | `(msg) => void` | — | 验证失败回调 |
| `onCancel` | `() => void` | — | 用户取消回调 |
| `api` | `ApiConfig` | — | 当前实例的接口配置覆盖 |
| `locale` | `LocaleTexts` | — | 当前实例的文案覆盖 |
| `theme` | `ThemeTokens` | — | 当前实例的主题覆盖 |
| `config` | `CaptchaConfig` | — | 当前实例的验证码配置 |
| `ref` | `Ref` | — | 通过 `ref.current.verify()` 触发验证 |

### `LocaleTexts` 文案配置

通过 `locale` 覆盖默认中文文案：

```tsx
<CaptchaProvider
  locale={{
    userCancel: 'User Cancelled',
    loading: 'Loading...',
    clickTitle: 'Please click in order',
    slideTitle: 'Drag the slider to complete',
    confirmButton: 'Confirm',
    buttonText: 'Click to verify',
  }}
>
  <App />
</CaptchaProvider>
```

`{n}`、`{p}`、`{s}` 为运行时替换的占位符；`*Tag`、`poweredBy`、`slideNote`、`rotateNote` 设为空字符串即隐藏。

| Key | 默认值 | 说明 |
|-----|--------|------|
| `userCancel` | `用户取消` | 用户取消时的提示 |
| `loading` | `加载中...` | 加载状态 |
| `clickTitle` | `安全点选验证` | 点选标题 |
| `slideTitle` | `拼图验证` | 滑块拼图标题 |
| `regionTitle` | `空间拼图` | 滑动区域标题 |
| `rotateTitle` | `旋转验证` | 旋转标题 |
| `clickSubtitle` | `请依序点击图中的指定汉字` | 点选副标题 |
| `slideSubtitle` | `拖动滑块使拼图完全吻合` | 滑块拼图副标题 |
| `regionSubtitle` | `自由拖动图块至缺口，松手提交` | 滑动区域副标题 |
| `rotateSubtitle` | `拖动滑块使图片角度摆正` | 旋转副标题 |
| `clickTag` | `风控防护` | 点选标题右侧的小标签 |
| `slideTag` | `高精咬合` | 滑块拼图标题右侧的小标签 |
| `regionTag` | `免滑条` | 滑动区域标题右侧的小标签 |
| `rotateTag` | `角度矫正` | 旋转标题右侧的小标签 |
| `confirmButton` | `确认点选` | 点选确认按钮 |
| `buttonText` | `点击按键进行验证` | 触发验证按钮的默认文案 |
| `clickHint` | `请依序点击：` | 点选提示条 |
| `clickMeta` | `已依序选择 {n} 处` | 点选计数，`{n}` 为已点击数量 |
| `clickClear` | `重选` | 点选清除按钮 |
| `slideHint` | `向右拖动滑块完成拼图` | 滑块轨道内的待拖动提示 |
| `rotateHint` | `向右拖动滑块使图片摆正` | 旋转轨道内的待拖动提示 |
| `slideProgress` | `拖动至与缺口精准咬合` | 滑块轨道下方左侧说明 |
| `slidePercent` | `当前进度 {p}%` | 滑块轨道下方右侧的拖动进度 |
| `rotateProgress` | `松手自动校准判定` | 旋转轨道下方右侧说明（左侧显示当前角度） |
| `regionHint` | `按住图块自由拖拽入框` | 滑动区域图片下方的操作提示 |
| `regionBadge` | `松手即验` | 滑动区域操作提示右侧的角标 |
| `regionReset` | `原位重置` | 滑动区域把图块移回原位的按钮 |
| `rotateAngle` | `当前偏移` | 旋转图片下方角度标签的前缀 |
| `successText` | `验证通过` | 校验通过画面的文案 |
| `elapsedText` | `耗时 {s}s` | 校验通过画面的耗时 |
| `doneText` | `已完成` | 点选校验通过后的按钮文案 |
| `failTitle` | `验证未通过，请重试` | 校验未通过时替换副标题 |
| `failBanner` | `正在重新生成验证码…` | 校验未通过时图片上的提示条 |
| `trackSuccess` | `验证已通过` | 校验通过时轨道内的文案 |
| `trackFail` | `验证未通过，请重新拖动` | 校验未通过时轨道内的文案 |
| `poweredBy` | `二维拖拽轨迹与运动特征分析` | 滑动区域底部的安全提示 |
| `slideNote` | `多维速度曲线与轨迹时序分析` | 滑块拼图底部的安全提示 |
| `rotateNote` | `拖动速度与微抖动行为分析` | 旋转底部的安全提示 |
| `refreshLabel` | `刷新` | 刷新按钮的无障碍标签 |
| `closeLabel` | `关闭` | 关闭按钮的无障碍标签 |

### `ThemeTokens` 主题配置

```tsx
<CaptchaProvider
  theme={{
    primaryColor: '#3904e7',
    backgroundColor: '#ffffff',
    textColor: '#1b1a25',
    borderColor: '#e3e8ee',
    borderRadius: 24,
  }}
>
  <App />
</CaptchaProvider>
```

### `useCaptcha()` 返回值

| 属性 | 类型 | 说明 |
|------|------|------|
| `verify` | `(options?: CaptchaVerifyOptions) => Promise<any>` | 按次触发验证，可覆盖 `type / api / locale / theme / config` |
| `holder` | `ReactNode` | 必须渲染在当前 React 树中的承载节点 |

---

## 安全机制

| 机制 | 说明 | 相关配置 |
|---|---|---|
| 随机 key | 验证码 key 为 128 位加密随机数，与答案无关，无法离线反推 | — |
| 客户端绑定 | 出题时记录客户端指纹（IP + UA 哈希），提交客户端不一致时题目直接作废 | `CAPTCHA_BIND_CLIENT` |
| 时间窗口 | 出题到提交间隔低于阈值视为自动化提交，计一次失败 | `CAPTCHA_MIN_SOLVE_TIME` |
| 工作量证明 PoW | 每道题下发 nonce 与难度位数，SDK 在后台用 SHA-256 碰撞；真人无感，批量脚本成本线性上升，难度随风险等级提升 | `CAPTCHA_POW_*` |
| 难度分级 | 1-5 级，决定字符数量、干扰强度、拼图尺寸、旋转区间与校验容差；连续失败或高风险分自动升级 | `CAPTCHA_DEFAULT_DIFFICULTY` |
| 服务端选题 | `type=auto` 时由服务端按风险等级定题型，客户端无法固定选最易自动化的题型 | — |
| 行为/环境风控 | SDK 采集指针轨迹（速度、直线度、停顿、过冲等）与环境信号（webdriver、自动化框架痕迹、软件渲染器等），服务端规则评分 0-100：达到升级阈值提升后续难度，达到拦截阈值拒绝本次校验（响应与答案错误一致） | `RISK_*` |
| Redis 限流 | 基于有序集合的滑动窗口，多实例共享；Redis 不可用时回退进程内限流 | `RATE_LIMIT_BACKEND` |
| 可信代理 | 只读取 `REMOTE_IP_HEADERS` 列出的代理头，且仅在直连来源命中 `TRUSTED_PROXIES` 时采信 | `TRUSTED_PROXIES` `REMOTE_IP_HEADERS` |
| 请求体上限 | 校验提交接口在全局上限之外单独限制请求体（chunked 同样生效），遥测列表单项长度也有上限 | `MAX_REQUEST_SIZE` `VERIFY_MAX_BODY_SIZE` |

**采集级别与隐私**

- `telemetry='full'`（默认）：轨迹 + 环境信号 + WebGL 渲染器 + 设备指纹哈希
- `telemetry='basic'`：轨迹 + 基础环境信号（UA、语言、屏幕、时区、触控能力）
- `telemetry='off'`：不采集，仅计算 PoW

采集内容不含可直接识别个人的信息，服务端只保存客户端哈希与评分，不落原始 IP / UA；建议在隐私政策中声明验证码会采集交互行为用于人机判别。上线初期可用 `RISK_MODE=log` 只记分不拦截，确认误报率后再切到 `enforce`。

## 二次验证（S2S）

前端 `onSuccess` 拿到的 `data.second_key` 随表单提交到**业务后端**，由业务后端凭**应用密钥**向验证码服务查询：

```bash
curl https://captcha.yourdomain.com/api/v1/captchas/states/{second_key} \
  -H "Authorization: Bearer sk_xxxxxxxxxxxxxxxx"
```

| 情况 | HTTP | 响应体 |
|---|---|---|
| 校验通过 | 200 | `{"valid": true}` |
| `second_key` 无效、已过期或已查询过（只能查一次） | 200 | `{"code": 1004, "error": "验证码已失效"}` |
| 应用密钥缺失、无效或应用已停用 | 401 | `{"code": 3001, "error": "应用密钥缺失或无效"}` |

只有拿到 `"valid": true` 才放行，其余一律按校验失败处理。

- 应用密钥在 [管理后台](#管理后台) 的「接入应用」里按业务后端分别创建（`sk_` 开头），泄露时只需重置或停用对应应用，日志里也能看出每次查询来自哪个应用。
- 密钥只能放在业务后端，**绝不能**进入前端代码；服务端只存哈希，明文只在创建或重置时显示一次。
- 修改过 `API_PREFIX` 时，S2S 地址和 SDK 的 `api.prefix` 都要换成同一前缀。

---

## 管理后台

管理后台内嵌在服务里、常驻开启，访问 **`http://<服务地址>/console/`**（路径由 `CONSOLE_PATH` 决定），与 API 同源，无需配置跨域。登录受行为验证码保护，登录后可管理背景图、接入应用与管理员密码，不影响任何验证码接口。

页面由 `sdk/playground/console` 构建、经 `//go:embed` 打进二进制，镜像构建时自动生成；`API_PREFIX` 在返回页面时注入，修改后无需重新构建前端。页面带严格的 CSP（只允许同源脚本、禁止被 iframe 嵌入）与 `noindex`。

### 管理员密码

- **首次启动**随机生成 20 位密码，明文只在服务日志打印一次（容器部署用 `docker compose logs captcha | grep -A6 管理后台初始密码` 取出）。
- **首次登录必须改密**，改密前管理接口只开放会话查询、注销与改密。新密码 10 个字符以上、不超过 72 字节。
- 改密或重置后，所有已登录会话立即失效，发起改密的页面自动换用新会话。
- 遗失密码时重新生成（下次登录同样要求改密），运行中的服务无需重启：

  ```bash
  cd backend && ./run-dev.sh reset-admin-password                    # 本地
  docker compose exec captcha ./simple-captcha reset-admin-password  # 容器
  ```

用户名由 `ADMIN_USERNAME` 决定，默认 `admin`。

### 数据目录

- **`ADMIN_DATA_DIR`**（默认 `./data`）保存管理员凭据 `admin.json` 与接入应用 `apps.json`，只含哈希、权限 0600，**必须持久化**：删掉对应的卷（例如 `docker compose down -v`）会重新生成管理员密码，并使全部应用密钥失效。
- **`CAPTCHA_IMAGE_PATH`**（默认 `./images`）是背景图目录，上传与删除直接写入，必须可写。容器以 uid 1001 运行，改为绑定宿主机目录时需 `chown -R 1001:1001 <目录>`。

### 登录流程

```
前端                                     验证码后端
 │  useCaptcha().verify()  ──► 完成行为验证，得到 second_key
 │  POST /admin/login { username, password, captchaKey: second_key }
 ├────────────────────────────────────────►│ 1. 一次性消费 second_key（无论密码对错）
 │                                         │ 2. 常量时间比对用户名 + bcrypt 校验密码
 │  ◄── { token, mustChangePassword, … }   │ 3. 签发随机会话令牌，仅在 Redis 保存其哈希
 │  后续请求携带 Authorization: Bearer <token>
 │
 │  mustChangePassword 为 true 时：
 │  PUT /admin/password { currentPassword, newPassword }
 ├────────────────────────────────────────►│ 4. 校验当前密码 → 写入新哈希 → 旧会话全部失效
 │  ◄── { token, mustChangePassword: false }  （新会话）
```

每次登录都必须重新过一次行为验证，暴力猜密码的成本因此与过验证码绑定；登录与改密接口另受 `CAPTCHA_IP_RATE_LIMIT` 限流。会话有效期由 `ADMIN_SESSION_TTL` 控制，注销即失效。

### 接入应用

每个调用 S2S 接口的业务后端对应一个应用、一把独立密钥。

| 操作 | 说明 |
|------|------|
| 新建 | 名称 1-40 个字符（不区分大小写去重），返回 `sk_` 开头的密钥，**明文只显示这一次** |
| 列表 | 应用 ID、名称、密钥前缀（仅供辨认）、启用状态、最近使用时间、密钥更新时间，不含密钥本身 |
| 停用 / 启用 | 停用后密钥立即无法通过鉴权，启用后恢复 |
| 重置密钥 | 生成新密钥并立即作废旧密钥；要平滑切换，可先新建一个应用，切换完成再删除旧应用 |
| 删除 | 密钥立即失效 |

服务端只保存密钥的 SHA-256 哈希并做常量时间比较，最多 100 个应用。**一个应用都没有时 S2S 查询一律返回 401**，不存在“未配置则放行”。每次查询都会记录应用 ID（服务日志）与最近使用时间（Redis，仅供展示）。

> **从旧版本升级**：`API_KEY`、`ADMIN_ENABLED`、`ADMIN_PASSWORD`、`ADMIN_PASSWORD_HASH` 已移除，留在 `.env` 里也不再生效。升级后先取出初始密码登录，为每个业务后端创建应用并替换密钥，在此之前 S2S 查询返回 401。

### 背景图管理

| 操作 | 说明 |
|------|------|
| 列表 | 文件名、像素尺寸、体积、格式、修改时间，以及当前的上传约束 |
| 上传 | `multipart/form-data`，字段名 `file`。按真实内容识别格式（仅 JPEG / PNG），校验最小尺寸、像素总数与体积，文件名由服务端重新生成 |
| 删除 | 拒绝删除最后一张，保证始终有素材可出题 |
| 预览 | 返回原文件，需携带会话令牌 |

上传或删除后素材**热重载**，下一次出题即生效，无需重启。

**素材规格**：点选与两种拼图使用 300×200（3:2）画布，旋转题从同一批素材裁出正方形、显示为 128px 的圆；服务端最低按 2 倍出图（600×400），3 倍屏出 900×600。每张素材按 110% / 125% 两档放大并随机取景，出图后再叠加轻微的颜色扰动与噪点，同一张素材每次出题的像素都不同，拿原图逐像素相减也找不出缺口。

| 项目 | 要求 |
|---|---|
| 比例 | 3:2 横图（其他比例也能用，但会多裁掉一部分画面） |
| 尺寸 | 建议 1800 × 1200；下限 900 × 600（上传时校验，保证 3 倍出图不靠放大） |
| 格式 | JPG（sRGB，质量 ≥ 85）或 PNG |
| 数量 | 至少 50 张，100 张以上更好；素材越多越难被建库比对 |

内容要求：

- 画面处处有细节和中等明暗，避免大片纯色、纯白或纯黑区域，否则缺口一眼可见、点选文字也看不清。
- 有明确的“上下方向”（地平线、建筑、站立的人），旋转题依赖它；俯拍和抽象纹理不适合。
- 不带文字、Logo、水印，避免可识别的人脸；整套素材风格统一（全部摄影或全部插画）。

每张素材常驻内存约 12 MB（含各出图尺寸、各放大档的缓存），100 张约 1.2 GB。放大档位见 `backend/services/backgrounds.go` 的 `backgroundZoomPercents`，档位越多越难比对，内存占用也越大。

> 素材保存在本地磁盘，适用于单实例部署；多副本部署需挂载共享卷，否则各实例素材不一致。

---

## 环境变量参考

开发默认值见 [`backend/.env.development`](backend/.env.development)，生产模板见 [`backend/.env.example`](backend/.env.example)。应用密钥与管理员密码不走环境变量，见 [管理后台](#管理后台)。

### 基础

| 变量 | 说明 | 默认值 |
|------|------|--------|
| `REDIS_HOST` | Redis 主机地址（**必填**） | — |
| `REDIS_PORT` | Redis 端口 | `6379` |
| `REDIS_PASSWORD` | Redis 密码 | 空 |
| `REDIS_DB` | Redis 数据库编号 | `0` |
| `REDIS_POOL_SIZE` | Redis 连接池大小 | `10` |
| `PORT` | 服务监听端口 | `3321` |
| `API_PREFIX` | 验证码 API 路径前缀；配置 `/` 表示根路径 | `/api/v1` |
| `CAPTCHA_EXPIRE_TIME` | 验证码过期时间 | `5m` |
| `CAPTCHA_MAX_ATTEMPTS` | 最大验证尝试次数 | `5` |
| `CAPTCHA_IP_RATE_LIMIT` | 单 IP 频率限制 | `30` |
| `ENABLE_RATE_LIMIT` | 是否启用频率限制 | `true` |
| `RATE_LIMIT_REQUESTS` | 全局频率限制 | `100` |
| `CORS_ENABLED` | 是否启用跨域响应 | `true` |
| `CORS_ALLOW_ORIGINS` | 允许的来源，逗号分隔 | `*` |
| `CORS_ALLOW_METHODS` | 允许的方法，逗号分隔 | `GET,POST,PUT,DELETE,OPTIONS` |
| `CORS_ALLOW_HEADERS` | 允许的请求头，逗号分隔 | `Content-Type,...` |
| `CORS_EXPOSE_HEADERS` | 向浏览器暴露的响应头，逗号分隔 | 空 |
| `CORS_ALLOW_CREDENTIALS` | 是否允许 Cookie 等跨域凭证 | `false` |
| `CORS_MAX_AGE` | 浏览器缓存预检结果的时长 | `24h` |
| `ENABLE_METRICS` | 是否启用监控指标 | `true` |
| `GIN_MODE` | Gin 运行模式 | `release` |

> 生产环境建议把 `CORS_ALLOW_ORIGINS` 配成明确的 HTTPS 来源。启用 `CORS_ALLOW_CREDENTIALS` 时，中间件返回具体来源而不是 `*`。

### 安全与风控

| 变量 | 说明 | 默认值 |
|------|------|--------|
| `CAPTCHA_DEFAULT_DIFFICULTY` | 基础难度等级 1-5 | `3` |
| `CAPTCHA_MIN_SOLVE_TIME` | 出题到提交的最短间隔，低于则判定为脚本 | `500ms` |
| `CAPTCHA_BIND_CLIENT` | 是否将题目绑定到客户端（IP + UA 哈希） | `true` |
| `CAPTCHA_IMAGE_QUALITY` | 主图 JPEG 质量 55-100 | `80` |
| `CAPTCHA_MAX_SCALE` | 允许的最大出图倍率 1-3 | `3` |
| `CAPTCHA_POW_ENABLED` | 是否下发工作量证明挑战 | `true` |
| `CAPTCHA_POW_REQUIRED` | 是否强制要求 PoW（`false` 时旧版 SDK 仍可通过，但计入风险分） | `false` |
| `CAPTCHA_POW_DIFFICULTY` | PoW 基础难度（前导零位数） | `14` |
| `CAPTCHA_POW_MAX_DIFFICULTY` | PoW 难度上限 | `20` |
| `RISK_ENABLED` | 是否启用行为/环境风控评分 | `true` |
| `RISK_MODE` | `enforce` 按阈值拦截；`log` 仅记录分数 | `enforce` |
| `RISK_ESCALATE_THRESHOLD` | 达到该分数后提升该客户端后续难度与 PoW 位数 | `55` |
| `RISK_BLOCK_THRESHOLD` | 达到该分数直接拒绝本次校验 | `85` |
| `RISK_STATE_TTL` | 客户端风控状态保留时长 | `30m` |
| `RISK_TELEMETRY_LOG_SIZE` | Redis 中保留的最近遥测记录条数（`captcha:telemetry` 列表），0 表示不保存 | `1000` |
| `RATE_LIMIT_BACKEND` | `redis` 滑动窗口（多实例共享）或 `memory` | `redis` |
| `TRUSTED_PROXIES` | 可信代理网段，只有来自这些来源的代理头才会被采信；生产环境建议收窄为实际代理地址 | 私有网段 |
| `REMOTE_IP_HEADERS` | 解析客户端 IP 时按顺序读取的代理头，逗号分隔；部署在 Cloudflare / Akamai 之后追加 `CF-Connecting-IP` / `True-Client-IP` | `X-Forwarded-For,X-Real-IP` |
| `MAX_REQUEST_SIZE` | 全局请求体上限（字节） | `33554432` |
| `VERIFY_MAX_BODY_SIZE` | 校验提交接口（`/attempts`）的请求体上限（字节），超出返回 HTTP 413 | `262144` |

### 管理后台

| 变量 | 说明 | 默认值 |
|------|------|--------|
| `ADMIN_USERNAME` | 管理员用户名 | `admin` |
| `ADMIN_DATA_DIR` | 管理员凭据与接入应用的保存目录，必须持久化 | `./data` |
| `CONSOLE_PATH` | 内嵌管理后台的挂载路径；不能是根路径，也不能与 API、健康检查等路由重叠 | `/console` |
| `ADMIN_SESSION_TTL` | 登录会话有效期 | `2h` |
| `ADMIN_MAX_UPLOAD_SIZE` | 单张背景图上传体积上限（字节），不能超过 `MAX_REQUEST_SIZE` | `5242880` |
| `ADMIN_MAX_IMAGE_PIXELS` | 单张背景图像素总数上限，防止解压炸弹 | `24000000` |

---

## 本地开发

### 项目结构

```
simple-captcha/
├── backend/              # Go 后端服务
│   ├── controllers/      # 路由控制层
│   ├── services/         # 业务逻辑层
│   ├── middleware/       # 中间件（CORS、鉴权、限流等）
│   ├── config/           # 环境变量配置
│   ├── images/           # 验证码素材图片（管理后台直接读写此目录）
│   ├── data/             # 管理员凭据与接入应用（首次启动生成，不入库）
│   ├── console/          # 内嵌的管理后台页面（dist/ 由 sdk 的 pnpm build:console 生成，不入库）
│   └── Dockerfile
├── sdk/                  # React 前端 SDK
│   ├── src/
│   │   ├── Captcha.tsx   # 核心组件
│   │   ├── _internal/    # 内部渲染引擎（点选、滑块、旋转）
│   │   └── hooks/        # 公共 Hooks
│   ├── playground/       # Vite 演示站（直接引用 src，热更新）；console/ 为内嵌进 Go 服务的管理后台入口
│   └── package.json
├── docker-compose.yml    # 本地开发编排
└── .github/workflows/    # CI/CD 自动发布
```

### 启动后端

```bash
cd backend
./run-dev.sh               # 等价于 make dev；验证：curl http://localhost:3321/health
```

- 配置按优先级加载：命令行变量（如 `PORT=4000 ./run-dev.sh`）> `.env`（个人覆盖，不入库）> `.env.development`（仓库默认）。
- 首次启动在终端打印管理员初始密码，凭据保存在 `backend/data/`；遗失执行 `./run-dev.sh reset-admin-password`。
- `/console/` 内嵌的是 `backend/console/dist` 里现有的产物，没构建过时只显示构建提示；需要时先 `make console`（即 `cd sdk && pnpm build:console`）再重启后端。

用 compose 跑生产形态：

```bash
cp backend/.env.example backend/.env       # 填写 Redis、CORS 等
docker compose up -d
docker compose logs backend | grep -A6 管理后台初始密码
```

SDK 的开发、构建与测试命令见 [`sdk/README.md`](sdk/README.md#本地开发)。

---

## License

[MIT](LICENSE)
