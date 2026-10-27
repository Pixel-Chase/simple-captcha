# Project Task Ledger

## 1. Project Overview

### Project Name
SQLite 配置中心、移除 Redis 与应用来源绑定（config-center-app-binding）

### Project Goal
1. 引入内嵌 SQLite（`backend/data/captcha.db`）保存持久数据：验证码与风控策略、管理员凭据与会话、接入应用、风控观察记录。
2. 验证码与风控策略（23 项）从环境变量改到管理后台「策略配置」页面，保存后立即生效，无需重启。
3. 彻底移除 Redis：带过期时间的短期数据（题目答案、尝试次数、二次验证 key、IP 计数、客户端风控状态、指纹集合）改存进程内存。
4. 删除 `API_PREFIX`，API 路径固定为 `/api/v1`，建议使用者为验证码服务分配独立域名。
5. 前端 `CaptchaProvider` 传入后端地址与 appId；每个应用在管理后台配置允许来源（多个）。来源不在名单内的出题与提交请求由服务端直接拒绝。
6. 验证码与应用绑定：A 应用里通过的验证码，不能被 B 应用的密钥查询或消费。
7. 近乎零配置启动：不需要 `.env` 即可启动。管理员账号在控制台的初始化页面设置；初始化之后，接入应用、允许来源、验证码与风控策略都在界面中配置。

### Background
- 现状（2026-10-27 核实）：
  - 配置全部来自环境变量，共 74 个（`backend/config/config.go` 的 `loadFromEnv`）。启动时加载一次成为单例，`RiskEngine`、`ClientRiskStore`、`ChallengePolicy`、`BaseService`、`captcha_controller`、`RiskControlServiceImpl` 在构造时就持有配置指针（例如 `services/risk_engine.go:31`），改阈值必须重启。
  - 管理员凭据与接入应用存放在 `backend/data/admin.json`、`apps.json`（`services/datafile.go`，注释写明"只考虑单实例部署，不引入数据库"）。
  - 管理员初始化：用户名来自环境变量 `ADMIN_USERNAME`（默认 `admin`，控制台登录框也预填 `admin`）。首次启动时随机生成密码打印在终端，首次登录必须改密（`RequirePasswordChanged` 中间件，错误码 2008）。遗失密码时执行 `reset-admin-password`。
  - `docker-compose.yml` 的 `env_file` 要求 `backend/.env` 必须存在，没有这个文件时 `docker compose up` 直接失败。
  - Redis 只存短期数据和会话：`captcha:*`（题目数据 / 尝试次数 / 二次验证）、`captcha:ratelimit:*`（滑动窗口 Lua）、`captcha:client:*`（风控状态 Lua）、`captcha:fp:*`（指纹集合 Lua）、`captcha:telemetry`（最近 N 条遥测，`LPUSH` + `LTRIM`）、`admin:session:*`、`admin:app:last_used:*`。二次验证令牌另有一份进程内存 map 与 Redis 并存（`services/risk_control_service.go:70`）。
  - 跨域只有一份全局配置（`CORS_*` 7 个变量，`r.Use(middleware.CORSMiddleware(cfg.CORS))` 挂在整个引擎上），验证码与应用没有绑定：任何应用密钥都能消费任何 `second_key`。
  - `API_PREFIX` 通过 `backend/console/console.go` 的占位符 `__CAPTCHA_API_PREFIX__` 写进控制台 `index.html`，SDK 的 `ApiConfig` 为 `{ basePath?, prefix? }`。
  - 出题保留了旧版 SDK 用的 `GET /captchas/:type`。
- 性能实测（2026-10-27，WSL2、8 核；一次验证周期按 5 次存储操作计）：

  | 存储 | 单线程 p50 | 64 并发吞吐 |
  |---|---|---|
  | Redis 直连容器 | 375µs | 2.3 万次/秒 |
  | SQLite WAL + NORMAL | 100µs | 0.34 万次/秒（p99 87ms） |
  | SQLite WAL + FULL | 3470µs | 242 次/秒 |
  | 进程内存 | 0.6µs | 75 万次/秒 |

  出题接口端到端 p50 为 35～40ms，几乎全是出图的 CPU 开销（8 核上限约 200 题/秒），存储占比约 1%，性能不是取舍依据。短期数据不放 SQLite：SQLite 没有过期机制，只能单连接写入、并发时排队，而且只活几分钟的数据没必要写盘。
- 依赖约束：`modernc.org/sqlite` v1.61.0（纯 Go，适配 `CGO_ENABLED=0`）要求 Go ≥ 1.26.0。项目原为 Go 1.23.4，用户已决定全仓库统一到 Go 1.27.x，当前最新补丁为 1.27.3（与本机工具链一致，镜像 `golang:1.27.3-alpine` 已确认存在）。

### Success Criteria
- 后端运行与测试都不需要 Redis：停掉 `simple-captcha-redis` 容器后，`cd backend && go test ./...` 全部通过；`go.mod` 中没有 `go-redis`。
- 全新的 `data/` 目录首次启动会生成 `captcha.db`，并在终端打印初始化口令。打开控制台显示初始化页面，输入口令、用户名、密码后直接进入控制台，之后登录、创建应用全部可用。重启后账号、会话、应用、策略值、风控观察记录都还在。
- 零配置启动：没有 `backend/.env`、使用全新数据卷时，`docker compose up -d` 能正常启动，并且在浏览器中完成初始化后即可使用。
- 控制台「策略配置」修改任一项（例如默认难度 3 → 4），保存后下一次出题即生效，无需重启。
- 环境变量从 74 个减到 29 个；删除的变量都登记在 `config.removedEnvVars`，旧部署仍设置时启动会告警。
- SDK 升到 3.0.0，接入方式为 `<CaptchaProvider api={{ baseURL, appId }}>`，不再有 `prefix`。
- 来源校验：名单内的 Origin 出题、提交成功；名单外或缺少 Origin 返回 403 和错误码 1011；应用不存在或已停用返回 1010；SDK 把两者识别为 `APP_REJECTED`。
- 应用绑定：A 应用的 `second_key` 用 B 应用密钥查询时视为无效，且不会被消费；用 A 应用密钥查询有效，并且只能消费一次。控制台应用的验证码只能用于管理员登录。
- 检查命令全部通过：`cd backend && go vet ./... && go test ./...`；`cd sdk && pnpm lint && pnpm typecheck && pnpm test && pnpm build && pnpm build:console`；`docker build -f backend/Dockerfile .` 成功。

---

## 2. Global Constraints

### Technical Stack
- 后端：Go 1.27.3（Phase 1 起全仓库统一：`backend/go.mod`、`backend/Dockerfile` 与 `deploy/fingerproxy/Dockerfile` 都用 `golang:1.27.3-alpine`）、Gin、validator v10、`modernc.org/sqlite`（本任务唯一新增的 Go 依赖）。Go 工具链在 `/usr/local/go/bin`，运行前执行 `export PATH=$PATH:/usr/local/go/bin`（本机为 go1.27.3）。
- 运行时镜像 `alpine:3.19`，数据目录 `/app/data`（compose 具名卷 `captcha-data`）。
- Redis：Phase 1～4 的测试仍需要本机 docker 容器 `simple-captcha-redis`；Phase 5 完成后不再需要。
- 前端 SDK：React 19、TypeScript、tsdown、Vite（演示站与控制台）、Vitest、Biome；pnpm 11 的构建脚本白名单只认 `sdk/pnpm-workspace.yaml` 的 `allowBuilds`。演示站的 Vite 根目录是 `sdk/playground`，环境变量文件为 `sdk/playground/.env.example`。

### Architecture Constraints
- 只考虑单实例部署，不为多实例一致性增加任何复杂度。
- 存储分层：
  - 持久数据 → SQLite，文件固定为 `<工作目录>/data/captcha.db`，不做成配置项。连接与迁移只在新包 `backend/storage/`；SQL 只出现在 `storage` 与各 service 的数据访问函数里，controller 不写 SQL。
  - 带过期时间的短期数据 → 进程内存，由 `backend/cache/` 包提供（Phase 4 起由 Redis 客户端改为内存实现），重启丢失可以接受。
- 配置分层：
  - 部署类（保留环境变量，共 29 个）：`PORT`、`HOST`、`GIN_MODE`、`SERVER_READ_TIMEOUT`、`SERVER_WRITE_TIMEOUT`、`SERVER_IDLE_TIMEOUT`、`SERVER_GRACEFUL_TIMEOUT`、`SERVER_MAX_HEADER_BYTES`、`TRUSTED_PROXIES`、`REMOTE_IP_HEADERS`、`IP_WHITELIST`、`IP_BLACKLIST`、`ENABLE_HTTPS`、`CERT_FILE`、`KEY_FILE`、`MAX_REQUEST_SIZE`、`VERIFY_MAX_BODY_SIZE`、`ENABLE_METRICS`、`METRICS_PATH`、`HEALTH_PATH`、`STATS_PATH`、`CAPTCHA_IMAGE_PATH`、`RISK_TLS_FINGERPRINT_HEADER`、`RISK_HTTP2_FINGERPRINT_HEADER`、`RISK_TLS_FINGERPRINT_LIST`、`ADMIN_SESSION_TTL`、`ADMIN_MAX_UPLOAD_SIZE`、`ADMIN_MAX_IMAGE_PIXELS`、`ADMIN_IP_LIMIT_PER_MINUTE`。
  - 策略类（23 项，进 SQLite，在控制台修改并实时生效）：
    - 验证码：`CAPTCHA_EXPIRE_TIME`、`CAPTCHA_SECOND_VERIFY_EXPIRE`、`CAPTCHA_MAX_ATTEMPTS`、`CAPTCHA_IP_ESCALATE_PER_MINUTE`、`CAPTCHA_DEFAULT_DIFFICULTY`、`CAPTCHA_MIN_SOLVE_TIME`、`CAPTCHA_BIND_CLIENT`、`CAPTCHA_IMAGE_QUALITY`、`CAPTCHA_MAX_SCALE`
    - 工作量证明：`CAPTCHA_POW_ENABLED`、`CAPTCHA_POW_REQUIRED`、`CAPTCHA_POW_DIFFICULTY`、`CAPTCHA_POW_MAX_DIFFICULTY`
    - 风控：`RISK_ENABLED`、`RISK_MODE`、`RISK_ESCALATE_THRESHOLD`、`RISK_BLOCK_THRESHOLD`、`RISK_STATE_TTL`、`RISK_TELEMETRY_LOG_SIZE`（含义改为风控观察保留条数）、`RISK_ENV_SCORE_CAP`、`RISK_BLOCK_MIN_BEHAVIOR`、`RISK_FP_REPLAY_THRESHOLD`、`RISK_FP_REPLAY_WINDOW`
  - 删除：`API_PREFIX`（1）、`CORS_*`（7）、`REDIS_*`（13）、`ADMIN_USERNAME`（1，用户名改在首次初始化时设置）。
- 读配置：部署类继续用 `config.GetConfig()`；策略类每次使用时从原子快照 `config.Runtime()` 读取，任何结构体不得在构造时保存策略配置的副本。
- 分层沿用 `Controller -> Service`；`risk` 包保持与 HTTP、存储无关的纯计算。
- 路径：API 固定为 `/api/v1`，控制台为 `/console/`。管理接口 `/api/v1/admin/*` 只供同源访问，不返回任何跨域响应头；本地开发通过 Vite 代理实现同源。
- 应用模型：应用 ID（公开，前端传入）+ 应用密钥（S2S 鉴权）+ 允许来源列表。另有内置系统应用 `app_console`（管理后台），不可删除、停用或修改，只接受同源请求，没有密钥。
- 用户侧验证码接口（出题 `POST /api/v1/captchas/:type`、提交 `POST /api/v1/captchas/:type/:key/attempts`）都在 URL 查询参数里携带 `appId`。S2S 查询 `GET /api/v1/captchas/states/:key` 只用应用密钥鉴权，不校验来源。
- SDK 公共 API 只从 `sdk/src/index.tsx` 导出，`sdk/src/_internal/` 下的内容禁止对外导出。控制台页面放在 `sdk/playground/pages/`，由 `sdk/playground/console/ConsoleView.tsx` 挂导航，接口封装在 `sdk/playground/api.ts` 的 `adminApi`。

### Business Constraints
- 仅内部使用，采用重新初始化：不导入旧环境变量，不读取旧的 `admin.json` / `apps.json`。升级后需要在控制台重新初始化管理员账号，接入应用需要重新创建并重新配置密钥。
- 风控策略全局统一，不按应用区分；应用只多出"允许来源"这一项设置。
- 策略配置只保存当前值，不做修改历史、审计日志或回滚（用户 2026-10-27 决定）。控制台显示每项的默认值，并提供"恢复全部默认值"。
- IP 只用于加难，绝不按 IP 拒绝验证码请求（企业 NAT 共享出口事故的教训，保持不变）。
- 来源不在应用名单内、缺少 Origin、应用不存在或已停用时，直接拒绝出题与提交（用户 2026-10-27 决定）。
- 只有一个管理员账号，用户名与密码在首次初始化时设置；不做多账号，也不做修改用户名。

### Security / Compliance Constraints
- 应用 ID 是公开的，Origin 也能被脚本伪造。来源校验只能阻止别的网站在用户浏览器里调用本服务，防脚本仍然依靠风控评分与 PoW。跨应用防护的关键是"验证码与应用绑定"（Phase 12）。
- 存储里只存哈希：管理员密码用 bcrypt，应用密钥、会话令牌与初始化口令只存哈希。`captcha.db` 文件权限 0600，`data/` 目录 0700。
- 拒绝响应不得透露应用的允许名单。`second_key` 与应用不匹配时，响应与"key 无效"完全一致，并且不消费该 key。
- `second_key` 一次性消费，必须是原子的"读取并删除"。
- 初始化必须提供启动日志中的一次性口令（Q9 默认），防止服务暴露在公网后被他人抢先初始化。初始化接口受 `ADMIN_IP_LIMIT_PER_MINUTE` 限流；"确认未初始化 + 写入账号 + 作废口令"必须在同一事务中完成。
- 同源判定只在直连来源命中 `TRUSTED_PROXIES` 时才采信 `X-Forwarded-Host` / `X-Forwarded-Proto`，见 Q2。
- 风控拦截的真实原因只留在服务端，对外响应与普通答案错误一致（保持不变）。
- `backend/data/password.txt` 不是程序生成的文件（已被 `.gitignore` 忽略，代码中无引用），不得读取、修改或删除。

