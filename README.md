# Simple Captcha

一个开箱即用的行为验证码解决方案，包含 Go 后端服务和 React 前端 SDK。

支持 **文字点选**、**图形点选**、**滑块拼图**、**滑块区域**、**旋转验证** 五种验证模式，采用标准 RESTful API 设计，通过 Docker 一键部署，前端通过 npm 包快速接入。

## 目录

- [架构概览](#架构概览)
- [验证流程](#验证流程)
- [后端部署](#后端部署)
- [前端接入](#前端接入)
- [API 参考](#api-参考)
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
docker pull aetherlib/simple-captcha:latest
```

**2. 创建 `docker-compose.yml`**

```yaml
services:
  captcha:
    image: aetherlib/simple-captcha:latest
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
      - API_KEY=your_secure_api_key_here    # S2S 二次验证鉴权密钥

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

### RESTful API 路由

| 方法 | 路径 | 说明 | 鉴权 |
|------|------|------|------|
| `GET` | `/api/v1/captchas/:type` | 获取验证码 | 无 |
| `POST` | `/api/v1/captchas/:type/:key/attempts` | 提交验证答案 | 无 |
| `GET` | `/api/v1/captchas/states/:key` | 查询验证状态 (S2S) | `Authorization: Bearer <API_KEY>` |
| `GET` | `/health` | 健康检查 | 无 |

**支持的验证码类型 (`:type`)**

| 值 | 验证方式 |
|---|---|
| `click-text` | 文字点选 |
| `click-shape` | 图形点选 |
| `slide-text` | 滑块验证 |
| `slide-region` | 区域滑块 |
| `rotate` | 旋转验证 |

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
| `api` | `ApiConfig` | — | 全局接口配置，使用 `api.basePath` 指定后端服务地址 |
| `locale` | `LocaleTexts` | — | 全局文案覆盖 |
| `theme` | `ThemeTokens` | — | 全局主题配置 |
| `zIndex` | `number` | `9999` | 弹窗层级 |

### `<Captcha>` Props

| 属性 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `type` | `CaptchaType` | `'auto'` | 验证码类型，`auto` 随机选择 |
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
| `REDIS_PORT` | Redis 端口 | `6379` |
| `REDIS_PASSWORD` | Redis 密码 | 空 |
| `REDIS_DB` | Redis 数据库编号 | `0` |
| `REDIS_POOL_SIZE` | Redis 连接池大小 | `10` |
| `CAPTCHA_EXPIRE_TIME` | 验证码过期时间 | `5m` |
| `CAPTCHA_MAX_ATTEMPTS` | 最大验证尝试次数 | `5` |
| `CAPTCHA_IP_RATE_LIMIT` | 单 IP 频率限制 | `30` |
| `ENABLE_RATE_LIMIT` | 是否启用频率限制 | `true` |
| `RATE_LIMIT_REQUESTS` | 全局频率限制 | `100` |
| `ENABLE_METRICS` | 是否启用监控指标 | `true` |
| `GIN_MODE` | Gin 运行模式 | `release` |

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
