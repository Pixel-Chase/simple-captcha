# Simple Captcha

一个开箱即用的行为验证码解决方案，包含 Go 后端服务和 React 前端 SDK。

支持 **文字点选**、**图形点选**、**滑块拼图**、**滑块区域**、**旋转验证** 五种验证模式，采用标准 RESTful API 设计，通过 Docker 一键部署，前端通过 npm 包快速接入。

## 目录

- [架构概览](#架构概览)
- [验证流程](#验证流程)
- [后端部署](#后端部署)
- [前端接入](#前端接入)
- [API 参考](#api-参考)
- [安全机制](#安全机制)
- [二次验证（S2S）](#二次验证s2s)
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
                                      │ Authorization: Bearer <API_KEY>
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

### 前置要求

- Docker & Docker Compose
- 外部 Redis 服务（版本 6.0+）

### 快速启动

**1. 拉取镜像**

```bash
docker pull ghcr.io/pixel-chase/simple-captcha:latest
```

**2. 创建 `docker-compose.yml`**

```yaml
services:
  captcha:
    image: ghcr.io/pixel-chase/simple-captcha:latest
    container_name: simple-captcha
    ports:
      - "3321:3321"
    environment:
      # Redis 连接（必填，连接到你的业务 Redis）
      - REDIS_HOST=your-redis-host
      - REDIS_PORT=6379
      - REDIS_PASSWORD=your-redis-password
      - REDIS_DB=2

      # 服务配置
      - PORT=3321
      - API_PREFIX=/api/v1                  # API 路径前缀，可选
      - API_KEY=your_secure_api_key_here    # S2S 二次验证鉴权密钥

      # 跨域配置（多个来源使用逗号分隔）
      - CORS_ALLOW_ORIGINS=https://example.com,https://admin.example.com
      - CORS_ALLOW_CREDENTIALS=false

      # 可选：频率限制
      - CAPTCHA_EXPIRE_TIME=5m
      - CAPTCHA_MAX_ATTEMPTS=5
      - ENABLE_RATE_LIMIT=true
    restart: unless-stopped
    healthcheck:
      test: ["CMD", "wget", "--no-verbose", "--tries=1", "--spider", "http://localhost:3321/health"]
      interval: 30s
      timeout: 10s
      retries: 3
```

**3. 启动服务**

```bash
docker compose up -d
```

**4. 验证部署**

```bash
curl http://localhost:3321/health
# 返回: {"status":"ok","redis":{"status":"ok"}, ...}
```

### 镜像发布

后端镜像由 GitHub Actions 直接构建并发布到 GitHub Container Registry：

- 镜像地址：`ghcr.io/pixel-chase/simple-captcha`
- 推送到默认分支时生成 `latest`、分支名和 `sha-*` 标签
- 推送 `v*` 版本标签时生成语义化版本标签，并同步更新 `latest`
- 支持 `linux/amd64` 和 `linux/arm64` 两种架构

私有仓库或私有 Package 拉取前需要登录 GHCR：

```bash
echo "<GITHUB_TOKEN>" | docker login ghcr.io -u <GITHUB_USERNAME> --password-stdin
```

### RESTful API 路由

| 方法 | 路径 | 说明 | 鉴权 |
|------|------|------|------|
| `GET` | `/api/v1/captchas/:type` | 获取验证码 | 无 |
| `POST` | `/api/v1/captchas/:type/:key/attempts` | 提交验证答案 | 无 |
| `GET` | `/api/v1/captchas/states/:key` | 查询验证状态 (S2S) | `Authorization: Bearer <API_KEY>` |
| `GET` | `/health` | 健康检查 | 无 |

以上验证码接口使用默认的 `API_PREFIX=/api/v1`。例如配置 `API_PREFIX=/captcha/api` 后，获取验证码接口将变为 `/captcha/api/captchas/:type`；配置 `/` 则直接挂载到 `/captchas`。

**支持的验证码类型 (`:type`)**

| 值 | 验证方式 |
|---|---|
| `click-text` | 文字点选 |
| `click-shape` | 图形点选 |
| `slide-text` | 滑块验证 |
| `slide-region` | 区域滑块 |
| `rotate` | 旋转验证 |
| `auto` | 由服务端根据该客户端的风险等级自动选择题型（低风险可得到滑块/旋转，高风险只给点选类） |

### 接口字段说明

**`GET /captchas/:type?mode=dark&dpr=2`**

| 查询参数 | 说明 |
|---|---|
| `mode` | `light` / `dark`，影响点选文字配色，可选 |
| `dpr` | 前端 `devicePixelRatio`（1-3），服务端按此倍率出图以适配高清屏，可选，默认 1 |

响应（HTTP 200）：

```json
{
  "key": "f3c1…（32 位随机十六进制）",
  "type": "slide-text",
  "image": "data:image/jpeg;base64,…",
  "thumb": "data:image/png;base64,…",
  "thumbWidth": 110, "thumbHeight": 110, "thumbX": 20, "thumbY": 60,
  "imageWidth": 600, "imageHeight": 440,
  "scale": 2,
  "difficulty": 3,
  "pow": { "nonce": "…", "difficulty": 14 }
}
```

- `imageWidth` / `imageHeight`、`thumb*` 均为位图像素；SDK 以 CSS 像素渲染后，会按 `imageWidth / 渲染宽度` 把坐标换算回位图空间再提交，服务端的容差也在位图空间按倍率放大。
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

答案字段按题型取其一：点选 `dots`（`"x1,y1,x2,y2"`）、滑块 `point`（`"x,y"`）、旋转 `angle`。`trace` / `env` / `pow` 由 SDK 自动填充，旧版 SDK 不携带时依旧可以验证，但会计入风险分。

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
| `1008` | 风控评分超过拦截阈值 |
| `1009` | 工作量证明缺失或无效 |

---

## 前端接入

### 安装

```bash
npm install @aetherlib/g-captcha
# 或
yarn add @aetherlib/g-captcha
# 或
pnpm add @aetherlib/g-captcha
```

> **要求**: React >= 19.0.0, React DOM >= 19.0.0

### 推荐接法

当前版本采用新的配置模型：

- `CaptchaProvider` 负责全局 `api / locale / theme / zIndex`
- `Captcha` 负责单个实例渲染
- `useCaptcha()` 负责按次触发验证，并返回 `holder`

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

通过 `locale` 属性覆盖默认中文文案，实现国际化：

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

| Key | 默认值 | 说明 |
|-----|--------|------|
| `userCancel` | `用户取消` | 用户取消时的提示 |
| `loading` | `加载中...` | 加载状态文案 |
| `clickTitle` | `请在下图依次点击` | 点选模式标题 |
| `slideTitle` | `请拖动滑块完成拼图` | 滑块/旋转模式标题 |
| `confirmButton` | `确认` | 点选确认按钮 |
| `buttonText` | `点击按键进行验证` | 触发验证按钮默认文案 |

### `ThemeTokens` 主题配置

```tsx
<CaptchaProvider
  theme={{
    primaryColor: '#347eff',
    backgroundColor: '#ffffff',
    textColor: '#333333',
    borderColor: '#e2e2e2',
    borderRadius: 4,
  }}
>
  <App />
</CaptchaProvider>
```

### `useCaptcha()` 返回值

| 属性 | 类型 | 说明 |
|------|------|------|
| `verify` | `(options?: CaptchaVerifyOptions) => Promise<any>` | 按次触发验证，并允许覆盖 `type / api / locale / theme / config` |
| `holder` | `ReactNode` | 必须渲染在当前 React 树中的承载节点 |

---

## 安全机制

| 机制 | 说明 | 相关配置 |
|---|---|---|
| 随机 key | 验证码 key 为 128 位加密随机数，与答案无关，无法离线反推 | — |
| 客户端绑定 | 出题时记录客户端指纹（IP + UA 哈希），提交客户端不一致时题目直接作废 | `CAPTCHA_BIND_CLIENT` |
| 时间窗口 | 出题到提交间隔低于阈值判定为自动化提交，计入一次失败 | `CAPTCHA_MIN_SOLVE_TIME` |
| 工作量证明 PoW | 每道题下发 nonce 与难度位数，SDK 在后台用 SHA-256 碰撞；真人无感，批量脚本成本线性上升。难度随客户端风险等级提升 | `CAPTCHA_POW_*` |
| 难度分级 | 1-5 级，映射到字符数量、干扰强度、拼图尺寸、旋转区间以及校验容差；连续失败或高风险分会自动升级 | `CAPTCHA_DEFAULT_DIFFICULTY` |
| 服务端选题 | `type=auto` 时由服务端按风险等级决定题型，客户端无法固定选择最易自动化的题型 | — |
| 行为/环境风控 | SDK 采集指针轨迹（速度、直线度、停顿、过冲等）与环境信号（webdriver、自动化框架痕迹、软件渲染器等），服务端规则评分 0-100；达到升级阈值提升后续难度，达到拦截阈值拒绝本次校验 | `RISK_*` |
| Redis 限流 | 基于有序集合的滑动窗口，多实例共享；Redis 不可用时自动回退进程内限流 | `RATE_LIMIT_BACKEND` |
| 可信代理 | `X-Forwarded-For` 等头只在直连来源命中 `TRUSTED_PROXIES` 时才被采信 | `TRUSTED_PROXIES` |

**采集级别与隐私**

- `telemetry='full'`（默认）：轨迹 + 环境信号 + WebGL 渲染器 + 设备指纹哈希
- `telemetry='basic'`：轨迹 + 基础环境信号（UA、语言、屏幕、时区、触控能力）
- `telemetry='off'`：不采集，仅计算 PoW

采集内容不含任何可直接识别个人的信息；服务端只保存客户端哈希与评分，不落原始 IP / UA。建议在隐私政策中声明验证码会采集交互行为用于人机判别。`RISK_MODE=log` 可先只记录分数不拦截，观察误报率后再切换为 `enforce`。

## 二次验证（S2S）

用户在前端完成验证后，`onSuccess` 回调返回的 `data` 中包含 `second_key`。业务前端应将此 key 随表单提交到**业务后端**，由业务后端发起服务端到服务端（S2S）的二次校验：

```bash
# 业务后端调用验证码后端，验证 second_key 是否有效
curl -X GET \
  https://captcha.yourdomain.com/api/v1/captchas/states/{second_key} \
  -H "Authorization: Bearer your_secure_api_key_here"
```

**成功响应**（HTTP 200）：

```json
{
  "valid": true
}
```

**失败响应**（HTTP 400/401/403）：

```json
{
  "code": 1003,
  "message": "验证码已失效"
}
```

> **安全提醒**：`API_KEY` 仅在业务后端使用，**绝不能**暴露到前端代码中。

若后端修改了 `API_PREFIX`，SDK 与业务后端的 S2S 请求也需使用相同前缀：

```tsx
<CaptchaProvider
  api={{
    basePath: 'https://captcha.yourdomain.com',
    prefix: '/captcha/api',
  }}
>
  <App />
</CaptchaProvider>
```

---

## 环境变量参考

### 必填

| 变量 | 说明 | 示例 |
|------|------|------|
| `REDIS_HOST` | Redis 主机地址 | `redis.example.com` |
| `API_KEY` | S2S 二次验证鉴权密钥 | `sk-xxxxxxxxxxxx` |

### 可选

| 变量 | 说明 | 默认值 |
|------|------|--------|
| `PORT` | 服务监听端口 | `3321` |
| `API_PREFIX` | 验证码 API 路径前缀；配置 `/` 表示根路径 | `/api/v1` |
| `REDIS_PORT` | Redis 端口 | `6379` |
| `REDIS_PASSWORD` | Redis 密码 | 空 |
| `REDIS_DB` | Redis 数据库编号 | `0` |
| `REDIS_POOL_SIZE` | Redis 连接池大小 | `10` |
| `CAPTCHA_EXPIRE_TIME` | 验证码过期时间 | `5m` |
| `CAPTCHA_MAX_ATTEMPTS` | 最大验证尝试次数 | `5` |
| `CAPTCHA_IP_RATE_LIMIT` | 单 IP 频率限制 | `30` |
| `ENABLE_RATE_LIMIT` | 是否启用频率限制 | `true` |
| `RATE_LIMIT_REQUESTS` | 全局频率限制 | `100` |
| `CORS_ENABLED` | 是否启用跨域响应 | `true` |
| `CORS_ALLOW_ORIGINS` | 允许的来源，多个值用逗号分隔 | `*` |
| `CORS_ALLOW_METHODS` | 允许的方法，多个值用逗号分隔 | `GET,POST,PUT,DELETE,OPTIONS` |
| `CORS_ALLOW_HEADERS` | 允许的请求头，多个值用逗号分隔 | `Content-Type,...` |
| `CORS_EXPOSE_HEADERS` | 向浏览器暴露的响应头，多个值用逗号分隔 | 空 |
| `CORS_ALLOW_CREDENTIALS` | 是否允许 Cookie 等跨域凭证 | `false` |
| `CORS_MAX_AGE` | 浏览器缓存预检结果的时长 | `24h` |
| `ENABLE_METRICS` | 是否启用监控指标 | `true` |
| `GIN_MODE` | Gin 运行模式 | `release` |

> 生产环境建议将 `CORS_ALLOW_ORIGINS` 配置为明确的 HTTPS 来源。启用 `CORS_ALLOW_CREDENTIALS` 时，中间件会返回具体来源而不是 `*`。

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
| `TRUSTED_PROXIES` | 可信代理网段，只有来自这些来源的 `X-Forwarded-For` 等头才会被采信 | 私有网段 |

---

## 本地开发

### 项目结构

```
simple-captcha/
├── backend/              # Go 后端服务
│   ├── controllers/      # 路由控制层
│   ├── services/         # 业务逻辑层
│   ├── middleware/        # 中间件（CORS、鉴权、限流等）
│   ├── config/           # 环境变量配置
│   ├── images/           # 验证码素材图片
│   └── Dockerfile
├── sdk/                  # React 前端 SDK
│   ├── src/
│   │   ├── Captcha.tsx   # 核心组件
│   │   ├── _internal/    # 内部渲染引擎（点选、滑块、旋转）
│   │   └── hooks/        # 公共 Hooks
│   └── package.json
├── docker-compose.yml    # 本地开发编排
└── .github/workflows/    # CI/CD 自动发布
```

### 启动后端

```bash
docker compose up -d
# 验证：curl http://localhost:3321/health
```

### 启动前端文档站

```bash
cd sdk
pnpm install
pnpm start
# 访问 http://localhost:8000/components/captcha 查看 Demo
```

---

## License

[MIT](LICENSE)