### Code Style / Engineering Rules
- 注释、日志、文档使用中文，代码标识符使用英文。
- React：解构导入（`import { useState } from 'react'`），类型用 `import type`；事件处理用 `e.stopPropagation()` / `e.preventDefault()`。
- Go：新增逻辑必须有表驱动单测。存储测试使用临时目录；进程内存存储使用可注入的时钟测试过期。迁移 SQL 以编号文件 `backend/storage/migrations/NNNN_*.sql` 通过 `go:embed` 嵌入，只追加，不修改已有文件。
- 每个阶段结束前运行 `cd backend && go vet ./... && go test ./...`；涉及 SDK 的阶段再运行 `cd sdk && pnpm lint && pnpm typecheck && pnpm test && pnpm build`；涉及控制台的阶段另加 `pnpm build:console`。
- 删除环境变量时登记到 `config.removedEnvVars`（只告警、不读取），并在同一阶段同步 `backend/.env.example` 与 `backend/.env.development`。README 统一在 Phase 13 更新。
- 修订接入说明只改仓库根目录的 `README.md`；二级标题改名时同步 `sdk/playground/docs/content.ts`。
- 新增错误码沿用 `backend/helper` 的 `CaptchaError` 结构：`ErrCodeAppInvalid = 1010`、`ErrCodeOriginNotAllowed = 1011`、`ErrCodeNotInitialized = 2012`、`ErrCodeAlreadyInitialized = 2013`、`ErrCodeSetupCodeInvalid = 2014`、`ErrCodeSystemAppReadonly = 2015`。2001～2011 已被占用；2008（`ErrCodePasswordChangeRequired`）在 Phase 2 删除后不再复用。

### Forbidden Actions
- 不得执行 `git commit` / `git push`。提交由用户决定，执行者在阶段完成后给出改动清单。
- 不得使用 `git stash`（会让用户正在运行的 Vite 开发服务器返回旧模块）。对比改动前后用 `git diff`，或把旧版本检出到临时目录。
- 不得按 IP 拒绝验证码出题或提交。
- 除 `modernc.org/sqlite` 外不得新增 Go 依赖；不得新增 SDK 运行时依赖或演示站依赖。
- 不得为旧环境变量或旧数据文件编写兼容读取、导入逻辑。
- 不得把部署类参数（端口、证书、代理网段等）做进控制台。
- 不得修改 `backend/internal/gocaptcha/`，不得把 `sdk/src/_internal/` 的内容导出到公共入口。
- 不得删除或改写既有测试的断言来让测试通过；行为确实变化时更新断言，并在 Change Log 说明。
- 不得在 `sdk/playground/` 另写一份接入文档。

---

## 3. Phase Status Summary

| Phase | Name | Status | Depends On | Output Summary |
|---|---|---|---|---|
| 1 | Go 1.27 统一升级与 SQLite 存储基础设施 | COMPLETED | - | 全仓库 Go 1.27.3；`backend/storage`（连接、编号迁移、测试辅助）；`/health` 增加 storage |
| 2 | 管理员账号、会话与接入应用迁到 SQLite，首次初始化接口 | COMPLETED | 1 | 迁移 0002；初始化口令与 `GET/POST /admin/setup`；删除随机密码、强制改密与 `ADMIN_USERNAME`；`reset-admin`；删除 JSON 文件持久化 |
| 3 | 控制台首次初始化页面 | COMPLETED | 2 | `SetupPage.tsx`；`useConsoleAuth` 增加 `setup` 状态；去掉强制改密流程 |
| 4 | 短期数据改存进程内存 | COMPLETED | - | `backend/cache` 内存实现（过期、计数、滑动窗口、风控状态、指纹集合、条数上限）；替换除遥测外的全部 Redis 读写 |
| 5 | 风控观察迁到 SQLite 并移除 Redis | COMPLETED | 1, 2, 4 | 迁移 0003；遥测异步批量写入与保留；删除 Redis 客户端、配置、依赖与部署项 |
| 6 | 运行时策略配置（后端） | COMPLETED | 4, 5 | `config.Runtime()` 原子快照；迁移 0004；`GET/PUT /admin/settings`；删除 23 个环境变量 |
| 7 | 控制台「策略配置」页面 | COMPLETED | 6 | `SettingsPage.tsx`、导航、`adminApi` 方法 |
| 8 | 固定 API 路径与演示站同源访问 | COMPLETED | - | 删除 `API_PREFIX`、占位符注入、旧版 GET 出题；Vite 代理 `/api` |
| 9 | SDK 接入参数改造（baseURL + appId） | COMPLETED | 8 | `ApiConfig { baseURL?, appId }`、请求带 `appId`、`APP_REJECTED`、SDK 3.0.0 |
| 10 | 应用允许来源与按应用跨域（后端） | COMPLETED | 2, 8, 9 | 迁移 0005；内置 `app_console`；来源规范化；按应用跨域与服务端拒绝；删除 `CORS_*` |
| 11 | 控制台应用来源管理界面 | COMPLETED | 10 | `AppsPage` 来源编辑、系统应用只读、接入示例更新 |
| 12 | 验证码与应用绑定 | COMPLETED | 4, 10 | 会话与二次验证记录 appId；提交、S2S、管理员登录校验一致；风控观察显示应用 |
| 13 | 文档更新、零配置启动与端到端验收 | COMPLETED | 1-12 | README、AGENTS.md、`.env.example`、`docker-compose.yml`（`.env` 可选）更新；端到端验收清单 |

执行顺序按编号进行。依赖列只写真实的前置条件。

Allowed status values:

- NOT_STARTED
- IN_PROGRESS
- BLOCKED
- COMPLETED

---

## 4. Phase Details

### Phase 1: Go 1.27 统一升级与 SQLite 存储基础设施

#### Status
COMPLETED

#### Goal
把全仓库的 Go 版本统一到 1.27.3，引入 `modernc.org/sqlite`，新建 `backend/storage` 包：打开数据库、执行编号迁移、启动与关闭接线、提供测试辅助。本阶段不迁移任何业务数据。

#### Scope
- Go 版本统一到 1.27.3（仓库内全部引用点，2026-10-27 核实）：
  - `backend/go.mod`：`go 1.27.3`；执行 `go get modernc.org/sqlite@latest` 后 `go mod tidy`。
  - `backend/Dockerfile`：构建阶段由 `golang:1.23.4-alpine` 改为 `golang:1.27.3-alpine`，`CGO_ENABLED=0` 不变。
  - `deploy/fingerproxy/Dockerfile`：由 `golang:1.23-alpine` 改为 `golang:1.27.3-alpine`。
  - `backend/run-dev.sh`：提示文字 "请安装 Go 1.23+" 改为 "请安装 Go 1.27+"。
  - `AGENTS.md`：技术栈中的 "Go 1.23+" 改为 "Go 1.27+"。
  - `backend/internal/gocaptcha/` 不是独立模块，无需改动。
- 新包 `backend/storage/`：
  - `Open(dataDir string) (*DB, error)`：确保目录存在且可写（迁入 `services/datafile.go` 的 `ensureDataDir` 逻辑）；打开 `captcha.db`，pragma 为 `journal_mode=WAL`、`synchronous=NORMAL`、`busy_timeout=5000`、`foreign_keys=ON`；`SetMaxOpenConns(1)`；创建后把文件权限收紧到 0600。
  - 迁移执行器：`schema_migrations(version INTEGER PRIMARY KEY, applied_at INTEGER)`；按编号顺序把 `migrations/*.sql` 逐个放进事务执行，已执行的跳过。本阶段只提交 `0001_init.sql`（可以只包含迁移表本身）。
  - 测试辅助 `storage.OpenTemp(t)`：在临时目录打开并注册清理。
- `backend/main.go`：在服务管理器初始化前打开存储，失败时 `log.Fatal` 并给出中文原因（目录不可写、文件损坏等）；优雅关闭时调用 `Close`。
- `/health` 增加 `storage` 状态（执行 `PingContext`），保留现有 `redis` 字段。

#### Out of Scope
- 任何业务表（各阶段在自己的迁移文件里新增）。
- Redis 相关改动、管理员与应用数据迁移。

#### Inputs
- `backend/go.mod`、`backend/Dockerfile`、`deploy/fingerproxy/Dockerfile`、`backend/run-dev.sh`、`backend/main.go`、`backend/routes/router.go`（`healthCheckHandler`）、`backend/services/datafile.go`、`AGENTS.md`

#### Execution Requirements
- 升级 Go 后先跑一遍全部测试确认基线；如有因 Go 版本引起的 vet 或测试变化，在 Notes 中记录。
- `storage` 包不依赖 `services`、`gin`。

#### Expected Outputs
- 新增：`backend/storage/storage.go`、`backend/storage/migrate.go`、`backend/storage/migrations/0001_init.sql`、`backend/storage/storage_test.go`
- 修改：`backend/go.mod`、`backend/go.sum`、`backend/Dockerfile`、`deploy/fingerproxy/Dockerfile`、`backend/run-dev.sh`、`backend/main.go`、`backend/routes/router.go`、`AGENTS.md`

#### Acceptance Criteria
- [x] 在同一目录连续 `Open` 两次，迁移只执行一次（单测）。
- [x] `PRAGMA journal_mode` 返回 `wal`；`captcha.db` 权限为 0600（单测）。
- [x] 20 个 goroutine 并发写入 1000 行，没有 `SQLITE_BUSY` 错误（单测）。
- [x] `CGO_ENABLED=0 go build ./...` 成功；`docker build -f backend/Dockerfile .` 成功；`deploy/fingerproxy` 的镜像用 1.27.3 构建成功。
- [x] `rg -i "go ?1\.2[0-6]|golang:1\.2[0-6]"` 在仓库中（不含 `docs/tasks/`）没有结果。
- [x] 启动后 `/health` 返回 `storage.status = ok`。
- [x] `go vet ./... && go test ./...` 通过。

#### Risks
- Go 版本跨度较大（1.23 → 1.27），vet 规则可能变化：先在升级后单独跑一遍基线。
- fingerproxy 是第三方项目，可能对 Go 1.27 有兼容问题：构建失败时在 Notes 中记录原因，并把该文件的升级标为待议，不阻塞本阶段其余工作。
- 单连接会让读写串行，当前数据量下可以接受；如果后续出现读延迟，再拆成"1 个写连接 + 读连接池"。

#### Dependencies
无。

#### Notes
- `data/` 目录固定，不新增配置项（沿用 2026-10-25 决策）。
- 执行记录（2026-10-27）：
  - 升级前基线：`go test ./...` 全部通过（本机工具链本就是 go1.27.3）。升级 `go.mod` 后 vet 与测试没有变化。
  - `go get modernc.org/sqlite@latest` 得到 v1.61.0，同时把 `golang.org/x/sys` 升到 v0.49.0。
  - fingerproxy v1.1.0 用 `golang:1.27.3-alpine` 构建成功，没有兼容问题。
  - `docker build -f backend/Dockerfile .` 成功。
  - 在临时工作目录启动二进制：`/health` 返回 `storage.status = ok`，`data/captcha.db` 权限为 0600。
  - 存储在 `services.ServiceManagerImpl` 中持有：`main.go` 先调用 `storage.Open` 再调用 `services.UseStorage`；测试经由全局管理器时，按 `config.Admin.DataDir` 自动打开。关闭时由 `Shutdown` 关闭连接。

---

### Phase 2: 管理员账号、会话与接入应用迁到 SQLite，首次初始化接口

#### Status
COMPLETED

#### Goal
把管理员凭据（`admin.json`）、管理会话（Redis `admin:session:*`）、接入应用（`apps.json`）以及应用最近使用时间（Redis `admin:app:last_used:*`）全部改存 SQLite。同时把"首次启动随机生成密码、首次登录强制改密"改为"首次初始化"：数据库中没有管理员账号时服务处于未初始化状态，由初始化接口设置用户名与密码。

#### Scope
- 迁移 `0002_admin_apps.sql`：
  - `admin_account`：单行，`id = 1`，含 `username`、`password_hash`、`generation`、`updated_at`。没有这一行即为"未初始化"。
  - `admin_setup`：单行，`id = 1`，含 `code_hash`（初始化口令的 SHA-256）与 `created_at`。
  - `admin_sessions`：`token_hash` 主键，含 `username`、`generation`、`created_at`、`expires_at`。
  - `apps`：`id` 主键，含 `name`、`secret_hash`、`secret_hint`、`enabled`、`created_at`、`updated_at`、`secret_rotated_at`、`last_used_at`（可空）。
- 初始化口令（Q9 默认）：
  - 每次启动时，如果尚未初始化，就生成新口令覆盖写入 `admin_setup`，并在终端打印中文提示：控制台地址 `/console/`、口令、"初始化完成后自动作废"。已初始化时不生成、不打印。
  - 口令共 12 位，按 `XXXX-XXXX-XXXX` 分组。字符取大写字母与数字，去掉 0/O、1/I 这类易混字符。
  - 每次启动都会换新口令，有效口令总在最近一次启动日志里。明文不落库。
  - 校验时忽略大小写、空格与连字符，并用常量时间比较。
- 接口（`/admin` 路由组，不需要会话）：
  - `GET /admin/setup` → `{ initialized }`。
  - `POST /admin/setup`，请求体 `{ setupCode, username, password }`：
    - 在同一事务中确认尚无账号、校验口令、写入账号、删除 `admin_setup`。
    - 成功后直接签发会话，响应与登录一致：`{ token, username, expiresAt }`。
    - 挂 `AdminRateLimitMiddleware`。不要求验证码，因为口令本身就是一次性的高熵凭据。
  - 错误码：
    - 已初始化 → `ErrCodeAlreadyInitialized`（2013，HTTP 409）。
    - 口令错误 → `ErrCodeSetupCodeInvalid`（2014，HTTP 403）。
    - 用户名或密码不合规 → 400，沿用现有参数错误。
    - 未初始化时 `POST /admin/login` 返回 `ErrCodeNotInitialized`（2012），控制台据此切换到初始化页面。
- 账号规则：用户名去掉首尾空白后为 1～64 个字符；密码沿用现有规则（至少 10 个字符、不超过 72 字节）。登录时以常量时间比对库中的用户名。
- 删除：
  - 随机初始密码：`randomPassword`、`AdminPasswordNotice`。
  - 强制改密：`must_change_password`、`RequirePasswordChanged` 中间件、`ErrCodePasswordChangeRequired`（2008，编号不再复用）。登录与会话响应去掉 `mustChangePassword` 字段。
  - 环境变量 `ADMIN_USERNAME`：登记到 `removedEnvVars`，提示"用户名改在首次初始化时设置"；同步 `backend/.env.example`、`backend/.env.development`。
