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
┌──────────────────┐  HTTPS   ┌─────────────────────────────┐
│   前端页面        │────────►│  Simple Captcha Backend (Go) │
│  @aetherlib/     │ ?appId=  │  :3321                       │
│  g-captcha       │  Origin  │  ├─ data/captcha.db (SQLite) │
└──────────────────┘          │  │   账号、应用、策略、风控观察 │
                              │  └─ 进程内存：题目、计数等短期数据 │
                              └─────────────────────────────┘
                                       ▲
                                       │ Authorization: Bearer <应用密钥>
                               ┌───────┴────────┐
                               │  业务后端 (S2S)  │
                               │  二次验证查询     │
                               └────────────────┘
```

- 单个容器即可运行，不依赖 Redis 或外部数据库。持久数据（管理员账号、接入应用、验证码与风控策略、风控观察记录）存放在数据目录下的 SQLite 文件 `captcha.db`；题目答案、尝试次数、二次验证 key、IP 计数等带过期时间的短期数据存放在进程内存，重启后失效（已发出的题目需要重新获取）。
- 只支持单实例部署。
- 每个接入方在管理后台创建一个应用，前端通过 `appId` 标识自己；服务端只接受该应用「允许来源」里的页面发起的出题与提交。

## 验证流程

```
用户操作                    前端 SDK                     验证码后端                    业务后端

  │  点击验证                  │                            │                           │
  ├────────────────────────►│                            │                           │
  │                          │  采集环境信号(≤150ms)      │                           │
  │                          │  POST /captchas/:type      │                           │
  │                          │       ?appId=…             │                           │
  │                          │  (请求体携带 env)          │                           │
  │                          ├───────────────────────────►│                           │
  │                          │     校验应用与来源          │                           │
  │                          │     核验请求头+环境信号    │                           │
  │                          │     决定首题是否加难       │                           │
  │                          │  ◄── 返回图片+key           │                           │
  │  ◄── 显示验证码弹窗       │                            │                           │
  │                          │                            │                           │
  │  完成验证操作              │                            │                           │
  ├────────────────────────►│                            │                           │
  │                          │  POST /captchas/:type      │                           │
  │                          │       :key/attempts?appId= │                           │
  │                          ├───────────────────────────►│                           │
  │                          │  ◄── 返回 second_key       │                           │
  │  ◄── onSuccess(data)     │                            │                           │
  │                          │                            │                           │
  │  提交表单(含 second_key)   │                            │                           │
  ├─────────────────────────────────────────────────────────────────────────────────►│
  │                          │                            │  GET /captchas/states/    │
  │                          │                            │      :key（应用密钥）     │
  │                          │                            │  ◄────────────────────────┤
  │                          │                            │  ── { valid: true }      │
  │                          │                            │  ────────────────────────►│
  │  ◄── 业务处理结果          │                            │                           │
```

题目与 `second_key` 都绑定出题时的应用：提交必须来自同一应用，`second_key` 也只能被该应用的密钥查询。

## 后端部署

### 快速启动

只需要 Docker Compose，不需要 Redis，也不需要准备任何配置文件。

**1. 创建 `docker-compose.yml`**

```yaml
services:
  captcha:
    image: ghcr.io/pixel-chase/simple-captcha:latest
    container_name: simple-captcha
    ports:
      - "3321:3321"
    # 可选：端口、证书、可信代理网段等部署参数写在 .env 里，没有这个文件也能启动（需要 Docker Compose 2.24+）
    env_file:
      - path: .env
        required: false
    volumes:
      # 持久数据（管理员账号、接入应用、策略配置、风控观察），必须持久化：
      # 删除后回到未初始化状态，全部应用密钥失效
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

**2. 启动并检查**

```bash
docker compose up -d
curl http://localhost:3321/health   # {"status":"ok","storage":{"status":"ok"}, ...}
```

**3. 在控制台完成初始化**

首次启动还没有管理员账号，日志里会打印一次性的初始化口令：

```bash
docker compose logs captcha | grep -A4 初始化口令
```

打开 `http://<服务地址>:3321/console/`，在初始化页面填写口令，设置管理员用户名与密码，完成后直接进入控制台。口令在初始化后作废；未初始化时每次启动都会换一个新口令，以最近一次日志为准。

**4. 创建接入应用并配置**