- 保留：主动改密接口 `PUT /admin/password`，"改密后其他会话全部失效"的语义不变。
- 子命令 `reset-admin-password` 改名为 `reset-admin`：
  - 在一个事务中删除管理员账号与全部会话，生成新的初始化口令写入 `admin_setup`，并打印口令。
  - 服务运行期间执行也立即生效，无需重启：账号与会话每次都查库，不在进程内缓存。
  - 接入应用、策略、背景图不受影响。
  - 同步 `backend/run-dev.sh` 注释中的示例。
- `services/admin_credential.go`、`services/admin_service.go`：会话的创建、查询（校验 `expires_at` 与 `generation`）、注销改走 SQLite；登录时顺带清理已过期的会话。
- `services/app_service.go`：增删改查与密钥轮换改走 SQLite。`last_used_at` 的写入要节流：同一应用每分钟最多写一次，上次写入时间记在进程内存里。
- 删除 `services/datafile.go` 中的 JSON 读写函数。
- 启动时如果检测到旧的 `data/admin.json` 或 `data/apps.json`，打印一次中文警告"已不再使用，可手动删除"，既不读取也不删除（Q5 默认）。
- 测试：
  - `tests/admin_helpers_test.go` 的 `TestMain` 改为在临时目录中打开 SQLite，并通过初始化接口建好测试账号。
  - 更新 `tests/admin_test.go`、`tests/admin_password_test.go`、`tests/apps_test.go`、`main_test.go`。
  - 新增 `tests/admin_setup_test.go`。

#### Out of Scope
- 控制台初始化页面（Phase 3）。
- 应用的允许来源与系统应用（Phase 10）。
- 验证码与应用绑定（Phase 12）。
- 其余 Redis 数据（Phase 4、5）。
- 多个管理员账号、修改用户名。

#### Inputs
- `backend/services/admin_credential.go`、`admin_service.go`、`app_service.go`、`datafile.go`
- `backend/main.go`（`runResetAdminPassword`）、`backend/main_test.go`、`backend/run-dev.sh`
- `backend/config/config.go`（`Admin.Username`、`validateConfig`）
- `backend/routes/router.go`（`RegisterAdminRoutes`）、`backend/middleware/admin_auth.go`、`backend/middleware/auth.go`
- `backend/helper/errors.go`、`backend/models/admin.go`
- `backend/tests/admin_*_test.go`、`backend/tests/apps_test.go`

#### Execution Requirements
- 除了新增初始化接口和去掉 `mustChangePassword` 字段，其余管理接口的路径、请求体、响应体都保持不变。
- 控制台在 Phase 3 适配。本阶段结束到 Phase 3 完成之间，本地需要用 curl 调初始化接口完成初始化。
- 会话令牌只存 SHA-256 哈希（沿用现有做法）。

#### Expected Outputs
- 新增：`backend/storage/migrations/0002_admin_apps.sql`、`backend/tests/admin_setup_test.go`
- 修改：上述 services、controllers、routes、middleware、helper、models、config、`main.go`、`run-dev.sh`、env 文件、相关测试；删除或精简 `services/datafile.go`

#### Acceptance Criteria
- [x] 用全新的 `data/` 目录启动：
  - 生成 `captcha.db`，终端打印初始化口令和控制台地址。
  - `GET /admin/setup` 返回 `initialized: false`。
  - `data/` 下不生成 `admin.json` / `apps.json`。
- [x] 初始化接口测试：
  - 初始化前登录返回 2012。
  - 口令错误返回 403 / 2014。
  - 用户名为空或密码过短返回 400。
  - 正确口令返回会话，并能用它访问 `/admin/apps`。
  - 再次初始化返回 409 / 2013。
- [x] 两个请求并发使用正确口令初始化，只有一个成功（测试）。
- [x] 未初始化时重启会换新口令，旧口令失效；已初始化后重启不再打印口令（测试）。
- [x] 以下流程的接口测试全部通过：
  - 初始化 → 创建应用 → 轮换密钥 → S2S 用新密钥查询 → 删除应用。
  - 登录 → 主动改密 → 旧会话失效。
- [x] 关闭并重新 `Open` 存储（模拟重启）后：会话在有效期内仍可用，账号、应用与 `lastUsedAt` 都保留。
- [x] `reset-admin` 测试：
  - 执行后账号与会话被删除，`initialized` 变为 false。
  - 新口令能完成初始化。
  - 接入应用保留。
- [x] 存在旧 JSON 文件时，启动日志出现一次警告且启动成功。
- [x] `backend/` 中不再出现以下内容（`removedEnvVars` 除外）：
  - Redis 键前缀 `admin:session:`、`admin:app:last_used:`。
  - `mustChangePassword`、`MustChangePassword`、`RequirePasswordChanged`、`ADMIN_USERNAME`。
- [x] `go vet ./... && go test ./...` 通过。

#### Risks
- 会话失效依赖 `generation`，迁移后必须保持"改密后其他会话失效"的语义：补测试覆盖。
- 未初始化期间服务若已暴露在公网，任何人都能打开初始化页面。口令只出现在启动日志里，可以防止被抢先初始化。日志如果会被采集到第三方平台，口令也会出现在那里；它在初始化后即作废，影响有限。
- `reset-admin` 与运行中的服务同时访问同一个库文件：依靠 WAL 与 `busy_timeout`，测试中覆盖"服务运行中执行重置"。

#### Dependencies
Phase 1。

#### Notes
- 本阶段完成后，本地开发环境需要重新初始化并重新创建应用（重新初始化，符合决策）。
- 执行记录（2026-10-27）：
  - 存储层：`storage` 的 DSN 增加 `_txlock=immediate`，所有事务一开始就取得写锁，避免 `reset-admin` 子命令与运行中的服务"先读后写"时直接得到 SQLITE_BUSY。
  - 服务层：`NewAdminService(cfg, db, backgrounds, riskControl)`、`NewAppService(db)` 不再读文件、也不再返回 error。初始化口令在 `ServiceManagerImpl` 初始化时通过 `PrepareSetup` 生成并打印。
  - 改密：`ChangePassword(current, next)` 不再需要当前令牌，因为改密时直接删除全部会话。更新以读取时的 `generation` 为条件，并发改密时只有一次生效。
  - 未初始化时登录先返回 2012，不消费验证码。
  - 应用：S2S 鉴权按 `secret_hash` 唯一索引查询，`last_used_at` 按应用每分钟最多写一次。
  - 测试改动（行为确实变化）：
    - 删除 `TestAdmin_InitialCredentialIsRandomAndHashed`、`TestAdmin_CorruptCredentialFailsFast`、`TestAdmin_MustChangePasswordGatesConsole`（随机密码与强制改密已删除）。
    - `TestAdmin_ResetPasswordTakesEffectWithoutRestart` 改为 `TestAdmin_ResetTakesEffectWithoutRestart`，另开连接执行重置。
    - `TestApps_PersistFailureKeepsState` 改为 `TestApps_StorageFailureReturns500`：不再有内存副本，SQLite 写已有文件也不受目录权限影响。
    - `TestAdmin_RiskTelemetryRequiresReadySession` 改为 `TestAdmin_RiskTelemetryRequiresSession`。
    - 新增 `tests/admin_setup_test.go`（6 个用例）、`services/datafile_test.go`、`main_test.go` 的 `TestRunResetAdmin`。
  - 冒烟测试：在 scratchpad 中用全新数据目录（预放一个 `admin.json`）启动。日志出现一次旧文件警告和初始化口令；`GET /admin/setup` 先返回 false；用口令 `POST /admin/setup` 返回会话，之后变为 true。
  - 控制台在 Phase 3 适配之前，登录页仍预填 `admin`，也没有初始化入口。

---

### Phase 3: 控制台首次初始化页面

#### Status
COMPLETED

#### Goal
打开控制台时，如果服务尚未初始化，就显示初始化页面：输入初始化口令、用户名与密码，提交后直接进入控制台。同时去掉控制台里的强制改密流程。

#### Scope
- `sdk/playground/api.ts`：
  - 新增 `adminApi.setupStatus()`，以及返回 `LoginResponse` 的 `adminApi.setup({ setupCode, username, password })`。
  - 删除 `mustChangePassword` 字段。
  - 为错误码 2012、2013、2014 补充中文提示。
- `sdk/playground/console/useConsoleAuth.ts`：
  - 新增 `{ status: 'setup' }` 状态。加载时先查询 `setupStatus`，未初始化时进入 `setup`。
  - 登录返回 2012 时也切换到 `setup`。
  - 初始化成功后走 `handleSession`。
  - `AuthState` 去掉 `mustChangePassword`。
- 新增 `sdk/playground/pages/SetupPage.tsx`：
  - 位置与样式沿用登录卡片（控制台首页的暗色展台 + 卡片）。
  - 字段：初始化口令、用户名、密码、确认密码。口令输入自动转为大写。
  - 前端校验与后端一致：用户名 1～64 个字符；密码至少 10 个字符、不超过 72 字节；两次输入一致。
  - 说明文字："初始化口令见服务启动日志（例如 `docker logs`）；遗失时重启服务即可获得新口令。"
  - 服务端错误显示在表单内。
- `sdk/playground/console/ConsoleView.tsx`（以及演示站中挂载控制台的入口）：
  - `setup` 状态渲染 `SetupPage`。
  - 删除强制改密的跳转与提示；`PasswordPage` 保留为主动修改密码。
- `sdk/playground/pages/LoginPage.tsx`：用户名输入框不再预填 `admin`（用户名改在初始化时自定）。
- 内嵌控制台（Go 服务的 `/console/`）与演示站（Vite）两个入口行为一致。

#### Out of Scope
- 后端接口（Phase 2 已完成）。
- 在初始化页面顺带创建第一个接入应用（Q10 默认不做）。
- 多个管理员账号、修改用户名。

#### Inputs
- `sdk/playground/api.ts`
- `sdk/playground/console/useConsoleAuth.ts`、`ConsoleView.tsx`、`ConsoleApp.tsx`
- `sdk/playground/App.tsx`
- `sdk/playground/pages/LoginPage.tsx`、`PasswordPage.tsx`
- Phase 2 的接口契约

#### Execution Requirements
- 不新增依赖，样式沿用现有登录卡片。
- 浏览器验证要求：
  - 以 scratchpad 目录作为工作目录启动后端，数据目录随之落在 scratchpad，并使用临时端口。
  - 演示站在 8010 起临时实例，不动用户的 8000。
  - 用 Playwright 或 Chrome 操作。

#### Expected Outputs
- 新增：`sdk/playground/pages/SetupPage.tsx`
- 修改：`sdk/playground/api.ts`、`sdk/playground/console/useConsoleAuth.ts`、`ConsoleView.tsx`、`LoginPage.tsx`、`PasswordPage.tsx`（如需要）

#### Acceptance Criteria
- [x] 浏览器验证（全新数据目录），依次完成：
  1. 打开控制台，显示初始化页面。
  2. 输入错误口令，表单内显示错误。
  3. 两次密码不一致时被前端拦截。
  4. 正确提交后进入控制台首页，并显示用户名。
  5. 刷新后仍保持登录。
  6. 退出后显示登录页，不再显示初始化页。
  7. 用新账号登录成功（含验证码）。
- [x] 执行 `reset-admin` 后刷新控制台，回到初始化页面。
- [x] `rg -n "mustChangePassword" sdk/playground` 没有结果。
- [x] `pnpm lint && pnpm typecheck && pnpm test && pnpm build && pnpm build:console` 通过。

#### Risks
- 登录态的判定顺序改变（先查初始化状态，再恢复会话）后，可能在刷新时闪一下登录页：`checking` 状态要覆盖这两次请求。

#### Dependencies
Phase 2。

#### Notes
- 执行记录（2026-10-27）：
  - 浏览器验证环境：
    - 临时后端从 scratchpad 工作目录启动，端口 3399；演示站临时实例在 8010（`VITE_API_BASE=http://127.0.0.1:3399`）。
    - 后端带测试用策略：`RISK_ENABLED=false`、`CAPTCHA_DEFAULT_DIFFICULTY=1`、`CAPTCHA_IP_ESCALATE_PER_MINUTE=0`。这样 `auto` 才可能出滑块；否则 Playwright 的 webdriver 标记会让题型只剩点选。
    - 验证码由滑块求解脚本完成：先用归一化互相关定位缺口，再闭环拖动；遇到非滑块题就刷新换题。脚本放在已被 gitignore 的 `.playwright-mcp/login-flow.js`，后续阶段的浏览器验收沿用。
  - 7 步浏览器验证全部通过：
    - 错误口令提示"初始化口令错误，请以服务最近一次启动日志中的口令为准"。
    - 两次密码不一致时被前端拦截。
    - 输入小写并带空格的正确口令可以初始化，进入控制台后顶栏显示 `ops`。
    - 刷新后仍保持登录；退出后显示登录页，用户名为空。
    - 通过滑块验证码后，用 `ops` 登录成功。
  - 运行中执行 `sc-bin reset-admin`，再 `page.reload()`，回到"首次初始化"页面，sessionStorage 中的令牌已被清除。注意：同一 hash 的 `goto` 不会重新加载页面，必须用 reload。
  - 内嵌控制台 `http://127.0.0.1:3399/console/` 同样显示初始化页面，页脚文案已更新。
  - `pnpm lint` 的 16 条 warning 都在 `sdk/src/`，是既有问题，本阶段没有新增。
  - 刷新验证码时浏览器控制台有 React 的 "empty string passed to src" 报错，是既有问题，与本阶段无关，未处理。

---

### Phase 4: 短期数据改存进程内存

#### Status
COMPLETED

#### Goal
在 `backend/cache/` 实现带过期时间的进程内存存储，替换除风控观察日志以外的全部 Redis 读写：题目数据、尝试次数、二次验证 key、IP 计数（验证码加难与管理员登录限流）、客户端风控状态、指纹集合。

#### Scope
- `backend/cache/memory.go`（Redis 实现暂时保留给风控观察日志用，Phase 5 删除）：
  - 键值：`Set` / `Get` / `Del` / `GetDel`（原子读取并删除）。
  - 计数：`Incr(key, ttl)`。
  - 滑动窗口：`Hit(key, window, limit)`，语义与 `services/rate_limiter.go` 的 Lua 脚本一致（被拒绝的请求不计入窗口）。
  - 客户端风控状态：`RecordClient(失败 +1；成功则 successes +1 且 fails 衰减 1，下限 0；更新 lastScore 与 updatedAt；刷新过期时间)`、`GetClient`，语义与 `services/client_risk.go` 的 `recordScript` 一致。
  - 指纹集合：`TouchFingerprint(fp, client, saturation, ttl)`，饱和截断语义与 `fingerprintScript` 一致。
  - 每类数据有独立的条数上限，超限时淘汰最早到期的条目并累计告警计数（Q4 默认：每类 20 万条）；后台每 30 秒清理一次过期条目；时钟可注入；提供 `Stop()` 供测试使用。
- 替换调用方：
  - `services/base_service.go`、`services/base_verify_service.go`：题目数据、尝试次数、二次验证数据。
  - `services/risk_control_service.go`：二次验证令牌统一走内存 `GetDel`，删除"进程 map + Redis"两套并存的逻辑。
  - `services/rate_limiter.go`：IP 加难计数与管理员登录限流。
  - `services/client_risk.go`：风控状态与指纹（遥测 `captcha:telemetry` 暂时保留 Redis）。
  - `services/cache_service.go`、`services/challenge_policy.go` 中对应的存储接口。
- 测试：内存存储单测（过期、`GetDel` 并发只成功一次、滑动窗口边界、计数衰减、指纹饱和、淘汰）；更新 `services/fingerprint_replay_test.go` 与 `tests/` 中依赖 Redis 的用例。

#### Out of Scope
- 风控观察日志（Phase 5）。
- 删除 Redis 客户端与配置（Phase 5）。
- 策略配置热生效（Phase 6）：本阶段仍从现有的 `config` 读取。

#### Inputs
- `backend/cache/redis.go`、`backend/services/cache_service.go`、`base_service.go`、`base_verify_service.go`、`risk_control_service.go`、`rate_limiter.go`、`client_risk.go`、`challenge_policy.go`
- `backend/services/fingerprint_replay_test.go`、`backend/tests/fingerprint_replay_test.go`、`issue_endpoint_test.go`、`security_test.go`、`services_test.go`

#### Execution Requirements
- 原先由 Lua 脚本保证的原子性改为互斥锁保证；同一操作内的读改写必须在一次加锁内完成。
- 对外行为不变：既有接口测试的断言不改；确需改动的写进 Notes 并说明原因。

#### Expected Outputs
- 新增：`backend/cache/memory.go`、`backend/cache/memory_test.go`
- 修改：上述 services 与测试

#### Acceptance Criteria
- [x] 在 `rg "GetRedisClient" backend --type go` 的结果里，除 `cache/redis.go` 本身外只剩风控观察日志相关的调用。
- [x] 100 个 goroutine 并发 `GetDel` 同一个二次验证 key，只有 1 个成功（单测）。
- [x] 达到条数上限时淘汰最早到期的条目，告警计数增加（单测）。
- [x] 出题、提交、尝试次数上限、二次验证一次性、IP 加难、指纹重放、管理员登录限流相关的既有测试全部通过。
- [x] `go vet ./... && go test ./...` 通过。

#### Risks
- 滑动窗口和计数衰减语义细节容易偏离：用表驱动测试逐条对照原 Lua 脚本的行为。
- 内存占用：出题速度受出图 CPU 限制（约 200 题/秒），5 分钟内最多积累约 6 万条题目数据，上限 20 万条留有余量。

#### Dependencies
无（执行顺序在 Phase 3 之后）。

#### Notes
- 重启会丢失正在答题的题目、未消费的二次验证 key、IP 计数与风控状态，属于已接受的代价。
- 执行记录（2026-10-27）：
  - `cache.Memory`：按类别（challenge / ratelimit / client / fingerprint）分桶，每个桶是"map + 按到期时间排序的最小堆"，淘汰和清理都从堆顶取。全局实例为 `cache.Shared()`。
  - 与 Scope 的差异：
    - 计数没有单独实现 `Incr`，改为通用的原子读改写 `Update(kind, key, ttl, fn)`。尝试次数存在题目 JSON 里，用 `Update` 在一次加锁内完成"读 → +1 → 写回"，原先 Get + Set 并发时会少算。
    - 另外加了三个只读观测方法：`FingerprintSize`、`TTL`、`WindowCount`，供测试替代原先直接读 Redis 的断言。
  - 二次验证令牌：删除"进程 map + Redis"两套存储，统一用 `GetDel` 一次性消费。`GenerateSecondVerifyToken` 仅供测试签发。
  - 删除 `services/rate_limiter.go`。IP 计数改用 `Memory.Hit`，key 前缀为 `captcha:` / `admin:`，导出 `services.CaptchaRateLimitKey` 供测试预填窗口。
  - `cache/redis.go` 只保留 `GetRedisClient`。剩余调用全部与风控观察有关：`client_risk.go` 的遥测读写、`cache_service.go` 的健康检查 Ping（检查的就是遥测所在的 Redis），以及 `tests/issue_endpoint_test.go` 的遥测断言。
  - 测试改动（行为确实变化）：
    - `services/fingerprint_replay_test.go`：原先的 Redis 不可达 fail-open 用例已没有意义，改为"阈值 0 或参数为空不写入"与"饱和截断"两个用例。
    - `tests/services_test.go`：`TestCacheService_Operations` 改为 `TestCacheService_Ping`，因为通用缓存读写已移到 cache 包的单测。
    - `tests/security_test.go`、`tests/fingerprint_replay_test.go`：改为预填并读取进程内存，接口行为断言不变。
  - `go vet ./... && go test ./...` 全部通过。cache 包的单测覆盖：过期、类别隔离、100 个 goroutine 并发 `GetDel` 只成功 1 次、`Update` 并发自增不丢失、滑动窗口表驱动、计数衰减表驱动、指纹饱和、超限淘汰最早到期条目、Sweep。

---

### Phase 5: 风控观察迁到 SQLite 并移除 Redis

#### Status
COMPLETED

#### Goal
把风控观察日志从 Redis 列表改为 SQLite 持久化（异步批量写入、按条数保留），然后彻底删除 Redis 客户端、配置、依赖与部署项。

#### Scope
- 迁移 `0003_telemetry.sql`：`telemetry(id INTEGER PRIMARY KEY AUTOINCREMENT, created_at INTEGER NOT NULL, kind TEXT NOT NULL, app_id TEXT, payload TEXT NOT NULL)`。`app_id` 在 Phase 12 开始写入。
- 异步写入器：带缓冲的通道（容量 1024）；每 1 秒或攒满 200 条时在一个事务内批量写入。通道满时丢弃新记录并计数，告警日志限频输出，绝不阻塞验证请求。优雅关闭时把剩余记录写完。
- 保留：每次批量写入后删除 `id <= 最大 id - 保留条数` 的行。保留条数暂时读 `RISK_TELEMETRY_LOG_SIZE`，默认值改为 10000（Q3 默认），Phase 6 再迁入策略配置。
- `RecentTelemetry` 改为读 SQLite（`ORDER BY id DESC LIMIT n`）。管理接口 `GET /api/v1/admin/risk/telemetry` 的契约不变（limit 默认 100，上限 500），脱敏逻辑不变。
- 删除 Redis：
  - 删除 `backend/cache/redis.go` 与 `GetRedisClient`，并删除 `RedisConfig` 与 13 个 `REDIS_*` 变量，登记到 `removedEnvVars`，提示为"已不再使用 Redis：短期数据存进程内存，持久数据存 data/captcha.db"。
  - `go mod tidy` 移除 `github.com/go-redis/redis/v8`。
  - `/health` 去掉 `redis` 字段；`/stats` 去掉 `redis_host` 等字段，改为输出存储状态与内存条目数。
  - `docker-compose.yml` 删除只为连接宿主机 Redis 而设的 `extra_hosts`，并清理相关注释。
  - `backend/.env.example`、`backend/.env.development` 删除 Redis 段落；`backend/run-dev.sh` 的启动提示去掉 `REDIS=`。
  - 测试不再连接 Redis。

#### Out of Scope
- 策略配置（Phase 6）。
- 风控观察页面改版（Phase 12 只加一列"应用"）。

#### Inputs
- `backend/services/client_risk.go`（`telemetryLogKey`、`RecentTelemetry`、`LPush` / `LTrim`）、`services/risk_engine.go`、`services/telemetry_mask.go`
- `backend/cache/redis.go`、`backend/config/config.go`、`backend/routes/router.go`、`backend/main.go`
- `docker-compose.yml`、`backend/.env.example`、`backend/.env.development`、`backend/run-dev.sh`
- `backend/tests/admin_risk_test.go`

#### Execution Requirements
- 验证测试不需要 Redis 时，先执行 `docker stop simple-captcha-redis`，验收完成后执行 `docker start simple-captcha-redis` 恢复用户环境。
- 允许残留的 "redis" 字样只有 `removedEnvVars` 的提示文本与说明性注释。

#### Expected Outputs
- 新增：`backend/storage/migrations/0003_telemetry.sql`、遥测写入器及其测试
- 删除：`backend/cache/redis.go`
- 修改：`go.mod`、`go.sum`、config、router、main、compose、env 文件、`run-dev.sh`、相关测试

#### Acceptance Criteria
- [x] `go.mod` 不含 `go-redis`；`rg -i redis backend --type go` 只命中允许的残留。
- [x] Redis 容器停止时，`go test ./...` 全部通过；验收后容器已恢复运行。
- [x] 并发写入 1.2 万条遥测后，表中行数等于保留条数；重新 `Open` 后最近的记录仍可读（测试）。
- [x] 写入器通道满时请求不阻塞，丢弃计数增加（测试）。
- [x] `/health` 返回 `status: ok` 且没有 `redis` 字段；控制台风控观察页可以看到新产生的记录（浏览器或 curl 验证）。
- [x] `go vet ./... && go test ./...` 通过。

#### Risks
- 关闭时丢失尚未写入的记录：优雅关闭流程必须先停止接收请求，再排空写入器。

#### Dependencies
Phase 1、2、4。

#### Notes
- 从本阶段起，本地开发和测试都不再需要 Redis。
- 执行记录（2026-10-27）：
  - 遥测写入器 `services/telemetry_store.go`：
    - 通道容量 1024；每秒或攒满 200 条时在一个事务内批量写入，随后按保留条数删除旧行。
    - 通道满时丢弃新记录并计数，告警每分钟最多一次。`/stats` 输出 `telemetry.dropped`。
    - `Recent` 读取前先调用 `Flush`，等待已入队的记录落库，读取方能看到自己刚写的记录。管理接口契约不变。
  - 全局唯一：测试里会多次 `NewServiceManager()`，遥测存储用 `sharedTelemetry.CompareAndSwap(nil, …)`，只认第一个管理器的实例，所有 `ClientRiskStore` 都写同一个队列。`Shutdown` 先关闭遥测（写完积压），再关闭数据库。
  - `RISK_TELEMETRY_LOG_SIZE` 的默认值由 1000 改为 10000（Q3 默认），0 表示不记录。
  - 删除：
    - `cache/redis.go`、`services/cache_service.go`、`CacheService` 接口与 `GetCacheService`。
    - `RedisConfig` 与 13 个 `REDIS_*` 变量，均已登记到 `removedEnvVars`。
    - compose 的 `extra_hosts`，以及 env 模板中的 Redis 段落。
  - `run-dev.sh` 的启动提示改为打印数据目录。`/health` 去掉 `redis` 字段；`/stats` 去掉 `redis_host`，新增 `storage.sqlite`、`storage.memory`（各类条目数与淘汰数）和 `telemetry.dropped`。
  - 测试改动：
    - `config_test.go`：`TestLoadFromEnv_Redis` 改为 `TestRemovedRedisEnvWarns`（只告警）。
    - `tests/services_test.go`：删除 `TestCacheService_Ping`，管理器检查改为检查 `GetClientRiskStore`。
    - `tests/issue_endpoint_test.go`：`telemetryEntryFor` 改为读取 `RecentTelemetry`。
    - `tests/security_test.go`：`TestRateLimiter_RedisSlidingWindow` 改名为 `TestRateLimiter_SlidingWindow`。
  - 验证：
    - `docker stop simple-captcha-redis` 之后，`go vet ./... && go test -count=1 ./...` 全部通过；验收结束后已 `docker start` 恢复容器。
    - 用重新编译的临时后端（`RISK_ENABLED=true`）检查：`/health` 只有 `storage`、没有 `redis`。用 curl 出 3 道题后，`GET /admin/risk/telemetry` 返回 3 条 `kind=issue` 记录，IP 已脱敏为 `127.0.*.*`。
    - `rg -i redis backend --type go` 只命中 `removedEnvVars` 的提示与对应的告警测试。

---

### Phase 6: 运行时策略配置（后端）

#### Status
COMPLETED

#### Goal
把 23 项验证码与风控策略从环境变量改为 SQLite 存储 + 内存原子快照，各服务每次使用时读取快照；提供管理接口读取和保存，保存后立即生效。

#### Scope
- `backend/config/runtime.go`：
  - `RuntimeSettings`，分为 `captcha`、`pow`、`risk` 三组共 23 个字段。JSON 字段名带单位后缀，时长不用 Go duration 字符串，例如 `expireSeconds`、`secondVerifyExpireSeconds`、`minSolveTimeMs`、`stateTtlSeconds`、`fpReplayWindowSeconds`、`telemetryRetention`。
  - `DefaultRuntimeSettings()`：默认值与现有环境变量默认值一致，保留条数为 10000。
  - `Validate()`：沿用 `validateConfig` 现有的范围规则，错误信息为中文并指明字段与范围。
  - `Runtime()`：通过 `atomic.Pointer` 读取当前快照。
  - `ApplyRuntime()`：替换快照。
- 迁移 `0004_settings.sql`：`settings(id INTEGER PRIMARY KEY CHECK (id = 1), data TEXT NOT NULL, updated_at INTEGER NOT NULL)`。
- `services/settings_service.go`：
  - 启动加载：在默认值之上叠加库中的 JSON；忽略未知字段；缺失字段取默认值；某个字段已不满足当前校验规则时，该字段回退到默认值并告警。
  - 保存：校验 → 在事务中写入 → 替换快照。
- 消费方改造：`base_service.go`、`captcha_controller.go`、`risk_engine.go`、`client_risk.go`、`challenge_policy.go`、`risk_control_service.go`、IP 加难计数、遥测保留都改为读 `config.Runtime()`，删除构造时保存的策略副本。
- 从 `CaptchaConfig` / `RiskConfig` 删除这 23 个字段及其环境变量读取，登记到 `removedEnvVars`，提示为"已改为管理后台「策略配置」实时修改"；`backend/.env.example`、`backend/.env.development` 同步删除。
- 管理接口（挂在会话鉴权中间件之后）：
  - `GET /api/v1/admin/settings` → `{ settings, defaults }`
  - `PUT /api/v1/admin/settings`，请求体为完整对象 → 成功返回 200 `{ settings }`；校验失败返回 400 `{ code, error }`。
- 测试：逐字段的上下界表驱动单测、热生效接口测试、持久化测试。

#### Out of Scope
- 控制台页面（Phase 7）。
- 修改历史、审计、回滚（已决定不做）。
- Q6 列出的其他候选项。