在「接入应用」里为每个接入方创建应用，填写允许来源（前端页面的 origin），把应用 ID 交给前端、应用密钥交给业务后端。验证码难度、PoW、风控阈值等策略在「策略配置」里修改，保存后立即生效。详见 [管理后台](#管理后台)。

### 反向代理与备份

- 验证码服务建议使用独立域名（例如 `https://captcha.example.com`），API 路径固定为 `/api/v1`，不支持挂在子路径下。
- 管理后台只接受同源请求：浏览器的 `Origin` 必须与后端看到的请求地址（协议 + 主机名 + 端口）一致，否则控制台登录用的验证码返回 403 / 1011。HTTPS 在反向代理上终结时，后端收到的是明文 HTTP，只能从 `X-Forwarded-Proto` 得知原始协议，因此以下几点缺一不可：
  - 转发浏览器请求中的原始 `Host`（含端口）。nginx 要用 `$http_host`，不能用 `$host`：`$host` 不带端口，服务开在 8443 等非默认端口时比对失败。
  - 转发 `X-Forwarded-Proto` 与 `X-Forwarded-Host`。
  - 代理的直连地址（后端看到的连接来源）必须命中 `TRUSTED_PROXIES`，否则后端会忽略这两个头。默认值只包含私有网段（`10.0.0.0/8`、`172.16.0.0/12`、`192.168.0.0/16`），不包含 `127.0.0.1`：
    - 用本仓库的 compose 部署，nginx 在宿主机转发到映射端口：容器看到的来源是 Docker 网络网关（如 `172.20.0.1`），已在默认值内，不用另外配置。
    - nginx 与直接运行的后端进程在同一台机器上，转发到 `127.0.0.1`：需在 `backend/.env` 中写入 `TRUSTED_PROXIES=127.0.0.1,::1`。设置这一项后默认值不再生效，需要的网段都要写上。
    - 不确定直连地址时查看后端日志：代理未被信任时，`来源校验未通过` 日志里的 `ip` 就是代理的直连地址。

  nginx 示例（与后端同机或映射到宿主机端口）：

  ```nginx
  location / {
    proxy_pass http://127.0.0.1:3321;
    proxy_set_header Host $http_host;
    proxy_set_header X-Forwarded-Host $http_host;
    proxy_set_header X-Forwarded-Proto $scheme;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
  }
  ```

- 备份：停止服务后复制 `captcha-data` 卷（其中的 `captcha.db`、`captcha.db-wal`、`captcha.db-shm` 一起复制），恢复时放回原卷即可。

### 镜像发布

镜像由 GitHub Actions 构建并发布到 `ghcr.io/pixel-chase/simple-captcha`（`linux/amd64` / `linux/arm64`）：推送默认分支生成 `latest`、分支名与 `sha-*` 标签，推送 `v*` 标签生成语义化版本并更新 `latest`。私有 Package 拉取前需先 `docker login ghcr.io`。

本地构建以仓库根目录为上下文（镜像内先构建管理后台前端，再内嵌进 Go 二进制）：`docker build -f backend/Dockerfile .`

### RESTful API 路由

API 路径固定为 `/api/v1`。用户侧接口（出题、提交）在查询参数里携带 `appId`，并按应用的允许来源校验 `Origin`；S2S 查询用应用密钥鉴权；`/admin/*` 用登录返回的会话令牌（`Authorization: Bearer <…>`）。

| 方法 | 路径 | 说明 | 鉴权 |
|------|------|------|------|
| `POST` | `/api/v1/captchas/:type?appId=…` | 获取验证码（请求体携带环境信号，服务端出题前据此加难） | 应用 + 来源 |
| `POST` | `/api/v1/captchas/:type/:key/attempts?appId=…` | 提交答案 | 应用 + 来源 |
| `GET` | `/api/v1/captchas/states/:key` | 查询验证状态（S2S），只能查询本应用的 key | 应用密钥 |
| `GET` | `/health` | 健康检查 | — |
| `GET` / `POST` | `/api/v1/admin/setup` | 查询是否已初始化 / 首次初始化（需要启动日志中的口令，成功后直接返回会话） | — |
| `POST` | `/api/v1/admin/login` | 管理员登录（需携带管理后台验证码的 `second_key`，叠加 IP 限流） | — |
| `GET` / `DELETE` | `/api/v1/admin/session` | 查询 / 注销会话 | 会话令牌 |
| `PUT` | `/api/v1/admin/password` | 修改密码（旧会话全部失效，返回新会话） | 会话令牌 |
| `GET` / `PUT` | `/api/v1/admin/settings` | 读取（含默认值）/ 保存验证码与风控策略，保存后立即生效 | 会话令牌 |
| `GET` / `POST` | `/api/v1/admin/backgrounds` | 列出 / 上传背景图 | 会话令牌 |
| `GET` / `DELETE` | `/api/v1/admin/backgrounds/:name` | 预览 / 删除背景图 | 会话令牌 |
| `GET` / `POST` | `/api/v1/admin/apps` | 列出 / 新建接入应用（含允许来源） | 会话令牌 |
| `PUT` / `DELETE` | `/api/v1/admin/apps/:id` | 修改名称、启用状态与允许来源 / 删除应用 | 会话令牌 |
| `POST` | `/api/v1/admin/apps/:id/secret` | 重置应用密钥 | 会话令牌 |
| `GET` | `/api/v1/admin/risk/telemetry` | 最近的风控观察记录（已脱敏，`limit` 默认 100、上限 500） | 会话令牌 |

- 跨域：出题与提交接口的预检（`OPTIONS`）对任意来源都返回 204，真实请求才校验应用与来源，被拒绝时返回 HTTP 403 并同样带上 `Access-Control-Allow-Origin`，SDK 才能读到错误码。`/admin/*` 与 S2S 接口不返回任何跨域响应头。
- 旧版 `GET /api/v1/captchas/:type` 出题接口已删除。

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

**`POST /captchas/:type?appId=app_xxxxxxxxxxxx`**

```json
{
  "mode": "dark",
  "dpr": 2,
  "env": { "ua": "…", "platform": "Win32", "languages": ["zh-CN"], "tz": "Asia/Shanghai", "tzOffset": -480, "screen": [1920, 1080], "viewport": [1536, 864], "dpr": 2, "webdriver": false, "automation": [], "anomalies": [], "fp": "…" }
}
```

- `mode`：`light` / `dark`，影响点选文字配色，可选。
- `dpr`：前端 `devicePixelRatio`（1-3），服务端按此倍率出图（至少 2 倍，但不超过策略「最大出图倍率」），可选。
- `env`：出题前采集的环境信号（可选，`telemetry='off'` 时不携带）。服务端用它与请求头（`User-Agent`、`Accept-Language`、`Sec-Fetch-*`、Chromium 的 `Sec-CH-UA*`）交叉核验，连同指纹重放计数一起决定首题是否加难；`anomalies` 是 SDK 检测到的无头 / 篡改痕迹标记。请求体可以为空。

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

**`POST /captchas/:type/:key/attempts?appId=app_xxxxxxxxxxxx`**

```json
{
  "point": "123,45",
  "trace": { "points": [[x, y, t], …], "clicks": [[x, y, t]], "pointerType": "mouse", "startedAt": 820, "submittedAt": 2410, "downs": 1, "ups": 1, "moves": 96, "untrusted": 0 },
  "env":   { "ua": "…", "languages": ["zh-CN"], "screen": [1920, 1080], "viewport": [1440, 900], "dpr": 2, "webdriver": false, "automation": [], "anomalies": [], "fp": "…" },
  "pow":   { "nonce": "…", "solution": "48213" }
}
```

答案字段按题型取其一：点选 `dots`（`"x1,y1,x2,y2"`）、滑块 `point`（`"x,y"`）、旋转 `angle`。`trace` / `env` / `pow` 由 SDK 自动填充；不携带也能验证，但会计入风险分。`appId` 必须与出题时一致，否则按题目不存在处理（`1004`）。

**业务错误码**（HTTP 200 或 4xx，响应体为 `{ "code": …, "error": "…" }`）

| code | 含义 |
|---|---|
| `400` | 参数错误 |
| `429` | 触发频率限制（仅管理员登录、初始化与改密；验证码接口不按 IP 拒绝） |
| `1003` | 答案错误 |
| `1004` | 验证码不存在 / 已过期 / 已失效 / 不属于当前应用 |
| `1005` | 尝试次数过多 |
| `1006` | 出题到提交间隔过短（疑似脚本） |
| `1007` | 提交客户端与出题客户端不一致 |
| `1009` | 工作量证明缺失或无效 |
| `1010` | 应用不存在或已停用（HTTP 403） |
| `1011` | 请求来源不在应用的允许名单内，或缺少 `Origin`（HTTP 403） |

> 风控评分超过拦截阈值时，对外同样返回 `1003` 并消耗一次尝试机会；真实原因（内部码 `1008` 与评分理由）只记在服务端日志，避免成为脚本调参的探针。

---

## 前端接入

### 安装

```bash
pnpm add @aetherlib/g-captcha   # 或 npm install / yarn add
```

要求 React 与 React DOM ≥ 18。

### 使用

先在管理后台「接入应用」里创建应用，并把当前站点的 origin（例如 `https://www.example.com`）填进允许来源。`CaptchaProvider` 提供全局的 `api / locale / theme / zIndex / telemetry`，其中 `api.baseURL` 是验证码服务的地址、`api.appId` 是应用 ID；`Captcha` 渲染单个实例，`useCaptcha()` 按次触发验证并返回需要渲染的 `holder`：

```tsx
import { useRef } from 'react';
import { Captcha, CaptchaError, CaptchaProvider, useCaptcha } from '@aetherlib/g-captcha';
import type { CaptchaRef } from '@aetherlib/g-captcha';

function App() {
  const ref = useRef<CaptchaRef>(null);
  const { verify, holder } = useCaptcha();

  return (
    <CaptchaProvider
      api={{ baseURL: 'https://captcha.yourdomain.com', appId: 'app_xxxxxxxxxxxx' }}
      theme={{ primaryColor: '#111827', borderRadius: 12 }}
    >
      {holder}

      <Captcha
        type="auto"
        ref={ref}
        onSuccess={(data) => submitForm(data.second_key)}
        onFail={(error) => console.log('验证失败', error.code, error.message)}
        onCancel={() => console.log('用户关闭了弹窗')}
      >
        <button onClick={() => ref.current?.open()}>
          组件模式
        </button>
      </Captcha>

      <button
        onClick={async () => {
          try {
            const data = await verify({ type: 'auto' });
            submitForm(data.second_key);
          } catch (e) {
            if (e instanceof CaptchaError && e.code === 'CANCELLED') return;
            console.log('验证失败', e);
          }
        }}
      >
        Hook 模式
      </button>
    </CaptchaProvider>
  );
}
```

**从 2.x 升级到 3.0**：`api` 由 `{ basePath, prefix }` 改为 `{ baseURL, appId }`。`baseURL` 只能是验证码服务的 origin（不带路径，不传表示与页面同源），API 路径固定为 `/api/v1`；`appId` 必填，缺少时出题直接失败（`LOAD_FAILED`）。升级前先在管理后台创建应用并配置允许来源，否则请求会被拒绝（`APP_REJECTED`）。

---

## API 参考

### `<CaptchaProvider>` Props

| 属性 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `api` | `ApiConfig` | — | 接入参数 `{ baseURL?, appId }`：`baseURL` 为验证码服务的 origin（如 `https://captcha.example.com`，不传表示同源），`appId` 为管理后台「接入应用」里的应用 ID（必填） |
| `locale` | `LocaleTexts` | — | 全局文案覆盖 |
| `theme` | `ThemeTokens` | — | 全局主题配置 |
| `zIndex` | `number` | `9999` | 弹窗层级 |
| `telemetry` | `'off' \| 'basic' \| 'full'` | `'full'` | 行为/环境采集级别，见[安全机制](#安全机制) |

`<Captcha>` 和 `useCaptcha()` 不单独接收这些配置，统一从最近的 `CaptchaProvider` 读取。某一块区域需要不同配置时，嵌套一层 `CaptchaProvider`，未指定的字段沿用外层：

```tsx
<CaptchaProvider api={{ baseURL: 'https://captcha.yourdomain.com', appId: 'app_xxxxxxxxxxxx' }}>
  <CaptchaProvider theme={{ primaryColor: '#059669' }}>
    <PaymentForm />
  </CaptchaProvider>
</CaptchaProvider>
```

### `<Captcha>` Props

| 属性 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `type` | `CaptchaType` | `'auto'` | 验证码类型，`auto` 由服务端按风险等级选择题型（需要后端 ≥ 1.3） |
| `maxAttempts` | `number` | `5` | 连续答错的次数上限，达到后关闭弹窗并以 `MAX_ATTEMPTS` 触发 `onFail` |
| `width` | `number` | `300` | 题图渲染宽度（CSS 像素），高度按 3:2 推算 |
| `onSuccess` | `(data: CaptchaVerifySuccessData) => void` | — | 验证成功回调，`data.second_key` 用于[二次验证](#二次验证s2s) |
| `onFail` | `(error: CaptchaError) => void` | — | 题目加载失败或答错次数用完时触发，见[错误处理](#错误处理) |
| `onCancel` | `() => void` | — | 用户关闭弹窗时触发，此时不会再触发 `onFail` |
| `children` | `ReactNode` | — | 原样渲染，通常放触发按钮 |
| `ref` | `Ref<CaptchaRef>` | — | `ref.current.open()` 打开弹窗并加载题目；结果只经由上面三个回调通知 |

### `LocaleTexts` 文案配置

通过 `locale` 覆盖默认中文文案：

```tsx
<CaptchaProvider
  locale={{
    clickTitle: 'Please click in order',
    slideTitle: 'Drag the slider to complete',
    confirmButton: 'Confirm',
    successText: 'Verified',
  }}
>
  <App />
</CaptchaProvider>
```

`{n}`、`{p}`、`{s}` 为运行时替换的占位符；`*Tag`、`regionNote`、`slideNote`、`rotateNote` 设为空字符串即隐藏。

| Key | 默认值 | 说明 |
|-----|--------|------|
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
| `regionNote` | `二维拖拽轨迹与运动特征分析` | 滑动区域底部的安全提示 |
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
| `verify` | `(options?: CaptchaVerifyOptions) => Promise<CaptchaVerifySuccessData>` | 按次触发验证，`options` 与 `<Captcha>` Props 相同（不含 `children`）；成功时 resolve，取消或失败时以 `CaptchaError` reject |
| `holder` | `ReactNode` | 必须渲染在当前 React 树中的承载节点 |

### 错误处理

验证没有成功时，SDK 统一给出 `CaptchaError`（继承自 `Error`），按 `code` 区分原因，不需要比对文案：

| `code` | 触发场景 | 通知方式 |
|------|------|------|
| `CANCELLED` | 用户关闭弹窗 | 组件模式只触发 `onCancel`；Hook 模式同时以此 reject |
| `LOAD_FAILED` | 题目获取失败：网络异常、缺少 `appId`、`baseURL` 不是 origin，或服务端返回其他错误 | `onFail`，Hook 模式同时 reject |
| `MAX_ATTEMPTS` | 连续答错达到 `maxAttempts`，弹窗已关闭 | `onFail`，Hook 模式同时 reject |
| `ABORTED` | 上一次 `verify()` 被新的调用替换，或组件已卸载 | 仅 Hook 模式 reject |
| `APP_REJECTED` | 服务端拒绝了本应用：应用不存在或已停用（`1010`），或当前页面来源不在允许名单内（`1011`）。出题与提交都可能出现，提交时出现不计入答错次数 | `onFail`，Hook 模式同时 reject |

`error.message` 是可直接展示的中文说明；`LOAD_FAILED`、`MAX_ATTEMPTS` 与 `APP_REJECTED` 还带有 `error.response`，即服务端返回的 `{ code, error }`。

---

## 安全机制

| 机制 | 说明 | 相关配置 |
|---|---|---|
| 应用与来源校验 | 出题与提交都要带 `appId`，服务端只接受该应用允许来源（精确匹配的 origin）发起的请求，应用停用即全部拒绝；校验在出题之前完成，被拒绝的请求不消耗出图 CPU。应用 ID 是公开的，`Origin` 也能被脚本伪造，这一层只阻止别的网站在用户浏览器里调用本服务，防脚本仍靠下面的风控评分与 PoW | 接入应用 · 允许来源 |
| 验证码与应用绑定 | 题目与 `second_key` 都记录出题的应用：提交必须来自同一应用，`second_key` 只能被该应用的密钥查询，其他应用的查询既无效也不消费；管理后台登录只接受 `app_console` 的 key | — |
| 随机 key | 验证码 key 为 128 位加密随机数，与答案无关，无法离线反推 | — |
| 客户端绑定 | 出题时记录客户端指纹（IP + UA 哈希），提交客户端不一致时题目直接作废 | 策略 · 绑定客户端 |
| 时间窗口 | 出题到提交间隔低于阈值视为自动化提交，计一次失败 | 策略 · 最短作答时间 |
| 工作量证明 PoW | 每道题下发 nonce 与难度位数，SDK 在后台用 SHA-256 碰撞；真人无感，批量脚本成本线性上升，难度随风险等级提升 | 策略 · 工作量证明 |
| 难度分级 | 1-5 级，决定字符数量、干扰强度、拼图尺寸、旋转区间与校验容差；连续失败、高风险分、出口 IP 压力与出题前的环境评估都只会在默认难度之上加难，**只升不降**：真人保持默认难度，脚本逐级变难 | 策略 · 基础难度 |
| 出题前环境评估 | SDK 出题前采集环境信号随 `POST /captchas/:type` 上报，服务端结合请求头核验与指纹重放计数算出环境分：≥ 30 加 1 级、≥ 55 加 2 级。webdriver、自动化框架痕迹、Worker 与主线程不一致等硬性标记单独命中即加 1 级；软件渲染器（VDI、虚拟机）等弱信号单独命中不加难 | 策略 · 环境分上限 |
| 请求头核验 | 服务端检查请求头是否像真实浏览器：UA 含 `Mozilla/`、Chrome / Firefox 必带 `Sec-Fetch-*`、Chromium 89+ 必带 `Sec-CH-UA*`、有 `Accept-Language`；再与自报环境交叉比对 UA、平台、语言、时区偏移、DPR 与移动标记。curl、requests 等脚本与只改 UA 的伪装直接加难，Safari / WebView 不因缺少 Chromium 专有头扣分 | — |
| 指纹重放计数 | 同一设备指纹（SDK 上报的 `fp`）在时间窗内被多个不同客户端复用，即复制真浏览器 env 批量重放，超过阈值加 1 级；同一客户端反复出题不计 | 策略 · 指纹重放阈值 / 时间窗 |
| headless 痕迹检测 | SDK 出题前在 150ms 预算内并行检测：无头窗口（`outerWidth` 为 0）、通知权限矛盾、无媒体设备、桌面 Chrome 缺少 `window.chrome`、Worker 线程与主线程的 navigator 不一致、navigator 关键 getter 被改写。检测思路参考 BotD / CreepJS（声明见 [`sdk/THIRD_PARTY_NOTICES.md`](sdk/THIRD_PARTY_NOTICES.md)），只加难不拦截，检测失败即跳过 | — |
| TLS / HTTP2 指纹（可选） | 由 fingerproxy 等 TLS 终结代理注入 JA3 / JA4 / HTTP2 指纹头，仅在直连来源命中 `TRUSTED_PROXIES` 时采信：按可配置列表识别脚本工具链的指纹（硬性标记）与"指纹所属浏览器家族 ≠ UA 家族"的伪装；不配置列表时只记录到遥测。部署示例见 [`deploy/fingerproxy/`](deploy/fingerproxy/README.md) | `RISK_TLS_FINGERPRINT_HEADER` `RISK_HTTP2_FINGERPRINT_HEADER` `RISK_TLS_FINGERPRINT_LIST` |
| 服务端选题 | `type=auto` 时由服务端按风险等级定题型，客户端无法固定选最易自动化的题型 | — |
| 行为/环境风控 | SDK 采集指针轨迹（速度、直线度、停顿、过冲等）与环境信号（webdriver、自动化框架痕迹、软件渲染器等），服务端规则评分 0-100：达到升级阈值提升后续难度；达到拦截阈值且行为分不低于「拦截所需最低行为分」（或命中 webdriver 等硬性标记）时拒绝本次校验（响应与答案错误一致）。环境类信号对总分的贡献有上限（「环境分上限」），VDI、隐私浏览器等正常环境最多被加难，不会仅凭环境被拦 | 策略 · 风控评分 |
| 单 IP 加难 | 按出口 IP 统计每分钟出题与提交次数（进程内存中的滑动窗口），超过阈值只提升后续题目难度，**不按 IP 拒绝**：攻击者会更换 IP，按 IP 拒绝只会让企业 NAT、运营商 CGNAT 下共用出口的正常用户一起无法使用。S2S 状态查询不计入出口 IP。针对单一来源的流量洪水请在网关或 WAF 层按实际网络情况防护 | 策略 · 单 IP 加难阈值 |
| 可信代理 | 只读取 `REMOTE_IP_HEADERS` 列出的代理头，且仅在直连来源命中 `TRUSTED_PROXIES` 时采信 | `TRUSTED_PROXIES` `REMOTE_IP_HEADERS` |
| 请求体上限 | 校验提交接口在全局上限之外单独限制请求体（chunked 同样生效），遥测列表单项长度也有上限 | `MAX_REQUEST_SIZE` `VERIFY_MAX_BODY_SIZE` |

**采集级别与隐私**

- `telemetry='full'`（默认）：轨迹 + 环境信号 + 无头 / 篡改痕迹标记 + WebGL 渲染器 + 设备指纹哈希
- `telemetry='basic'`：轨迹 + 基础环境信号（UA、语言、屏幕、时区、触控能力）+ 无头 / 篡改痕迹标记
- `telemetry='off'`：不采集，仅计算 PoW；出题仍走 `POST`，服务端按无环境信号处理

采集内容不含可直接识别个人的信息；风控观察记录（`captcha.db` 的 `telemetry` 表）里的 IP 只保留前两段、UA 只保留浏览器家族与主版本摘要，客户端以 IP + UA 的哈希标识，TLS / HTTP2 指纹与设备指纹哈希原样保存供运维整理规则。建议在隐私政策中声明验证码会采集交互行为与运行环境特征用于人机判别。上线初期可在「策略配置」把运行模式设为 `log`，只记分不拦截，确认误报率后再切回 `enforce`。

Worker 一致性检测需要站点 CSP 允许 `worker-src blob:`；禁止时该项自动跳过，不影响其他功能，也不会误加难。

## 二次验证（S2S）

前端 `onSuccess` 拿到的 `data.second_key` 随表单提交到**业务后端**，由业务后端凭**应用密钥**向验证码服务查询：

```bash
curl https://captcha.yourdomain.com/api/v1/captchas/states/{second_key} \
  -H "Authorization: Bearer sk_xxxxxxxxxxxxxxxx"
```

| 情况 | HTTP | 响应体 |
|---|---|---|
| 校验通过 | 200 | `{"valid": true}` |
| `second_key` 无效、已过期、已查询过（只能查一次）或属于其他应用 | 200 | `{"code": 1004, "error": "验证码已失效"}` |
| 应用密钥缺失、无效或应用已停用 | 401 | `{"code": 3001, "error": "应用密钥缺失或无效"}` |

只有拿到 `"valid": true` 才放行，其余一律按校验失败处理。

- `second_key` 绑定出题时的应用：只能用**同一应用**的密钥查询；用其他应用的密钥查询时响应与无效 key 相同，并且**不会消费**该 key，一个应用无法用掉另一个应用的验证结果。管理后台（`app_console`）的 key 只能用于管理员登录。
- 应用密钥在 [管理后台](#管理后台) 的「接入应用」里创建（`sk_` 开头），泄露时只需重置或停用对应应用，日志里也能看出每次查询来自哪个应用。
- 密钥只能放在业务后端，**绝不能**进入前端代码；服务端只存哈希，明文只在创建或重置时显示一次。

---

## 管理后台

管理后台内嵌在服务里、常驻开启，访问 **`http://<服务地址>/console/`**，与 API 同源，无需配置跨域。登录受行为验证码保护（使用内置的系统应用 `app_console`），登录后可管理背景图、接入应用、策略配置与管理员密码，并查看风控观察页，不影响任何验证码接口。

页面由 `sdk/playground/console` 构建、经 `//go:embed` 打进二进制，镜像构建时自动生成。页面带严格的 CSP（只允许同源脚本、禁止被 iframe 嵌入）与 `noindex`。

### 首次初始化

- 数据目录里还没有管理员账号时，服务处于**未初始化**状态：每次启动都会生成一个新的一次性初始化口令，只打印在服务日志里（`docker compose logs captcha | grep -A4 初始化口令`），库中只存哈希。
- 打开 `/console/` 会显示初始化页面：填写口令，设置管理员用户名（1-64 个字符）与密码（10 个字符以上、不超过 72 字节），提交后直接进入控制台，口令随即作废。
- 口令防止服务在初始化前暴露在公网时被他人抢先初始化；初始化接口与登录一样按 IP 限流。
- 只有一个管理员账号。改密后所有已登录会话立即失效，发起改密的页面自动换用新会话。
- 遗失密码时重置管理员账号，回到初始化状态（运行中的服务无需重启；接入应用、策略与背景图不受影响）：

  ```bash
  cd backend && ./run-dev.sh reset-admin                    # 本地
  docker compose exec captcha ./simple-captcha reset-admin  # 容器
  ```

  命令会打印新的初始化口令，在控制台重新设置用户名与密码即可。

### 数据目录

- **`data/`**（工作目录下，镜像内为 `/app/data`）里的 `captcha.db` 是 SQLite 数据库，保存管理员账号与会话、接入应用与允许来源、策略配置、风控观察记录。密码用 bcrypt，应用密钥、会话令牌、初始化口令只存哈希；数据库文件权限为 0600，启动时数据目录收紧为 0700（已存在的目录也会收紧）。改为绑定宿主机目录时，该目录的属主须是容器内的 uid 1001（`chown 1001:1001 <目录>`），否则无法收紧权限，服务会拒绝启动。**必须持久化**：删掉对应的卷（例如 `docker compose down -v`）会回到未初始化状态，并使全部应用密钥失效。
- 题目、尝试次数、二次验证 key、IP 计数、客户端风控状态等短期数据只在进程内存里，重启即清空。
- **`CAPTCHA_IMAGE_PATH`**（默认 `./images`）是背景图目录，上传与删除直接写入，必须可写。容器以 uid 1001 运行，改为绑定宿主机目录时需 `chown -R 1001:1001 <目录>`。

### 登录流程

```
前端                                     验证码后端
 │  useCaptcha().verify()  ──► 以 app_console 完成行为验证，得到 second_key
 │  POST /admin/login { username, password, captchaKey: second_key }
 ├────────────────────────────────────────►│ 1. 一次性消费 second_key（只接受 app_console 的 key，无论密码对错）
 │                                         │ 2. 常量时间比对用户名 + bcrypt 校验密码
 │  ◄── { token, username, expiresAt }     │ 3. 签发随机会话令牌，库中只保存其哈希
 │  后续请求携带 Authorization: Bearer <token>
```

每次登录都必须重新过一次行为验证，暴力猜密码的成本因此与过验证码绑定；接入应用里通过的验证码不能拿来登录。登录、初始化与改密接口另按 IP 限流，每分钟上限为 `ADMIN_IP_LIMIT_PER_MINUTE`，与验证码请求分开计数，同一出口下的验证码流量不会挤占管理员的额度。会话有效期由 `ADMIN_SESSION_TTL` 控制，注销即失效。

### 接入应用

每个接入方（一个站点或一组前端页面 + 它的业务后端）对应一个应用：应用 ID 交给前端（公开），应用密钥交给业务后端（保密）。

| 操作 | 说明 |
|------|------|
| 新建 | 名称 1-40 个字符（不区分大小写去重）与允许来源，返回 `sk_` 开头的密钥，**明文只显示这一次**，同时给出前端接入代码 |
| 允许来源 | 前端页面的 origin（协议 + 域名 + 端口，例如 `https://www.example.com`），每个应用最多 20 个；自动统一小写、去掉默认端口与末尾斜杠，只做精确匹配，不支持通配符。没有配置来源的应用，前端出题与提交一律被拒绝 |
| 列表 | 应用 ID、名称、允许来源、密钥前缀（仅供辨认）、启用状态、最近使用时间、密钥更新时间，不含密钥本身 |
| 停用 / 启用 | 停用后前端请求返回 `1010`，密钥立即无法通过鉴权；启用后恢复 |
| 重置密钥 | 生成新密钥并立即作废旧密钥；要平滑切换，可先新建一个应用，切换完成再删除旧应用 |
| 删除 | 密钥与来源立即失效 |

- 列表第一项是内置的系统应用 **`app_console`（管理后台）**：只接受同源请求，没有密钥、不能用于 S2S，不能删除、停用、改名、重置密钥或修改来源。
- 服务端只保存密钥的 SHA-256 哈希，最多 100 个应用。**没有任何应用时 S2S 查询一律返回 401**，不存在“未配置则放行”。每次查询都会记录应用 ID（服务日志）与最近使用时间。

### 策略配置

出题、校验、工作量证明与风控评分的 23 项参数在「策略配置」页面修改，保存后下一次出题即生效，无需重启；页面显示每项的默认值与取值范围，并提供“恢复全部默认值”。只保存当前值，不记录修改历史。

| 分组 | 参数（默认值） |
|---|---|
| 验证码 | 基础难度（3 级）、题目有效期（300 秒）、二次验证有效期（120 秒）、最大尝试次数（5）、最短作答时间（500 毫秒）、单 IP 加难阈值（30 次/分钟，0 关闭）、绑定客户端（开启）、主图质量（80）、最大出图倍率（3） |
| 工作量证明 | 下发 PoW（开启）、强制要求 PoW（关闭）、基础难度（14 位）、难度上限（20 位） |
| 风控评分 | 启用（开启）、运行模式（`enforce` / `log`）、加难阈值（55）、拦截阈值（85）、拦截所需最低行为分（20）、环境分上限（60）、指纹重放阈值（5）、指纹重放时间窗（600 秒）、客户端状态保留时长（1800 秒）、风控观察保留条数（10000，0 不记录） |

对应接口：`GET /api/v1/admin/settings` 返回 `{ settings, defaults }`；`PUT` 提交完整对象，校验失败返回 400，错误信息逐条列出字段名与取值范围，失败时不保存。

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

### 风控观察

只读页面，列出最近的出题评估（应用、难度、加难级数、环境分、原因）与校验评分（应用、分数、是否通过、原因），调整「策略配置」中的风控参数前先在这里对照真人与脚本的原因分布。数据保存在 `captcha.db` 的 `telemetry` 表（条数上限为策略「风控观察保留条数」，出题与校验共用），后台异步批量写入，绝不阻塞验证请求。IP、UA 在写入时已脱敏，客户端只显示哈希前缀；可信代理注入的 TLS / HTTP2 指纹原样显示，便于整理指纹列表。对应接口 `GET /api/v1/admin/risk/telemetry?limit=100`（上限 500）。

---

## 环境变量参考

环境变量只保留部署相关的 29 项，全部有默认值，**不配置也能启动**。验证码与风控策略在管理后台「策略配置」修改，管理员账号在初始化页面设置，应用密钥与允许来源在「接入应用」里配置。开发默认值见 [`backend/.env.development`](backend/.env.development)，生产模板见 [`backend/.env.example`](backend/.env.example)。

### 基础

| 变量 | 说明 | 默认值 |
|------|------|--------|
| `PORT` | 服务监听端口（镜像内为 `3321`） | `8080` |
| `HOST` | 监听地址 | `0.0.0.0` |
| `GIN_MODE` | Gin 运行模式（`debug` / `release` / `test`） | `release` |
| `SERVER_READ_TIMEOUT` | 读超时 | `30s` |
| `SERVER_WRITE_TIMEOUT` | 写超时 | `30s` |
| `SERVER_IDLE_TIMEOUT` | 空闲连接超时 | `60s` |
| `SERVER_GRACEFUL_TIMEOUT` | 优雅关闭的最长等待时间 | `30s` |
| `SERVER_MAX_HEADER_BYTES` | 请求头上限（字节） | `1048576` |
| `ENABLE_HTTPS` | 是否直接以 HTTPS 监听（需同时配置证书） | `false` |
| `CERT_FILE` / `KEY_FILE` | HTTPS 证书与私钥路径 | 空 |
| `CAPTCHA_IMAGE_PATH` | 背景图目录，必须可写 | `./images` |
| `ENABLE_METRICS` | 是否采集请求指标并开放 `/metrics`；关闭后 `/stats` 不再附带请求指标 | `true` |
| `METRICS_PATH` / `HEALTH_PATH` / `STATS_PATH` | 监控接口路径 | `/metrics` / `/health` / `/stats` |

### 安全与风控

| 变量 | 说明 | 默认值 |
|------|------|--------|
| `TRUSTED_PROXIES` | 可信代理网段：只有来自这些来源的代理头（客户端 IP、`X-Forwarded-Host` / `X-Forwarded-Proto`）才会被采信；生产环境建议收窄为实际代理地址；不含 `127.0.0.1`，与后端同机的代理需显式加入，见 [反向代理与备份](#反向代理与备份) | 私有网段 |
| `REMOTE_IP_HEADERS` | 解析客户端 IP 时按顺序读取的代理头，逗号分隔；部署在 Cloudflare / Akamai 之后追加 `CF-Connecting-IP` / `True-Client-IP` | `X-Forwarded-For,X-Real-IP` |
| `IP_WHITELIST` | 白名单 IP，逗号分隔：不计入单 IP 加难，也不受管理员接口限流 | 空 |
| `IP_BLACKLIST` | 黑名单 IP，逗号分隔：运维显式封禁，直接返回 403 | 空 |
| `MAX_REQUEST_SIZE` | 全局请求体上限（字节） | `33554432` |
| `VERIFY_MAX_BODY_SIZE` | 校验提交接口（`/attempts`）的请求体上限（字节），超出返回 HTTP 413 | `262144` |
| `RISK_TLS_FINGERPRINT_HEADER` | 可信代理注入的 TLS 指纹（JA3 / JA4）请求头名，空表示不读取 | 空 |
| `RISK_HTTP2_FINGERPRINT_HEADER` | 可信代理注入的 HTTP/2 指纹请求头名，空表示不读取 | 空 |
| `RISK_TLS_FINGERPRINT_LIST` | 指纹参考列表 JSON（浏览器家族映射 + 拒绝列表）路径，空表示指纹只记录不计分；配置时必须同时配置至少一个指纹头名 | 空 |

### 管理后台

| 变量 | 说明 | 默认值 |
|------|------|--------|
| `ADMIN_SESSION_TTL` | 登录会话有效期 | `2h` |
| `ADMIN_MAX_UPLOAD_SIZE` | 单张背景图上传体积上限（字节），不能超过 `MAX_REQUEST_SIZE` | `5242880` |
| `ADMIN_MAX_IMAGE_PIXELS` | 单张背景图像素总数上限，防止解压炸弹 | `24000000` |
| `ADMIN_IP_LIMIT_PER_MINUTE` | 登录、初始化与改密接口的单 IP 每分钟上限，超出返回 `429`；与验证码请求分开计数，`0` 不限 | `30` |

### 已删除的变量

以下变量已删除，留在 `.env` 里不再生效，启动时会逐个打印警告：

| 变量 | 去向 |
|---|---|
| `REDIS_*`（13 个） | 不再使用 Redis：短期数据存进程内存，持久数据存 `data/captcha.db` |
| `CORS_*`（7 个） | 跨域改为在「接入应用」里为每个应用配置允许来源 |
| `API_PREFIX` | API 路径固定为 `/api/v1`，请为验证码服务分配独立域名 |
| `ADMIN_USERNAME` | 用户名在控制台初始化页面设置 |
| `CAPTCHA_EXPIRE_TIME`、`CAPTCHA_SECOND_VERIFY_EXPIRE`、`CAPTCHA_MAX_ATTEMPTS`、`CAPTCHA_IP_ESCALATE_PER_MINUTE`、`CAPTCHA_DEFAULT_DIFFICULTY`、`CAPTCHA_MIN_SOLVE_TIME`、`CAPTCHA_BIND_CLIENT`、`CAPTCHA_IMAGE_QUALITY`、`CAPTCHA_MAX_SCALE`、`CAPTCHA_POW_*`（4 个）、`RISK_ENABLED`、`RISK_MODE`、`RISK_ESCALATE_THRESHOLD`、`RISK_BLOCK_THRESHOLD`、`RISK_STATE_TTL`、`RISK_TELEMETRY_LOG_SIZE`、`RISK_ENV_SCORE_CAP`、`RISK_BLOCK_MIN_BEHAVIOR`、`RISK_FP_REPLAY_THRESHOLD`、`RISK_FP_REPLAY_WINDOW` | 改为管理后台「策略配置」实时修改 |
| `CAPTCHA_IP_RATE_LIMIT`、`CAPTCHA_RATE_LIMIT_WINDOW`、`ENABLE_RATE_LIMIT`、`RATE_LIMIT_BACKEND` | 验证码接口不按 IP 拒绝，只按「单 IP 加难阈值」加难；管理员接口限流用 `ADMIN_IP_LIMIT_PER_MINUTE` |
| `ADMIN_DATA_DIR`、`CONSOLE_PATH` | 数据目录固定为工作目录下的 `data`（镜像内 `/app/data`），管理后台固定为 `/console/` |

---

## 本地开发

### 项目结构

```
simple-captcha/
├── backend/              # Go 后端服务
│   ├── controllers/      # 路由控制层
│   ├── services/         # 业务逻辑层
│   ├── middleware/       # 中间件（按应用的来源校验、鉴权、限流等）
│   ├── config/           # 部署参数（环境变量）与运行时策略快照
│   ├── storage/          # SQLite（data/captcha.db）连接与编号迁移
│   ├── cache/            # 进程内存中的短期数据（带过期时间）
│   ├── images/           # 验证码素材图片（管理后台直接读写此目录）
│   ├── data/             # captcha.db（首次启动生成，不入库）
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

需要 Go 1.27+，不需要 Redis。

```bash
cd backend
./run-dev.sh               # 等价于 make dev；验证：curl http://localhost:3321/health
```

- 配置按优先级加载：命令行变量（如 `PORT=4000 ./run-dev.sh`）> `.env`（个人覆盖，不入库）> `.env.development`（仓库默认）。
- 首次启动在终端打印初始化口令，打开控制台完成初始化；数据保存在 `backend/data/captcha.db`。遗失密码执行 `./run-dev.sh reset-admin` 重新初始化。
- `/console/` 内嵌的是 `backend/console/dist` 里现有的产物，没构建过时只显示构建提示；需要时先 `make console`（即 `cd sdk && pnpm build:console`）再重启后端。
- 演示站（`cd sdk && pnpm start`，端口 8000）通过 Vite 代理把 `/api/` 转发给后端，页面与 API 同源，用内置的 `app_console` 出题；代理目标默认 `http://127.0.0.1:3321`，可用 `sdk/playground/.env` 中的 `VITE_API_PROXY_TARGET` 修改。

用 compose 跑生产形态（不需要 `.env`）：

```bash
docker compose up -d --build
docker compose logs backend | grep -A4 初始化口令
```

SDK 的开发、构建与测试命令见 [`sdk/README.md`](sdk/README.md#本地开发)。

---

## License

[MIT](LICENSE)

`sdk/src/_internal/telemetry/anomalies.ts` 的无头 / 篡改痕迹检测思路参考了 BotD 与 CreepJS（均为 MIT），声明见 [`sdk/THIRD_PARTY_NOTICES.md`](sdk/THIRD_PARTY_NOTICES.md)。