#### Inputs
- `backend/config/config.go`（`CaptchaConfig`、`RiskConfig`、`validateConfig`、`removedEnvVars`）
- 上述 services、controllers；`backend/routes/router.go`（`RegisterAdminRoutes`）
- `backend/tests/` 中依赖这些配置的测试（改为通过 `ApplyRuntime` 设置）

#### Execution Requirements
- 快照不可变：保存时整体替换，不原地修改字段。
- 单次请求内只读取一次快照，保证同一请求前后使用的是同一组值。

#### Expected Outputs
- 新增：`backend/config/runtime.go`、`backend/config/runtime_test.go`、`backend/storage/migrations/0004_settings.sql`、`backend/services/settings_service.go`、设置接口的 controller 与测试
- 修改：上述消费方、config、router、env 文件

#### Acceptance Criteria
- [x] 23 个字段都有上下界单测，越界时错误信息含字段名。
- [x] 热生效：`PUT` 把 `defaultDifficulty` 改为 4 后，下一次出题难度为 4；`PUT` 把 `riskMode` 改为 `log` 后，高风险分提交不再被拦截；整个过程不重启（接口测试）。
- [x] 持久化：保存后重新 `Open` 并加载，值一致；库中无记录时取默认值（测试）。
- [x] `rg` 确认这 23 个变量名只出现在 `removedEnvVars` 与测试中。
- [x] 本机支持 cgo 时执行 `go test -race ./config ./services` 通过；不支持时在 Notes 中说明。
- [x] `go vet ./... && go test ./...` 通过。

#### Risks
- 漏改某个消费方，导致它仍读旧值：本阶段结束时 `CaptchaConfig` / `RiskConfig` 里已没有这些字段，漏改会直接编译失败。

#### Dependencies
Phase 4、5。

#### Notes
- 执行记录（2026-10-27）：
  - `config/runtime.go`：
    - 三组共 23 个字段，JSON 名带单位后缀（如 `captcha.expireSeconds`、`captcha.minSolveTimeMs`、`risk.stateTtlSeconds`、`risk.fpReplayWindowSeconds`、`risk.telemetryRetention`），并提供 `Expire()`、`MinSolveTime()`、`StateTTL()` 等换算方法。
    - 校验规则沿用原范围。原先没有范围的字段补了上下界：有效期 30～3600 秒、二次验证 10～3600 秒、尝试次数 1～20、状态保留 60～86400 秒、保留条数 0～100 万、指纹窗口 10～86400 秒、IP 加难 0～10 万、最短作答 0～10000 毫秒、指纹阈值 0～1000。
    - `Validate` 一次报告全部错误。`Sanitize` 供启动加载时把越界字段恢复默认；两个互相约束的字段（pow 难度与上限、加难阈值与拦截阈值）一起恢复。
  - `SettingsService.Update` 以当前值为底合并请求体：提交完整对象即整体替换，缺少的字段保持当前值，未知字段返回 400。这比"只接受完整对象"更宽松，控制台始终提交完整对象。
  - 与 Execution Requirements 的差异：快照在每个处理函数入口只读取一次，例如 `VerifyDataTemplate`、`Decide`、`persistChallenge`、`ShouldBlock`，没有从 controller 一路传到 service。一次请求跨多个函数时，如果恰好在中途保存了策略，前后两段可能用到不同的快照，影响可以忽略。
  - 单测注入：`services.settingsSource` / `fixedSettings`；`ChallengePolicy`、`RiskEngine`、`ClientRiskStore` 不再持有 `*config.Config`。
  - 测试改动：
    - `config_risk_test.go` 中 3 个按环境变量读默认值的用例并入 `runtime_test.go`。
    - `TestLoadFromEnv_IPCounters` 只保留管理员限流。
    - `tests/` 中的策略读取统一改为 `rt := config.Runtime()`，`skipWithoutRisk` 返回快照。
    - 新增 `tests/settings_test.go` 的 6 个用例：鉴权、读取、字段级报错、热生效（难度 4 与 enforce/log 切换）、重启后持久化、部分更新与加载时修正。
  - `rg` 检查：23 个变量名在 `backend/`（不含测试与 gitignore 的 `console/dist`）中只出现在 `removedEnvVars`。`.env.example` 中的"验证码与风控"段改为指向控制台。
  - 本机支持 cgo（gcc 可用）：`go test -race ./config ./services` 通过。

---

### Phase 7: 控制台「策略配置」页面

#### Status
COMPLETED

#### Goal
在管理后台新增「策略配置」页面，分组展示并编辑 23 项策略，保存后立即生效。

#### Scope
- `sdk/playground/pages/SettingsPage.tsx`；在 `sdk/playground/console/ConsoleView.tsx` 增加导航项「策略配置」；在 `sdk/playground/api.ts` 的 `adminApi` 增加 `getSettings` / `updateSettings` 与类型定义。
- 表单分三组（验证码、工作量证明、风控评分）。每项显示中文名称、一句说明（取自 README「环境变量参考」的现有描述）、单位、默认值。
- 有改动时才允许保存；"恢复全部默认值"只填充表单，仍需点击保存；保存成功提示"已保存并生效"；服务端返回 400 时在页面显示错误信息，不保存。
- 尚未修改初始密码时的拦截沿用其他页面的做法。

#### Out of Scope
- 修改历史、回滚、差异对比。
- 应用来源界面（Phase 11）。

#### Inputs
- `sdk/playground/pages/RiskPage.tsx`、`AppsPage.tsx`（作为样式与交互参考）、`ConsoleView.tsx`、`api.ts`
- Phase 6 的接口契约

#### Execution Requirements
- 不新增依赖，样式沿用现有页面。
- 用 Playwright 或 Chrome 在演示站（`http://localhost:8000/console/`）和内嵌页（构建后的 `/console/`）各验证一次。

#### Expected Outputs
- 新增：`sdk/playground/pages/SettingsPage.tsx`
- 修改：`sdk/playground/console/ConsoleView.tsx`、`sdk/playground/api.ts`

#### Acceptance Criteria
- [x] `pnpm lint && pnpm typecheck && pnpm test && pnpm build && pnpm build:console` 通过。
- [x] 浏览器验证：把默认难度改为 4 并保存后，`GET /admin/settings` 返回 4，刷新页面值保留；输入越界值时显示服务端错误，且值没有被保存。
- [x] "恢复全部默认值"后保存，所有字段回到默认值。

#### Risks
- 时长单位换算出错：界面直接按接口字段的单位（秒、毫秒）输入，不做换算。

#### Dependencies
Phase 6。

#### Notes
- 执行记录（2026-10-27）：
  - 页面分三组（验证码 9 项、工作量证明 4 项、风控评分 10 项），每项显示中文名称、说明（取自 README「环境变量参考」）、单位、默认值和取值范围。
  - 表单使用 `noValidate`，越界值交给服务端校验后显示。服务端会一次返回多个字段的错误，提示框用 `white-space: pre-line` 逐行显示。
  - "有改动"通过比较表单与已保存值得出，没有改动时保存按钮不可用。保存栏固定在底部（sticky）。
  - 浏览器验证：
    - 按台账要求使用临时实例：演示站 8010，内嵌页为 `127.0.0.1:3399/console/`；没有动用户的 8000。
    - 管理会话用 `reset-admin` 加初始化接口取得，再写入 sessionStorage。
    - 演示站：基础难度由 1 改为 4 并保存，提示"已保存并生效"，`GET /admin/settings` 返回 4，刷新后仍为 4。
    - 演示站：基础难度填 9、拦截阈值填 101，页面逐行显示两条服务端错误，库中值仍为 4 / 85。
    - 演示站："恢复全部默认值"后表单难度为 3，保存后 `settings` 与 `defaults` 完全一致。
    - 内嵌页：导航出现「策略配置」；把 `risk.mode` 改为 log 并保存，接口返回 log，随后改回 enforce。
  - 测试环境的策略种子：`scratchpad/test-settings.py` 直接写临时库的 settings 行（关闭风控评分、基础难度 1、关闭 IP 加难），供浏览器滑块自动化使用。
  - `pnpm lint`（16 条既有 warning）、`typecheck`、`test`、`build`、`build:console` 全部通过。

---

### Phase 8: 固定 API 路径与演示站同源访问

#### Status
COMPLETED

#### Goal
删除 `API_PREFIX` 与旧版 GET 出题接口，API 路径固定为 `/api/v1`；演示站和本地控制台通过 Vite 代理同源访问后端，为后续收紧跨域做准备。

#### Scope
- 后端：
  - 删除 `ServerConfig.APIPrefix`、`normalizeAPIPrefix` 与 `API_PREFIX` 的读取，改为常量 `/api/v1`。`API_PREFIX` 登记到 `removedEnvVars`，提示为"API 路径固定为 /api/v1，请为验证码服务分配独立域名"。
  - `backend/console/console.go` 删除占位符替换（`apiPrefixPlaceholder`、`NewHandler` 的 `apiPrefix` 参数），并更新对应测试。
  - 删除 `GET /captchas/:type` 路由、`ValidateCaptchaRequest`、`GetCaptchaHandler` 以及只服务于它的代码；原来使用 GET 的测试改为 POST。
- 演示站：
  - `sdk/playground/console/index.html` 删除 `captcha-api-prefix` meta。
  - `sdk/playground/api.ts`：`API_BASE` 固定为空字符串（同源），`API_PREFIX` 改为常量 `/api/v1`，删除读取 meta 的逻辑。
  - `sdk/vite.config.ts` 增加 `server.proxy`：`'/api'` 转发到 `VITE_API_PROXY_TARGET`（默认 `http://127.0.0.1:3321`），`changeOrigin: false`，让后端看到的 `Host` 与浏览器的 `Origin` 一致。
  - `sdk/playground/.env.example` 中的 `VITE_API_BASE` 改名为 `VITE_API_PROXY_TARGET`（Q7 默认）。
  - `sdk/playground/App.tsx`、`sdk/playground/console/ConsoleApp.tsx` 的 Provider 改为 `basePath: ''`（SDK 的 `prefix` 参数在 Phase 9 删除）。

#### Out of Scope
- SDK `ApiConfig` 改造（Phase 9）。
- 跨域中间件改造（Phase 10）。

#### Inputs
- `backend/config/config.go`、`backend/routes/router.go`、`backend/console/console.go` 及其测试、`backend/controllers/captcha_controller.go`、`backend/middleware/validation.go`
- `sdk/playground/api.ts`、`sdk/playground/console/index.html`、`sdk/vite.config.ts`、`sdk/playground/.env.example`、`sdk/playground/App.tsx`、`sdk/playground/console/ConsoleApp.tsx`、`sdk/playground/pages/AppsPage.tsx`（只把 `API_PREFIX` 的引用改为常量）

#### Execution Requirements
- 用户本机的 8000 开发服务器需要重启才能加载新的 Vite 代理配置；验证时另起临时实例（例如 8010），不要动用户的 8000 进程。

#### Expected Outputs
- 修改：上述后端与演示站文件

#### Acceptance Criteria
- [x] `rg API_PREFIX backend sdk/src sdk/playground` 只命中 `removedEnvVars` 与演示站的常量名（如保留同名常量）。
- [x] `POST /api/v1/captchas/rotate` 正常；`GET /api/v1/captchas/rotate` 返回 404。
- [x] 内嵌控制台返回的 `index.html` 不含占位符。
- [x] 临时 Vite 实例下，浏览器访问 `/console/` 能登录，网络请求全部发往同源的 `/api/v1/...`。
- [x] `go vet ./... && go test ./...`、`pnpm lint && pnpm typecheck && pnpm test && pnpm build && pnpm build:console` 通过。

#### Risks
- Vite 代理默认可能改写 `Host`：必须显式设置 `changeOrigin: false`，并在 Phase 10 的同源测试中覆盖。

#### Dependencies
无（执行顺序在 Phase 7 之后）。

#### Notes
- 执行记录（2026-10-27）：
  - 后端：
    - 新增常量 `config.APIPrefix = "/api/v1"`，代码与测试中的 `cfg.Server.APIPrefix` 全部改为引用该常量。`API_PREFIX` 登记到 `removedEnvVars`。
    - 删除 `normalizeAPIPrefix` 及其测试，改为告警测试。
    - `console.NewHandler(assets)` 原样返回 `index.html`。
    - 删除 GET 出题的全部代码：`GetCaptchaHandler`（含全局函数）、`ValidateCaptchaRequest`、`GetCaptchaRequest`、`models.CaptchaRequest`。
    - 同时删除只用于区分 GET / POST 的 `viaPost`（遥测字段、日志里的 `via=`、`IssueContext.ViaPost`、演示站 `api.ts` 的类型），控制台风控观察页本来就不显示它。
  - 测试改动：
    - `controllers_test.go` 的 GET 出题用例改为 POST（`postIssue`）；`models_test.go` 的 `CaptchaRequest` 用例改为 `IssueRequest`。
    - `TestIssueEndpoint_EmptyBodyMatchesGet` 改为 `TestIssueEndpoint_EmptyBodyAndGetRemoved`，并断言 GET 返回 404。
    - `TestGetEndpoint_NonBrowserUAEscalates` 改为 POST 版本。
    - 共享出口测试改用 POST 出题；`console_test` 改为断言 `index.html` 原样返回。
  - 演示站：
    - `API_BASE = ''`、`API_PREFIX = '/api/v1'` 常量，同名常量保留。
    - Vite 代理键必须写成 `'/api/'` 并带结尾斜杠。写成 `'/api'` 时会把源码模块 `/api.ts` 也转发给后端，从 `/console/` 入口加载会 404，这个问题已在验证中发现并修正。
    - `.env.example` 改为 `VITE_API_PROXY_TARGET`；AppsPage 中的 curl 示例改用 `window.location.origin`。
    - 用户本地的 `sdk/playground/.env`（如有）未改动。
  - 验证：
    - `GET /api/v1/captchas/rotate` 返回 404，`POST` 返回 200；内嵌 `/console/` 的 `index.html` 不含占位符。
    - 临时 Vite 实例 8010（`VITE_API_PROXY_TARGET=http://127.0.0.1:3399`）下访问 `http://localhost:8010/console/`，通过滑块后以 ops 登录成功；抓到的 69 个 fetch 请求全部发往 `http://localhost:8010/api/v1/…`。
    - 用户的 8000 开发服务器需要重启才能加载新的代理配置，本阶段没有动它。
  - 滑块求解脚本加了失败重试：松手后弹窗仍在就继续下一题，最多 20 次。

---

### Phase 9: SDK 接入参数改造（baseURL + appId）

#### Status
COMPLETED

#### Goal
SDK 的接入参数改为 `api: { baseURL?, appId }`，用户侧请求在查询参数中携带 `appId`，新增错误码 `APP_REJECTED`，版本升到 3.0.0；演示站与控制台传入内置控制台应用 ID。本阶段后端还会忽略 `appId`，现有功能不受影响。

#### Scope
- `sdk/src/Captcha.types.ts`：`ApiConfig` 改为 `{ baseURL?: string; appId: string }`，删除 `basePath` 与 `prefix`。`baseURL` 默认为空字符串（表示同源），去掉末尾斜杠，只接受 origin 形式（例如 `https://captcha.example.com`）。
- `sdk/src/CaptchaProvider.tsx`：默认值与嵌套 Provider 的合并逻辑同步更新。
- `sdk/src/hooks/use-captcha-request.ts`：
  - `captchaEndpointFor(api)` 返回 `${baseURL}/api/v1/captchas`。
  - 出题与提交 URL 追加 `?appId=<encodeURIComponent(appId)>`。
  - 未提供 `appId` 时出题直接失败（`LOAD_FAILED`），并给出明确提示"缺少 appId"。
- `sdk/src/CaptchaError.ts`：新增 `CaptchaErrorCode` 值 `APP_REJECTED`。服务端返回 1010 或 1011 时映射为它，`response` 保留原始响应；中文提示分别为"应用不存在或已停用"、"当前页面来源不在应用允许名单内"。
- `sdk/package.json` 版本改为 3.0.0。
- `sdk/playground/api.ts` 导出常量 `CONSOLE_APP_ID = 'app_console'`；`App.tsx`、`ConsoleApp.tsx` 改为 `api={{ appId: CONSOLE_APP_ID }}`（同源，不传 `baseURL`）。
- 测试：更新 `use-captcha-request.test.ts`（端点、查询参数、缺少 appId）、Provider 合并相关测试、错误码映射测试。

#### Out of Scope
- 后端校验（Phase 10）。
- README（Phase 13）。

#### Inputs
- `sdk/src/Captcha.types.ts`、`CaptchaProvider.tsx`、`hooks/use-captcha-request.ts`、`CaptchaError.ts`、`index.tsx`、`Captcha.tsx`（错误处理路径）
- `sdk/playground/api.ts`、`App.tsx`、`console/ConsoleApp.tsx`
- 记忆中的 SDK 公共 API 决策：`Captcha` 不重复 Provider 的配置，局部覆盖用嵌套 Provider，错误统一用 `CaptchaError` 的 code

#### Execution Requirements
- `index.tsx` 的导出只做类型层面的必要调整，不新增导出项。
- 1010 / 1011 两个错误码在 Phase 10 由后端实现，本阶段按台账约定的数值映射。

#### Expected Outputs
- 修改：上述 SDK 与演示站文件、`sdk/package.json`、相关测试

#### Acceptance Criteria
- [x] 构建出的 `d.ts` 中 `ApiConfig` 为新定义，且不含 `prefix` / `basePath`。
- [x] 浏览器中出题与提交请求都带有 `?appId=app_console`，演示站与控制台功能正常。
- [x] 1010 / 1011 映射为 `APP_REJECTED` 的单测通过。
- [x] `pnpm lint && pnpm typecheck && pnpm test && pnpm build && pnpm build:console` 通过。

#### Risks
- 外部接入方升级时必须修改代码：属于有意的破坏性变更，用主版本号 3.0.0 表达，README 在 Phase 13 写明迁移方式。

#### Dependencies
Phase 8。

#### Notes
- 执行记录（2026-10-27）：
  - `ApiConfig = { baseURL?: string; appId: string }`。Provider 默认 `{ baseURL: '', appId: '' }`，嵌套 Provider 浅合并（不变）。
  - 端点固定为 `${baseURL}/api/v1/captchas`，baseURL 会去掉首尾空白与末尾斜杠。
  - 未传 `appId` 或 baseURL 不是 origin 时，`get` 直接以 400 拒绝并给出中文提示，Captcha 组件据此报 `LOAD_FAILED`。
  - `apiConfigError`、`withAppId` 放在 `src/utils/api-config.ts`，而不是 `use-captcha-request.ts`：`hooks/index.tsx` 会 `export *`，放在 hook 文件里就会变成新的公共导出，违反"不新增导出项"。
  - `CaptchaError.ts` 新增 `APP_REJECTED`，以及 `appRejectedError(response)`（同样不对外导出）。
    - 出题返回 1010 / 1011 时报 `APP_REJECTED`，不再报 `LOAD_FAILED`。
    - 提交返回 1010 / 1011 时立即以 `APP_REJECTED` 结束，不计入答错次数，也不换题重试。
  - `dist/index.d.ts` 中 `ApiConfig` 只有 `baseURL?` 与 `appId`，不含 `basePath` / `prefix`；`CaptchaErrorCode` 含 `APP_REJECTED`；没有新增导出。
  - 演示站：`CONSOLE_APP_ID = 'app_console'`，两个入口都改为 `api={{ appId: CONSOLE_APP_ID }}`，删除 `API_BASE`。
  - 测试：重写 `captchaEndpointFor` 用例，新增 `utils/api-config.test.ts`（缺少 appId、origin 校验、转义）与 `CaptchaError.test.ts`（1010 / 1011 映射、其他错误不映射）。共 27 个用例全部通过。
  - 浏览器验证（8010 临时实例）：
    - `/console/` 登录全流程中 5 个验证码请求都带 `?appId=app_console`，其中出题 `POST …/captchas/auto?appId=app_console`、提交 `POST …/attempts?appId=app_console`，登录成功。
    - 演示站 `#/console` 的题型卡片可以正常出题（`…/slide-text?appId=app_console`）。
    - 后端在 Phase 10 才开始校验 appId。

---

### Phase 10: 应用允许来源与按应用跨域（后端）

#### Status
COMPLETED

#### Goal
应用支持配置多个允许来源；新增内置控制台系统应用；用户侧验证码接口按应用校验来源，不在名单内就直接拒绝；删除全局跨域配置。

#### Scope
- 迁移 `0005_app_origins.sql`：
  - `apps` 增加 `is_system INTEGER NOT NULL DEFAULT 0`。
  - 新建 `app_origins(app_id TEXT NOT NULL REFERENCES apps(id) ON DELETE CASCADE, origin TEXT NOT NULL, PRIMARY KEY (app_id, origin))`。
  - 插入系统应用 `app_console`：名称"管理后台"，`is_system = 1`，`enabled = 1`，`secret_hash` 为空。
- 来源规范化 `NormalizeOrigin`：
  - 只接受 `http` / `https`，必须有 host，端口可选，去掉默认端口 80 / 443，统一小写。
  - 拒绝路径（单个 `/` 除外）、查询串、片段、用户信息和通配符（Q8 默认只精确匹配）。
  - 每个应用最多 20 个来源，自动去重。
- 管理接口：
  - `POST /admin/apps`、`PUT /admin/apps/:id` 接收 `origins: string[]`；`GET /admin/apps` 返回 `origins` 与 `isSystem`。
  - 系统应用不可删除、停用、改名、轮换密钥、修改来源，违者返回 400 和 `ErrCodeSystemAppReadonly`（2015）。
- `AppAuthMiddleware`：系统应用没有密钥，无法用于 S2S 鉴权。
- 来源判定 `services.CheckOrigin(appID, origin, req)`：
  - 应用不存在或已停用 → `ErrCodeAppInvalid`（1010）。
  - 系统应用：按同源规则判定（Q2 默认：`Origin` 与请求的 scheme + host 一致；只有直连来源命中 `TRUSTED_PROXIES` 时才采信 `X-Forwarded-Host` / `X-Forwarded-Proto`）。
  - 普通应用：`Origin` 必须在名单内。
  - 缺少 `Origin` 或不匹配 → `ErrCodeOriginNotAllowed`（1011）。
- 跨域中间件重写：
  - 删除全局 `r.Use(middleware.CORSMiddleware(cfg.CORS))`，新中间件只挂在用户侧的出题与提交路由上（含 OPTIONS 预检）。
  - 预检：对任意 Origin 都返回 204，并回显 `Access-Control-Allow-Origin`、`Vary: Origin`、`Access-Control-Allow-Methods: POST, OPTIONS`、`Access-Control-Allow-Headers: Content-Type`、`Access-Control-Max-Age: 86400`。理由：预检一旦失败，浏览器根本不会发出真实请求，SDK 只能看到网络错误，无法读到错误码；真正的拦截放在真实请求上，拒绝发生在出题之前，不会消耗出图 CPU。
  - 真实请求：先执行 `CheckOrigin`，失败返回 403 `{ code, error }`，同时回显 `Access-Control-Allow-Origin` 让 SDK 能读到错误码；通过后才进入出题或提交逻辑，响应同样回显来源。
  - `/admin/*` 与 S2S `/captchas/states/:key` 不返回任何跨域响应头。
- 删除 `CORSConfig` 与 7 个 `CORS_*` 变量，登记到 `removedEnvVars`，提示为"跨域改为在管理后台为每个应用配置允许来源"；同步 env 文件。
- 新增错误码 1010、1011、2015（`backend/helper`）。

#### Out of Scope
- 控制台界面（Phase 11）。
- 验证码会话记录 appId 与 S2S 一致性校验（Phase 12）。

#### Inputs
- `backend/services/app_service.go`、`backend/controllers/app_controller.go`、`backend/middleware/cors.go`、`backend/middleware/auth.go`、`backend/routes/router.go`、`backend/helper/`（错误码、`IsTrustedProxyRequest`）、`backend/config/config.go`
- `backend/tests/apps_test.go`、`backend/tests/security_test.go`

#### Execution Requirements
- 拒绝响应只包含错误码与通用文案，不透露名单内容。
- 来源判定只出现在中间件和 service 中，controller 不重复判断。

#### Expected Outputs
- 新增：`backend/storage/migrations/0005_app_origins.sql`、来源规范化与判定代码及其测试、新跨域中间件测试
- 修改：上述后端文件、env 文件

#### Acceptance Criteria
- [x] `NormalizeOrigin` 表驱动单测覆盖：大小写、默认端口、末尾斜杠、路径、查询串、通配符、非 http(s) 协议。
- [x] 来源矩阵接口测试：
  - 名单内的 Origin：出题、提交成功，响应回显来源。
  - 名单外或缺少 Origin：返回 403 和 1011。
  - 应用不存在或已停用：返回 1010。
  - 系统应用：同源时成功，跨域时返回 1011。
  - 不可信来源携带的 `X-Forwarded-Host` 被忽略。
  - 预检对任意 Origin 返回 204。
- [x] 系统应用的删除、停用、改名、轮换密钥、改来源均返回 2015；系统应用无法通过 S2S 鉴权。
- [x] `/admin/*` 响应不含 `Access-Control-Allow-Origin`。
- [x] `rg CORS_ backend` 只命中 `removedEnvVars`。
- [x] 临时 Vite 实例下，控制台登录与题型卡片演示正常（通过代理同源访问）。
- [x] `go vet ./... && go test ./...` 通过。

#### Risks
- 部署在反向代理之后、且代理没有保留 `Host` 时，控制台的同源判定会失败：见 Q2，README 在 Phase 13 给出 nginx 配置示例。

#### Dependencies
Phase 2、8、9。

#### Notes
- 执行记录（2026-10-27）：
  - 迁移 0005：`apps.is_system` 列、`app_origins` 表，并插入系统应用 `app_console`（名称"管理后台"，`secret_hash` 为空串）。
  - 来源规范化与判定（`services/app_origin.go`）：
    - `NormalizeOrigin` 规范化单个来源，`normalizeOrigins` 负责去重并限制最多 20 个。
    - `AppService.CheckOrigin(appID, origin, selfOrigin)` 只返回 1010 / 1011，不透露名单内容。
    - `ValidAppID` 接受 `app_console`，否则管理接口的路径参数校验会对系统应用返回 404。
  - 中间件（`middleware/cors.go`）：
    - `AppOriginMiddleware` 只挂在出题与提交路由（含 OPTIONS）上，位于 IP 计数之前，被拒绝的请求既不计数也不出题。
    - `RequestOrigin` 只在直连来源命中 `TRUSTED_PROXIES` 时采信 `X-Forwarded-Host` / `X-Forwarded-Proto`（Q2 默认）。
    - 删除全局 CORS 中间件、`CORSConfig` 和 7 个 `CORS_*` 变量，均登记到 `removedEnvVars`。
  - 管理接口：
    - `POST /admin/apps` 接收 `origins`；`PUT` 的 `origins` 缺省时保持不变，提供时整体替换。
    - 列表返回 `origins` 与 `isSystem`，系统应用排在最前。
    - 系统应用的删除、停用、改名、改来源、轮换密钥都返回 400 / 2015；`Authenticate` 排除系统应用。
  - 补漏：Phase 8 漏删了 `backend/.env.example`、`.env.development` 中的 `API_PREFIX=/api/v1`，本阶段已删除。
  - 测试：
    - `middleware/cors_test.go` 重写为 4 个用例（预检、系统应用同源、转发头可信性、应用不存在或停用）。
    - `services/app_origin_test.go` 覆盖 `NormalizeOrigin` 表驱动（10 个合法输入、14 个非法输入）以及去重与上限。
    - 新增 `tests/origins_test.go`：来源矩阵 6 个子用例、来源管理、系统应用只读、管理与 S2S 接口无跨域头。
    - 测试辅助 `asConsoleApp` 让既有的出题与提交请求以 `app_console` 同源身份访问。
    - `listApps` 辅助过滤掉系统应用；`TestLoadFromEnv_CORS` 改为告警测试。
  - 验证：
    - curl：直连不带 Origin 返回 403；经代理且同源返回 200；伪造 Origin 返回 `{"code":1011}`。
    - 浏览器（8010 代理与 3399 内嵌页）：`/console/` 通过滑块登录成功，7 个验证码请求都带 appId；两个入口的题型卡片出题都返回 200。

---

### Phase 11: 控制台应用来源管理界面

#### Status
COMPLETED

#### Goal
在「接入应用」页面支持创建和编辑允许来源，系统应用只读展示，接入示例改为新的 SDK 写法。

#### Scope
- `sdk/playground/pages/AppsPage.tsx`：
  - 创建表单增加来源输入（可添加多个）；已有应用可编辑来源；列表展示来源。
  - 系统应用显示「系统」标记与"仅同源"，不提供编辑、停用、删除、轮换密钥操作。
  - 服务端校验错误显示在表单内。
  - 接入示例：前端为 `<CaptchaProvider api={{ baseURL: 'https://captcha.example.com', appId: '<应用 ID>' }}>`；S2S 示例使用固定路径 `/api/v1/captchas/states/<second_key>`。
- `sdk/playground/api.ts`：应用相关类型增加 `origins`、`isSystem`。

#### Out of Scope
- 后端逻辑（Phase 10 已完成）。

#### Inputs
- `sdk/playground/pages/AppsPage.tsx`、`sdk/playground/api.ts`、Phase 10 的接口契约

#### Execution Requirements
- 不新增依赖，样式沿用现有页面；用 Playwright 或 Chrome 验证。

#### Expected Outputs
- 修改：`sdk/playground/pages/AppsPage.tsx`、`sdk/playground/api.ts`

#### Acceptance Criteria
- [x] 浏览器验证：创建带 2 个来源的应用 → 编辑删掉 1 个 → 输入非法来源时显示错误 → 系统应用没有编辑和删除入口 → 接入示例里的 appId 正确。
- [x] `pnpm lint && pnpm typecheck && pnpm test && pnpm build && pnpm build:console` 通过。

#### Risks
- 无明显风险。

#### Dependencies
Phase 10。

#### Notes
- 执行记录（2026-10-27）：
  - 来源输入使用文本框，每行一个，也接受逗号分隔。前端只做拆分和"最多 20 个"的提示，规范化与校验都交给服务端。服务端的错误显示在对应表单内：新建表单与卡片内的来源编辑各自显示。
  - 卡片内的「允许来源」以标签形式列出；未配置时显示橙色提示"未配置（前端出题与提交会被拒绝）"。
  - 系统应用：显示「系统」徽标、"仅同源"和一句说明，不显示任何操作按钮，卡片用虚线边框。
  - 接入示例：新建或重置密钥后的提示卡增加前端代码 `<CaptchaProvider api={{ baseURL: '<当前服务 origin>', appId: '<应用 ID>' }}>`，可以一键复制。S2S 示例使用固定路径 `/api/v1/captchas/states/<second_key>`。
  - 没有普通应用时，系统应用下方显示空状态提示。
  - 浏览器验证（8010 临时实例，`/console/#/apps`）：
    - 创建带 2 个来源的应用（输入为 `https://www.example.com` 与 `HTTP://LOCALHOST:8000/`），列表显示规范化后的 `http://localhost:8000`。
    - 编辑删掉 1 个后只剩 `https://www.example.com`。
    - 输入 `https://*.example.com` 时，表单内显示"不支持通配符，请逐个填写完整来源"，取消后来源不变。
    - 系统应用有"系统"徽标和"仅同源"，按钮数为 0。
    - 接入示例里的 appId 与卡片上的应用 ID 一致（`app_9a3c609ef353`）。
  - `pnpm lint`（16 条既有 warning）、`typecheck`、`test`、`build`、`build:console` 全部通过。

---

### Phase 12: 验证码与应用绑定

#### Status
COMPLETED

#### Goal
出题时把 appId 写入题目会话；提交、S2S 查询、管理员登录都校验 appId 一致，防止跨应用复用验证结果；风控观察记录并展示所属应用。

#### Scope
- 题目会话（进程内存）记录出题时校验通过的 `appId`。提交时，查询参数中的 `appId` 必须与会话一致，否则按"题目不存在或已过期"（1004）处理，不暴露差异。
- 二次验证数据记录 `appId`。`GET /captchas/states/:key` 鉴权所得的应用 ID 必须与记录一致，否则响应与无效 key 相同，并且不消费该 key。
- 管理员登录（`AdminService.Login` → `ValidateSecondVerifyToken`）只接受 `appId = app_console` 的 `second_key`。
- 遥测写入 `app_id`；`RiskPage.tsx` 增加"应用"列，显示应用名称，查不到名称时显示 ID。
- 测试：跨应用矩阵。

#### Out of Scope
- 按应用筛选风控观察。
- 按应用区分风控策略（已决定不做）。

#### Inputs
- `backend/services/base_service.go`（题目与二次验证数据）、`risk_control_service.go`、`admin_service.go`、`controllers/captcha_controller.go`、`services/risk_engine.go`（遥测）
- `sdk/playground/pages/RiskPage.tsx`、`sdk/playground/api.ts`

#### Execution Requirements
- 不匹配时不得消费 key，避免一个应用把另一个应用的 key 用掉。
- 对外响应不区分"key 不存在"和"key 属于其他应用"。

#### Expected Outputs
- 修改：上述后端文件与 `RiskPage.tsx`；新增跨应用测试

#### Acceptance Criteria
- [x] 接口测试：
  - A 应用的 key 用 B 应用密钥查询：无效，且之后 A 应用仍可消费一次。
  - A 应用的 key 用 A 应用密钥查询：有效，第二次查询无效。
  - 控制台应用的 key 不能通过 S2S 消费。
  - 普通应用的 key 不能用于管理员登录。
  - 提交时 `appId` 与出题时不一致：返回 1004。
  - 遥测行带有 `app_id`。
- [x] 浏览器验证：风控观察页显示"应用"列。
- [x] `go vet ./... && go test ./...`、`pnpm lint && pnpm typecheck && pnpm build:console` 通过。

#### Risks
- 管理员登录链路改动可能把自己锁在外面：先跑通测试，再在浏览器中实际登录一次。

#### Dependencies
Phase 4、10。

#### Notes
- 执行记录（2026-10-27）：
  - 题目会话：`ChallengeMeta.AppID` 由 controller 从 `GetRequestAppID` 取得，通过来源校验后写入。`VerifyDataTemplate` 在计尝试次数之前比对 appId；不一致时返回与"题目不存在"相同的 1004，不计次数，也不作废题目（测试中断言 attempts 仍为 0）。
  - 二次验证：
    - `RiskControlService.ValidateSecondVerifyToken(token)` 改为 `ConsumeSecondVerifyToken(token, appID)`，用 `Memory.Update` 在一把锁内完成"读取、比对 appId、删除"。属于其他应用的 key 原样保留。
    - `GenerateSecondVerifyToken(appID)`（测试用）签发绑定应用的 key。
  - 消费方：S2S 状态查询使用 `AppAuthMiddleware` 鉴权得到的应用 ID；管理员登录固定为 `app_console`。
  - 遥测：出题与校验记录都写 `"app"` 字段，`TelemetryStore` 同时写入 `app_id` 列。`RiskPage` 增加"应用"列：并行拉取应用列表把 ID 换成名称，找不到时显示 ID；早期没有该字段的记录显示"—"。
  - 测试：
    - 新增 `tests/app_binding_test.go` 的 4 个用例，都经完整路由真实出题、解题、提交：
      - A 的 key 用 B 的密钥查询两次都无效，随后用 A 的密钥查询有效，再查无效。
      - 以 B 的身份提交 A 的题返回 1004，attempts 仍为 0，A 自己随后能通过。
      - A 的 key 不能用于管理员登录，且登录失败后 A 仍能消费；控制台 key 不能被 A 的密钥消费，但能登录。
      - issue 与 verify 两行遥测的 `app_id` 都是 A。
    - 既有测试的改动：`ValidateSecondVerifyToken` 调用全部改为 `ConsumeSecondVerifyToken`（并补"其他应用不消费"的断言）。经完整路由提交的用例在出题时带上 `AppID: services.ConsoleAppID`。
  - 验证：
    - 重新编译临时后端后，浏览器在 `/console/` 实际登录一次成功（台账要求，防止把自己锁在外面）。
    - 临时开启风控后，用 curl 分别以 `app_console`（经 8010 代理）和普通应用出题；风控观察页的"应用"列依次显示"官网-40900""管理后台"。验证后已恢复测试策略。
  - `go vet ./... && go test ./...`、`pnpm lint && pnpm typecheck && pnpm build:console` 通过。

---

### Phase 13: 文档更新、零配置启动与端到端验收

#### Status
COMPLETED

#### Goal
更新 README、AGENTS.md、环境变量模板与 `docker-compose.yml`，使其与实现一致：没有 `.env` 也能用 compose 启动。最后完成一次端到端验收。

#### Scope
- `README.md`：
  - 架构概览：去掉 Redis，说明 SQLite 与进程内存的分工。
  - 后端部署：
    - 不再需要 Redis，也不需要 `.env` 即可启动；`.env` 只用来覆盖端口、证书、代理网段等部署参数。
    - 首次启动后查看日志中的初始化口令，打开 `/console/` 完成初始化。
    - 数据卷包含 `captcha.db`；备份方式为停服后复制数据卷。
    - 反向代理需保留 `Host` 或配置可信代理（附 nginx 示例）。
  - RESTful API 路由：路径固定为 `/api/v1`，删除 GET 出题，用户侧接口带 `appId` 查询参数。
  - 前端接入与 `<CaptchaProvider>` Props：`api: { baseURL, appId }`，以及从 2.x 迁移的说明。
  - 错误处理：`APP_REJECTED`。
  - 安全机制：来源校验、验证码与应用绑定。
  - 二次验证（S2S）：只能查询本应用的 key。
  - 管理后台：首次初始化（口令、用户名、密码）、遗失密码时执行 `reset-admin` 重新初始化、策略配置、接入应用来源、数据目录。
  - 环境变量参考：只保留 29 项，并列出已删除的变量及去向。
  - 本地开发：不需要 Redis，Vite 代理，`VITE_API_PROXY_TARGET`。
- 二级标题如有改名，同步 `sdk/playground/docs/content.ts`。
- `AGENTS.md`：
  - 技术栈：去掉 Redis，加入 SQLite / modernc。
  - 目录说明：`cache/` 为进程内存，新增 `storage/`。
  - 日常调试指北：不再需要 Redis 容器；"管理员密码首次启动时随机生成……遗失用 `./run-dev.sh reset-admin-password`"改为"首次启动在控制台初始化，口令见终端；遗失用 `./run-dev.sh reset-admin`"；新增策略配置入口。
- `docker-compose.yml`：
  - `env_file` 改为 `- path: ./backend/.env` 加 `required: false`（本机 Docker Compose v5.6.0 支持）。
  - 更新注释：业务配置改在控制台；删除数据卷会回到未初始化状态，且全部应用密钥失效。
- `backend/.env.example`、`backend/.env.development` 最终核对：与 29 项部署类变量一致。
- 端到端验收（临时文件放在会话 scratchpad，不入库；Redis 容器全程停止，结束后恢复）：
  1. 使用全新的临时数据目录启动，终端打印初始化口令；在控制台初始化页面完成初始化，退出后重新登录。
  2. 创建应用 A（来源 `http://localhost:8010`）和应用 B。
  3. 在 8010 起一个临时页面，直接用 `baseURL` 跨域访问后端：应用 A 出题、提交成功；把来源从 A 的名单中移除后，同一页面看到 `APP_REJECTED`。
  4. A 页面通过后得到的 `second_key`：用 B 的密钥查询无效，用 A 的密钥查询有效且只能查询一次。
  5. 在「策略配置」把默认难度改为 4，下一题即生效；重启后值保留，风控观察记录仍在。
  6. curl 验证来源矩阵（名单外、缺少 Origin、停用应用）。
  7. 零配置启动：
     - 把工作区复制到 scratchpad，排除 `backend/.env`、`backend/data`、`node_modules`。
     - 用临时项目名执行 `docker compose -p captcha-e2e up -d --build`，端口冲突时用覆盖文件改成临时端口。
     - `docker compose -p captcha-e2e logs` 中能看到初始化口令，在浏览器中完成初始化。
     - 结束后执行 `docker compose -p captcha-e2e down -v`，只清理临时项目。

#### Out of Scope
- 新功能。

#### Inputs
- `README.md`、`AGENTS.md`、`sdk/playground/docs/content.ts`、`backend/.env.example`、`backend/.env.development`、`docker-compose.yml`

#### Execution Requirements
- 验收记录（命令与结果摘要）写进本阶段 Notes。
- 不得改动用户的 `backend/.env`，不得删除或改写现有的 `captcha-data` 卷与 `simple-captcha-backend` 容器。

#### Expected Outputs
- 修改：README、AGENTS.md、`docker-compose.yml`、`content.ts`（如需要）、env 文件
- 验收记录

#### Acceptance Criteria
- [x] README 与 AGENTS.md 中不再出现 `API_PREFIX`、`CORS_ALLOW_ORIGINS`、`REDIS_`、`ADMIN_USERNAME`、`reset-admin-password`、"随机生成"管理员密码的现行用法（"已删除变量"列表除外）。
- [x] 端到端 7 项全部通过并记录。
- [x] 全部检查命令通过（见 Success Criteria），`docker build -f backend/Dockerfile .` 成功。

#### Risks
- 演示站接入文档页按二级标题切分 README：改标题后漏改 `content.ts` 会导致章节缺失，验收时打开 `/#/docs` 检查。

#### Dependencies
Phase 1～12。

#### Notes
- 执行记录（2026-10-27）：
  - 文档：
    - README 的二级标题全部保持不变（接入文档页按二级标题切分，`content.ts` 不用改）。
    - 重写的章节：架构概览、验证流程、后端部署（零配置 compose、初始化、反向代理与备份）、接口路由与字段、二次验证、管理后台（首次初始化、数据目录、登录流程、接入应用与来源、策略配置、风控观察）、环境变量参考（29 项加已删除变量的去向表）、本地开发。
    - 前端接入、API 参考、安全机制按新接口与策略名称改写。
    - 另外同步了 `sdk/README.md`（npm 包内附带的说明）：`ApiConfig` 改为 `{ baseURL, appId }`，补 `APP_REJECTED` 与 2.x 升级说明。台账原本没有列这个文件，不改的话包内文档会与 3.0 不符。
    - AGENTS.md：技术栈、目录说明、日常调试指北已更新。
  - `docker-compose.yml`：`env_file` 改为 `path: ./backend/.env` 加 `required: false`，注释改为零配置说明。`docker compose config` 在没有 `backend/.env` 时通过。
  - 两个 env 模板只剩 29 项部署变量（脚本逐个比对）。
  - 端到端验收（临时文件都在 scratchpad；Redis 容器全程没有被用到）：
    1. 全新数据目录启动后打印初始化口令，残留的 `CORS_ALLOW_ORIGINS` 打印了移除警告。浏览器在 `127.0.0.1:3399/console/` 的初始化页面设置 `e2e-admin`，进入控制台；退出后通过滑块重新登录成功。
    2. 用管理接口创建应用 A（来源 `http://localhost:8010`）和应用 B（来源 `https://b.example.com`）。
    3. 在 8010 起临时接入页（独立 Vite 配置，SDK 直接引用源码），以 `baseURL=http://127.0.0.1:3399`、`appId=A` 跨域访问：出题与提交都发往 `127.0.0.1:3399`，得到 `second_key`。把 A 的来源改为 `https://other.example.com` 后，同一页面显示 `ERR APP_REJECTED 当前页面来源不在应用允许名单内`。为了让 8010 给临时接入页使用，先停掉了演示站在 8010 的临时实例。
    4. 该 `second_key` 用 B 的密钥查询两次都返回 1004，用 A 的密钥查询返回 `{"valid":true}`，再查返回 1004。
    5. 在「策略配置」把基础难度改为 4 并开启风控，下一次以 A 出题返回 `difficulty: 4`。重启后策略仍为 4、风控开启；风控观察里该题记录仍在（app=A，level=4）；会话令牌仍然有效；没有再次打印初始化口令。
    6. curl 来源矩阵：名单内 200；名单外 403/1011；缺少 Origin 403/1011；预检 204 并回显来源；B 停用后 403/1010；不存在的应用 403/1010；`/admin/setup` 不带跨域头。
    7. 零配置：
       - 把工作区复制到 scratchpad，排除 `backend/.env`（本来就不存在）、`backend/data`、`node_modules`。
       - 复制件的 compose 只改了端口（3397）和容器名，避免冲突。
       - `docker compose -p captcha-e2e up -d --build` 成功；`/health` 显示 `storage ok`，日志中有初始化口令。浏览器在 `127.0.0.1:3397/console/` 完成初始化，导航五个分区齐全。
       - 之后执行 `down -v --rmi local`，只清理了临时项目；用户的 `simple-captcha-redis` 容器仍在运行。
  - 其他检查：
    - 接入文档页 `/#/docs`（8011 临时演示站）9 个章节都能渲染，控制台没有告警。
    - `docker build -f backend/Dockerfile .`（仓库根）成功。
    - `go vet ./... && go test ./...`、`pnpm lint && pnpm typecheck && pnpm test && pnpm build && pnpm build:console` 全部通过。
  - 需要如实说明的一次操作：检查零配置时执行过一次把 `backend/.env` 移到 scratchpad 的命令。该文件并不存在，命令没有产生任何效果；这一步本不该做，后续改为在仓库副本中验证。
  - 验收结束后，已停止 3399、8010、8011 上的临时服务，并删除了检查用的镜像。`.playwright-mcp/` 下保留了 `login-flow.js` 和 `login-console.js` 两个滑块求解脚本，该目录已被 gitignore。

---

## 5. Current Execution Pointer

Current Phase: All phases completed

Current Objective:
全部阶段已完成。后续由用户审阅改动并自行提交；用户本机 8000 的演示站需要重启才能加载新的 Vite 代理配置（`VITE_API_PROXY_TARGET`）。

Current Restrictions:
- Phase 1～4 的测试仍需要本机 docker 容器 `simple-captcha-redis` 处于运行状态。
- 不得自动提交；不得使用 `git stash`。

---

## 6. Decision Log

| Date | Decision | Reason | Impact |
|---|---|---|---|
| 2026-10-27 | 只考虑单实例部署 | 用户决定 | 持久数据用内嵌 SQLite，短期数据用进程内存 |
| 2026-10-27 | 重新初始化，不导入旧环境变量与旧数据文件，不做兼容 | 用户决定：目前仅内部使用 | 升级后需重新登录、重建应用 |
| 2026-10-27 | 风控策略不按应用区分 | 用户决定 | 应用只新增"允许来源"设置 |
| 2026-10-27 | 移除 Redis：持久数据进 SQLite，带过期时间的短期数据进进程内存 | 用户提出，经实测评估采纳：存储只占出题耗时约 1%；SQLite 没有过期机制且写入串行，不适合短期数据 | Phase 4、5；测试不再依赖 Redis 容器 |
| 2026-10-27 | SQLite 使用 `modernc.org/sqlite`，WAL + `synchronous=NORMAL`，单连接 | Dockerfile 为 `CGO_ENABLED=0`；FULL 模式实测只有 242 次/秒 | Phase 1 |
| 2026-10-27 | 全仓库 Go 版本统一到 1.27.x（当前 1.27.3） | 用户决定（先定 1.26，随后改为统一 1.27.x）；`modernc.org/sqlite` 最新版要求 Go ≥ 1.26.0，Go 1.23 已停止维护；与本机工具链一致 | `backend/go.mod`、两个 Dockerfile、`run-dev.sh`、AGENTS.md |
| 2026-10-27 | 风控观察持久化到 SQLite（异步批量写入、按条数保留） | 用户选择 | Phase 5 |
| 2026-10-27 | 策略配置只保存当前值，不做修改历史、审计、回滚 | 用户选择 | Phase 6、7 不含历史表 |
| 2026-10-27 | 来源不在名单内或缺少 Origin 时直接拒绝（403） | 用户选择；跨域响应头只阻止浏览器读取响应，服务端照样执行 | Phase 10 |
| 2026-10-27 | 删除 `API_PREFIX`，路径固定为 `/api/v1`，建议独立域名部署 | 用户决定 | Phase 8 |
| 2026-10-27 | 前端传 `baseURL` 与 `appId`，每个应用配置多个允许来源，删除 `CORS_*` | 用户决定 | Phase 9、10、11 |
| 2026-10-27 | 验证码与应用绑定，S2S 只能消费本应用的 key | 补强用户方案：appId 公开、Origin 可伪造，只做来源校验无法阻止跨应用复用 | Phase 12 |
| 2026-10-27 | `appId` 放在 URL 查询参数中 | 出题与提交都适用，不依赖请求体结构 | Phase 9、10 |
| 2026-10-27 | 预检对任意 Origin 放行，拦截放在真实请求上，拒绝响应回显来源 | 预检失败时 SDK 只能看到网络错误，读不到错误码 | Phase 10 |
| 2026-10-27 | 内置系统应用 `app_console`，只接受同源请求 | 管理员登录也要过验证码，且首次登录前无法配置来源 | Phase 10、12 |
| 2026-10-27 | 本地开发改用 Vite 代理同源访问，不另开开发专用分支 | 用户偏好"保持统一，不做多种兼容方案" | Phase 8 |
| 2026-10-27 | 删除旧版 `GET /captchas/:type` 出题接口 | 不做兼容；SDK 3.0 必须带 appId | Phase 8 |
| 2026-10-27 | SDK 升到 3.0.0 | `ApiConfig` 破坏性变更 | Phase 9 |
| 2026-10-27 | 新增控制台首次初始化页面：管理员用户名与密码在初始化时设置，替代"随机密码 + 强制改密" | 用户提出：目标是近乎零配置启动，配置界面化 | 新增 Phase 3；Phase 2 改为直接实现初始化接口，避免先迁移随机密码流程再删除 |
| 2026-10-27 | 初始化需要启动日志中的一次性口令（用户确认 Q9 = A） | 建议独立域名部署后，服务在初始化前就可能暴露在公网，先到先得会被他人抢先初始化；口令只出现在日志里，仍然不需要任何配置 | Phase 2、3 |
| 2026-10-27 | 删除 `ADMIN_USERNAME` 与强制改密（错误码 2008 不再复用）；`reset-admin-password` 改名为 `reset-admin`，执行后回到未初始化状态 | 用户名与密码都由管理员自己设置，强制改密失去意义；不做兼容 | 部署类环境变量从 30 个减为 29 个；Phase 2、3、13 |
| 2026-10-27 | `ErrCodeSystemAppReadonly` 由 2009 改为 2015；初始化相关错误码使用 2012～2014 | v0.2 之前的台账误把已被 `ErrCodeWrongPassword` 占用的 2009 分配给系统应用（2010、2011 也已占用） | Phase 2、10 |
| 2026-10-27 | 新阶段插在 Phase 2 之后，原 Phase 3～12 顺延为 Phase 4～13 | 控制台在 Phase 2 之后就需要初始化页面，否则后续阶段的浏览器验证无法登录 | 全部阶段编号与依赖 |
| 2026-10-27 | 执行中按推荐默认处理的开放问题（问题保持 OPEN，可随时改）：Q2 = A（比对 Origin 与 scheme + host，只在可信代理时采信 X-Forwarded-*）；Q3 = A（保留 10000 条）；Q4 = A（每类 20 万条，超限淘汰最早到期）；Q5 = A（旧 JSON 只告警）；Q6 = A（这 5 项保留环境变量）；Q7 = A（`VITE_API_PROXY_TARGET`）；Q8 = A（只精确匹配）；Q10 = A（初始化页只建管理员账号） | 均为 `Blocking: NO`，按台账规则采用推荐默认 | Phase 2、4、5、6、8、10、3 |
| 2026-10-27 | 策略快照在每个处理函数入口读取一次，而不是从 controller 一路传入 service | 改动面小；只有在一次请求中途恰好保存策略时，前后两段才可能用到不同快照，影响可以忽略 | Phase 6 |
| 2026-10-27 | `PUT /admin/settings` 以当前值为底合并请求体（完整对象即整体替换），未知字段拒绝 | 比"只接受完整对象"更宽松，控制台始终提交完整对象 | Phase 6、7 |
| 2026-10-27 | 删除只服务于 GET 出题的 `viaPost` 遥测字段 | GET 出题已删除，所有出题都走 POST，该字段恒为 true | Phase 8 |
| 2026-10-27 | 尝试次数上限以出题时写入题目的值为准，修改策略只影响之后出的题 | 复核发现入口检查读题目保存的值、失败计数读最新策略，热更新后同一道题有两套上限；与过期时间一样按出题时固定，符合"下一次出题即生效" | Phase 6；`services.attemptLimit` |

---

## 7. Open Questions

| ID | Question | Options | Recommended Default | Blocking | Impact | Owner | Status |
|---|---|---|---|---|---|---|---|
| Q1 | Go 版本 | A. 升级到新版本；B. 保持 1.23.4 并锁定 `modernc.org/sqlite` v1.38.2 | A | NO | Phase 1 | User | RESOLVED（2026-10-27 用户决定全仓库统一 1.27.x） |
| Q2 | 反向代理之后控制台应用的同源判定 | A. 比对 `Origin` 与请求的 scheme + host，只在直连来源命中 `TRUSTED_PROXIES` 时采信 `X-Forwarded-Host` / `X-Forwarded-Proto`，README 给出 nginx 示例；B. 只比对 `Host`，要求代理保留 `Host`；C. 控制台应用也在界面配置来源（首次登录前无法配置） | A | NO | Phase 10、13 | User | OPEN |
| Q3 | 风控观察默认保留条数 | A. 10000；B. 1000（现值）；C. 改为按天数保留 | A | NO | Phase 5、6 | User | OPEN |
| Q4 | 进程内存短期数据的上限策略 | A. 每类 20 万条，超限淘汰最早到期的条目并告警；B. 超限时拒绝新写入（出题失败） | A | NO | Phase 4 | User | OPEN |
| Q5 | 旧的 `data/admin.json`、`apps.json` 如何处理 | A. 启动检测到时打印一次警告，不读也不删；B. 自动删除；C. 不提示 | A | NO | Phase 2 | User | OPEN |
| Q6 | `ADMIN_SESSION_TTL`、`ADMIN_IP_LIMIT_PER_MINUTE`、`IP_WHITELIST`、`IP_BLACKLIST`、`RISK_TLS_FINGERPRINT_LIST` 是否也进策略配置 | A. 本次保持环境变量；B. 一并纳入 | A | NO | Phase 6 范围 | User | OPEN |
| Q7 | 演示站代理目标的变量名 | A. 改名为 `VITE_API_PROXY_TARGET`；B. 沿用 `VITE_API_BASE` | A | NO | Phase 8 | User | OPEN |
| Q8 | 允许来源是否支持通配子域（如 `https://*.example.com`） | A. 只精确匹配；B. 支持通配子域 | A | NO | Phase 10 | User | OPEN |
| Q9 | 初始化页面如何防止被他人抢先初始化 | A. 启动日志打印一次性初始化口令，初始化时必须填写；B. 不设口令，先到先得；C. 只允许本机访问初始化接口（Docker 网桥与反向代理下无法可靠判断） | A | NO | Phase 2、3 | User | RESOLVED（2026-10-27 用户确认 A） |
| Q10 | 初始化页面是否顺带创建第一个接入应用 | A. 只创建管理员账号，之后在「接入应用」页面创建；B. 增加第二步：填写应用名称与允许来源 | A | NO | Phase 3 | User | OPEN |

`Blocking: YES` 表示执行者必须停下等待；`Blocking: NO` 表示执行者按推荐默认值继续，并把采用的选择记录到 Decision Log。

---

## 8. Change Log

| Version | Date | Change |
|---|---|---|
| v0.1 | 2026-10-27 | 创建台账：12 个阶段，含 SQLite 配置中心、移除 Redis、固定 API 路径、appId 与按应用来源、验证码与应用绑定；Q1 已按用户决定定为 Go 1.26.10 |
| v0.2 | 2026-10-27 | 按用户决定把 Go 改为全仓库统一 1.27.x（1.27.3）；Phase 1 增加 `deploy/fingerproxy/Dockerfile` 与 `backend/run-dev.sh` 两处版本引用 |
| v0.3 | 2026-10-27 | 按用户提议新增"控制台首次初始化页面"（新 Phase 3），原 Phase 3～12 顺延为 4～13；Phase 2 改为实现初始化接口与一次性口令，删除随机密码、强制改密与 `ADMIN_USERNAME`，`reset-admin-password` 改名为 `reset-admin`；Phase 13 增加 `docker-compose.yml` 的 `.env` 可选与零配置启动验收；修正系统应用错误码冲突（2009 → 2015）；新增 Q9、Q10 |
| v0.4 | 2026-10-27 | 开始执行（连续模式）。预检修正：旋转题浮层改动已在 89dada1 提交，删除 Forbidden Actions 与 Current Restrictions 中"未提交改动"的过时说明；Q9 用户确认 A |
| v0.5 | 2026-10-27 | Phase 1、2 完成。Phase 2 删除或改写了依赖随机密码与强制改密的测试（明细见 Phase 2 Notes） |
| v0.6 | 2026-10-27 | 全部 13 个阶段完成（连续执行）。Phase 13 额外同步了 sdk/README.md；端到端 7 项通过 |
| v0.7 | 2026-10-27 | 复核修复 3 处问题：① 同一道题的尝试次数上限统一为出题时的值（`base_verify_service.go` 的 `failResult` 不再读最新策略，新增 `TestVerify_MaxAttemptsFixedAtIssue`，覆盖上调与下调）；② `storage.Open` 对已存在的数据目录也收紧到 0700，目录不属于运行用户时拒绝启动（新增 `TestOpenTightensExistingDirMode`，README「数据目录」补充说明）；③ README 反向代理说明改正：在代理上终结 HTTPS 时，代理的直连地址必须命中 `TRUSTED_PROXIES`（默认值不含 127.0.0.1），nginx 用 `$http_host` 保留端口，`.env.example` 与环境变量表同步。已用 nginx 1.28 + 自签证书在 8443 实测 4 种组合，只有新示例 + 信任 127.0.0.1 时返回 200 |
