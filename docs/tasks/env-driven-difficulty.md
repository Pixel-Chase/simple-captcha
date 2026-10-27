# Project Task Ledger

## 1. Project Overview

### Project Name
验证码环境检测驱动难度分级（env-driven-difficulty）

### Project Goal
让验证码在**出题之前**就能根据运行环境与请求特征判断"像不像真实浏览器"，并据此**只做加难**：真人保持默认 3 级不变（用户实测 3 级对真人并不复杂），脚本、无头浏览器、请求头与自报环境不一致的客户端拿到逐级更难的题（4-5 级、更多 PoW 位数）。同时补上服务端可独立核验、客户端改 JSON 也伪造不了的信号，并为后续调权重提供观察面板。降级（真人更易）明确不在本次范围，留待观察数据后再议。

### Background
- 项目已有：SDK 在 `sdk/src/_internal/telemetry/env.ts` 采集环境信号、`use-behavior-trace.ts` 采集指针轨迹、`pow.ts` 计算工作量证明；后端 `backend/risk/score.go` 规则评分 0-100，`backend/services/client_risk.go` 在 Redis 保存每个客户端（IP + UA 哈希）的失败、成功、上次分数；难度 1-5 级（`backend/services/difficulty.go`），按 IP 压力加难（`challenge_policy.go`），`auto` 题型按等级选题。
- 三个结构性缺口：
  1. 环境信号只随 `POST /captchas/:type/:key/attempts` 提交，出题接口 `GET /captchas/:type` 只收 `type`、`mode`、`dpr`（`backend/models/request.go:25`），首题永远是默认难度。
  2. 升级只依据该客户端上一次校验的分数与失败次数（`backend/services/client_risk.go:123` 的 `LevelFromState`）以及 IP 压力，首题对所有人一视同仁：环境再异常也要等一次提交之后才开始加难。
  3. 自报的 `env` 与 HTTP 请求头之间没有任何比对，脚本原样重放一份真浏览器的 env JSON 即可绕过现有环境评分。
- 前提修正：环境信号是客户端自报数据，可被整包重放。因此本次**只用负面信号加难，不据正面信号降难**；真人不需要任何"证明自己"的信号就拿默认 3 级，脚本则因为忘了藏的痕迹、请求头不像浏览器、env 被重放而逐级加难。打了 stealth 补丁的真 Chrome 在环境层面与真人几乎无差别，这类对手只能靠行为轨迹与 PoW 成本对付，不在本任务目标内。
- 开源方案结论（2026-10-26 核实）：BotD（MIT，官方已进入"仅维稳"阶段，不检测 stealth 插件）与 CreepJS（MIT，研究工具）的检测点可借鉴后内置到 `env.ts`，不作为依赖引入；FingerprintJS v5 已回归 MIT 但本项目不需要稳定设备指纹，不引入；fingerproxy（Apache-2.0，Go）可作为 TLS 终结代理把 JA3/JA4/HTTP2 指纹写入请求头。

### Success Criteria
- 真实浏览器（Chrome / Firefox / Safari 任一）出题拿到默认难度 3，通过一次后仍为 3；旧版 SDK 走 `GET` 在真实浏览器中同样拿 3，响应结构与现在完全一致。
- `curl` 默认请求头出题（`GET` 或 `POST`）难度 ≥ 4；冒充浏览器 UA 但缺少 `Sec-Fetch-*` / Client Hints 的脚本出题难度 ≥ 4；复制真浏览器 env JSON 重放到 6 个以上不同客户端后，后续出题记录 `fp_replayed` 且难度 ≥ 4。
- 仅凭环境类信号（无行为证据、无硬性自动化标记）不会触发拦截；软件渲染器（VDI、虚拟机）用户拿到的难度不高于默认 3 级。
- `cd backend && go test ./...` 与 `cd sdk && pnpm test && pnpm typecheck && pnpm lint && pnpm build` 全部通过。
- README 的接口、安全机制、环境变量、管理后台章节与实现一致；管理后台能看到最近的出题评估与校验评分。

---

## 2. Global Constraints

### Technical Stack
- 后端：Go 1.23.4、Gin 1.10、go-redis v8、validator v10；Go 工具链在 `/usr/local/go/bin`，运行前需 `export PATH=$PATH:/usr/local/go/bin`；后端测试依赖本机 docker 启动的 Redis（`TestMain` 连接 `.env.development` 配置的地址）。
- 前端 SDK：React 19、TypeScript、tsdown（库构建）、Vite（演示页与控制台构建）、Vitest（`sdk/vitest.config.ts`，`environment: 'node'`，目前仓库内尚无测试文件）、Biome。包管理器 pnpm 11：构建脚本白名单只认 `sdk/pnpm-workspace.yaml` 的 `allowBuilds`。
- 运行时镜像 `alpine:3.19`，未安装 tzdata；Go 侧需要时区数据时必须 `import _ "time/tzdata"` 内嵌。

### Architecture Constraints
- 后端遵循 `Controller -> Service` 线性链路：Controller 只做参数绑定、校验、组装响应；评分、难度决策、Redis 读写全部在 `services` 与 `risk` 包。`risk` 包保持与 HTTP、Redis 无关的纯计算。
- 出题与校验共用同一套评分逻辑：出题时只有环境 + 请求信号，校验时再叠加行为轨迹与 PoW。
- 新增出题接口为 `POST /captchas/:type`，请求体 JSON `{mode, dpr, env}`，响应结构与 `GET` 完全相同；`GET /captchas/:type` 保留给旧版 SDK，视为"无环境信号"，只参与请求头核验（Phase 4 起），不参与 env 相关规则。
- SDK 公共 API 只从 `sdk/src/index.tsx` 导出；`sdk/src/_internal/` 下任何内容禁止对外导出；每个验证模式在自身目录闭环。
- 难度等级含义与 `difficultyProfiles` 的出题参数不变；难度只在 `CAPTCHA_DEFAULT_DIFFICULTY` 基础上向上调整，任何路径都不得产出低于默认值的等级；PoW 位数 = `CAPTCHA_POW_DIFFICULTY` + 等级附加位，随等级一同上升。
- 控制台页面放在 `sdk/playground/pages/`，由 `sdk/playground/console/ConsoleView.tsx` 挂导航，接口封装在 `sdk/playground/api.ts` 的 `adminApi`；演示站与内嵌页共用。

### Business Constraints
- IP 只用于加难，**绝不**按 IP 拒绝验证码请求（企业 NAT / 运营商 CGNAT 共享出口事故的教训）。
- 真人体验优先：所有新信号默认只加难不拦截；拦截必须有行为证据或硬性自动化标记参与。
- 出题前的环境采集与异常检测总预算不超过 150ms，不得让真人感知到出题变慢；采集失败一律静默降级为"无该信号"。
- 旧版 SDK（只会 `GET` 出题、可能不带 `trace`/`env`/`pow`）必须继续可用。

### Security / Compliance Constraints
- 不采集可直接识别个人的信息；新增字段只允许布尔、枚举标记、数值与哈希。遥测日志中的原始 IP 与完整 UA 在管理后台展示时必须脱敏。
- 风控拦截的真实原因只留在服务端日志，对外响应与普通答案错误完全一致（沿用 `in.RiskBlocked` 下沉到答案比对阶段的设计）。
- 代理注入的 TLS / HTTP2 指纹头只在直连来源命中 `TRUSTED_PROXIES` 时采信，其余情况即使携带也忽略，防止客户端自己伪造指纹头。
- 所有新增请求体字段必须有长度与条数上限（沿用 `backend/models/request.go` 的 `maxEnv*` 常量风格）；出题 `POST` 请求体复用 `VERIFY_MAX_BODY_SIZE` 上限。
- 借鉴 BotD / CreepJS 的检测点必须保留 MIT 版权与许可声明（新增 `sdk/THIRD_PARTY_NOTICES.md`），产品界面与文案不得使用 "CreepJS" 名称。

### Code Style / Engineering Rules
- 所有注释、日志、文档使用中文；代码标识符英文。
- React：解构导入 `import { useState } from 'react'`，类型用 `import type`；事件处理用 `e.stopPropagation()` / `e.preventDefault()`。
- Go：新增规则必须有表驱动单测；涉及 Redis 的逻辑放在 `backend/tests/` 或现有 `*_test.go` 并复用 `TestMain`；Redis 操作一律 fail-open（出错时记日志、不影响主流程）。
- 每个阶段结束前运行：`cd backend && go vet ./... && go test ./...`；涉及 SDK 的阶段再运行 `cd sdk && pnpm lint && pnpm typecheck && pnpm test && pnpm build`。
- 新配置项统一走 `backend/config/config.go` 的 `getXxxEnv` 并在 `Validate` 中校验范围，同时补进 `backend/.env.example`（注释形式）与 README「环境变量参考」。
- 修订接入说明只改仓库根目录 `README.md`；二级标题改名时同步 `sdk/playground/docs/content.ts` 的 `CURATED_GROUPS`。

### Forbidden Actions
- 不得按 IP 拒绝验证码出题或提交请求。
- 不得把 `sdk/src/_internal/` 内容导出到公共入口。
- 不得新增 SDK 运行时依赖（当前仅 `axios`）；不得引入 BotD、FingerprintJS、ThumbmarkJS 等作为依赖。开发依赖仅在 Vitest 环境确有需要时允许新增 `happy-dom`。
- 不得新增 Go 第三方模块（标准库 `time/tzdata` 允许）。
- 不得修改 `backend/internal/gocaptcha/`（本任务无需触碰引擎）。
- 不得把风控拦截原因、评分或难度决策依据写进对外响应。
- 不得自动执行 `git commit` / `git push`；提交由用户决定，执行者只在阶段完成后给出改动清单。
- 不得在 `sdk/playground/` 内另写一份接入文档。
- 不得删除或改写既有测试的断言来让测试通过；行为确实变更时需更新断言并在 Change Log 说明。

---

## 3. Phase Status Summary

| Phase | Name | Status | Depends On | Output Summary |
|---|---|---|---|---|
| 1 | 评分拆分与出题前加难策略（纯函数层） | COMPLETED | - | `risk.EvaluateEnv` / `EvaluateBehavior` / 环境分上限 / `ChallengePolicy.Decide` 只升不降 / `ShouldBlock` 需行为证据 / `RISK_ENV_SCORE_CAP`、`RISK_BLOCK_MIN_BEHAVIOR` / 4 个新测试文件 |
| 2 | POST 出题接口与出题评估接线 | COMPLETED | 1 | `POST /captchas/:type`（`IssueRequest` / `ValidateIssueRequest` / `IssueCaptchaHandler`）、GET 与 POST 共用 `issue()`、`RecordIssue` 写 `kind=issue` 遥测 / `tests/issue_endpoint_test.go` 7 个用例 + `TestIssueRequest_Validate` |
| 3 | SDK 切换到 POST 出题并前置环境采集 | COMPLETED | 2 | `get(env?)` 改 POST JSON、`buildIssuePayload` / `captchaEndpointFor`、`Captcha.tsx` 出题前 `collectEnv`、`CaptchaIssuePayload` 类型 / `use-captcha-request.test.ts`、`env.test.ts` 共 13 个用例；Playwright 实测四种题型 POST 出题与滑块通过 |
| 4 | 服务端请求头核验与跨层一致性 | COMPLETED | 2 | `risk/request.go`（12 条规则 + `Consistent`）、`helper.RequestSignalsFrom`、出题 / 校验均传 `Request`、遥测含 `consistent` / `requestReasons`、`tests/browser_headers_test.go` 辅助 / `risk/request_test.go` + 4 个接口用例 |
| 5 | 指纹重放计数 | COMPLETED | 2 | `RISK_FP_REPLAY_THRESHOLD` / `RISK_FP_REPLAY_WINDOW`、`ClientRiskStore.TouchFingerprint`（Lua 原子 + 饱和截断 + fail-open）、`Decide` 读配置阈值、控制器只在带 fp 的 POST 上计数 / `tests/fingerprint_replay_test.go` 3 个用例 + `services/fingerprint_replay_test.go` 2 个用例 |
| 6 | SDK headless 痕迹增补与服务端映射 | COMPLETED | 3, 4 | `anomalies.ts` 6 类检测（并行、150ms 预算、页面级缓存）、`collectEnvWithAnomalies`、`Env.Anomalies` 校验与权重表、`sdk/THIRD_PARTY_NOTICES.md` / `anomalies.test.ts` 10 个用例 + Go 权重 / 上限用例；Playwright 复核 |
| 7 | TLS / HTTP2 指纹读头与部署示例（可跳过） | COMPLETED | 4 | `RISK_TLS_FINGERPRINT_*` 三项配置、`helper.IsTrustedProxyRequest` 守卫、`risk/tlsfp.go` 列表与两条规则、遥测 `tls` / `h2`、`deploy/fingerproxy/`（Dockerfile + compose + 模板 + README）/ `risk/tlsfp_test.go` + `tests/tls_fingerprint_test.go` 6 个用例；fingerproxy 容器实跑一次 |
| 8 | 管理后台风控观察页 | COMPLETED | 2 | `GET /admin/risk/telemetry`（limit 默认 100 / 上限 500）、`services/telemetry_mask.go` 脱敏、`RecentTelemetry`、`RiskPage.tsx` + 导航「风控观察」/ `tests/admin_risk_test.go` 3 个用例 + 脱敏单测；Playwright 实测演示站与内嵌页 |
| 9 | 文档更新与手工端到端验收 | COMPLETED | 3, 4, 5, 6, 8（7 若执行） | README 8 处章节更新、`.env.example` 核对、AGENTS.md 目录树；端到端 6 项脚本核对 + Playwright 浏览器复核全部通过（v0.11） |

Allowed status values:

- NOT_STARTED
- IN_PROGRESS
- BLOCKED
- COMPLETED

---

## 4. Phase Details

### Phase 1: 评分拆分与出题前加难策略（纯函数层）

#### Status
COMPLETED（2026-10-26）

#### Goal
把现有单一的 `risk.Evaluate` 拆成"环境 / 请求侧"与"行为侧"两部分，给环境侧设贡献上限；新增出题前的难度决策 `ChallengePolicy.Decide`，在既有升级逻辑之上按环境分逐级加难，永不低于默认难度；拦截改为必须有行为证据或硬性自动化标记。本阶段不改任何 HTTP 接口。

#### Scope
- `backend/risk/score.go`：
  - 新增 `EnvResult{Score int; Reasons []string; HardMarker bool}` 与 `EvaluateEnv(env *Env, req *RequestSignals) EnvResult`。本阶段 `RequestSignals` 只定义空壳类型（字段在 Phase 4 填充），`req == nil` 视为中性。
  - 现有环境规则（`webdriver`、`automation_markers`、`software_renderer`、`touch_without_touchscreen`、`viewport_larger_than_screen`、`no_languages`、`desktop_chrome_without_plugins`、`no_hardware_concurrency`、`env_missing`）全部迁入 `EvaluateEnv`；`webdriver` 与 `automation_markers` 置 `HardMarker = true`。
  - 新增 `BehaviorResult{Score int; Reasons []string; HardMarker bool; Features *TraceFeatures}` 与 `EvaluateBehavior(captchaType string, trace *Trace, powExpected, powProvided bool) BehaviorResult`；`untrusted_events` 置 `HardMarker = true`。
  - `Evaluate(in Input) Result` 保留签名，内部组合两者：`env` 部分裁剪到 `in.EnvScoreCap`（新增字段，≤0 表示不裁剪），再与行为部分相加后裁剪到 [0, 100]。`Result` 新增 `EnvScore`、`BehaviorScore`、`HardMarker` 字段。
- `backend/services/challenge_policy.go`：
  - 新增 `IssueInput{ClientHash string; IPRequests int; Env *risk.Env; Request *risk.RequestSignals; FPReplays int; State ClientRisk}` 与 `IssueDecision{Level int; Base int; Boost int; EnvScore int; Consistent bool; Reasons []string}`。
  - 新增 `Decide(in IssueInput) IssueDecision`，替代 `DifficultyFor`（更新所有调用点与测试）。计算顺序：
    1. `level = LevelFromState(base, state)`（既有失败次数与上次分数升级逻辑不变）。
    2. `level += IPPressureBoost(in.IPRequests, cfg)`（不变）。
    3. 若 `in.Env != nil || in.Request != nil`：`envResult = risk.EvaluateEnv(in.Env, in.Request)`。`Env == nil` 时只跑请求侧规则（Phase 4 前 `Request` 恒为 nil，本步不触发）；`EvaluateEnv` 本身不计 `env_missing`，该原因仍只由校验阶段的 `Evaluate` 在 `Env == nil` 时追加。`FPReplays > threshold` 时追加原因 `fp_replayed(+30)`（阈值来自 Phase 5 配置，本阶段读配置默认值）。`envScore ≥ 30` 加 1 级，`≥ 55` 加 2 级（常量，写入代码注释）。
    4. `Level = ClampDifficulty(level)`。不存在任何降级分支，`Level ≥ CAPTCHA_DEFAULT_DIFFICULTY` 是不变量，用单测锁定。
  - `ResolveType` / `ChooseAutoType` 不变。
- `backend/services/risk_engine.go`：
  - `Evaluate` 把 `cfg.Risk.EnvScoreCap` 传入 `risk.Input`。
  - `ShouldBlock(result)`：`enforce` 且 `Score ≥ BlockThreshold` 且（`BehaviorScore ≥ RISK_BLOCK_MIN_BEHAVIOR` 或 `HardMarker`）。
  - 遥测日志条目新增 `"kind": "verify"`、`envScore`、`behaviorScore`。
- `backend/config/config.go`、`backend/.env.example`：新增 `RiskConfig.EnvScoreCap`（`RISK_ENV_SCORE_CAP`，0-100，默认 60）、`RiskConfig.BlockMinBehavior`（`RISK_BLOCK_MIN_BEHAVIOR`，0-100，默认 20），`Validate` 校验范围。
- 测试：`backend/risk/score_test.go`、`backend/services/difficulty_test.go`、`backend/tests/security_test.go` 中受影响的用例更新与新增。

#### Out of Scope
- 任何路由、请求模型、控制器改动（Phase 2）。
- 请求头规则本体（Phase 4）、指纹重放的 Redis 读写（Phase 5）、新异常标记（Phase 6）。
- README 更新（Phase 9）。

#### Inputs
- `backend/risk/score.go`、`backend/risk/types.go`、`backend/risk/features.go`
- `backend/services/challenge_policy.go`、`backend/services/client_risk.go`（`ClientRisk`、`LevelFromState`）、`backend/services/risk_engine.go`、`backend/services/difficulty.go`
- `backend/controllers/captcha_controller.go`（`DifficultyFor` 调用点，仅改为调用 `Decide` 并传入既有参数）
- `backend/config/config.go`、`backend/.env.example`
- 现有测试：`backend/risk/score_test.go`、`backend/services/difficulty_test.go`、`backend/tests/security_test.go`（`TestClientRiskStore_EscalatesDifficulty`、`TestRiskEngine_BlocksObviousBot`、`TestRouter_SharedEgressEscalatesInsteadOfBlocking`）

#### Execution Requirements
- `risk` 包不得引入 `net/http`、`gin`、Redis；`RequestSignals` 在本阶段只需 `type RequestSignals struct{}` 占位，并注明 Phase 4 填充。
- 既有 reason 名称与分值保持不变，便于对照历史遥测日志。
- `GET` 出题路径传入 `IssueInput{Env: nil, Request: nil}`，确保结果与改动前逐级一致（`TestRouter_SharedEgressEscalatesInsteadOfBlocking` 等既有测试不改断言即可通过）。
- 控制器日志行追加 `boost=%d envScore=%d`。

#### Expected Outputs
- 修改：`backend/risk/score.go`、`backend/risk/types.go`、`backend/services/challenge_policy.go`、`backend/services/risk_engine.go`、`backend/controllers/captcha_controller.go`、`backend/config/config.go`、`backend/.env.example`
- 新增 / 更新测试：`backend/risk/score_test.go`、`backend/services/difficulty_test.go`、`backend/config/config_test.go`

#### Acceptance Criteria
- [x] `EvaluateEnv` 对干净 env 返回 `Score 0`，对 `webdriver=true` 返回 `HardMarker=true`。（`risk/score_env_test.go`：`TestEvaluateEnv_CleanEnvScoresZero`、`TestEvaluateEnv_HardMarkers`）
- [x] 环境侧叠加 `webdriver + automation_markers + software_renderer + no_languages + ...` 时，`Evaluate` 的 `EnvScore ≤ RISK_ENV_SCORE_CAP`（默认 60）。（`TestEvaluate_EnvScoreCapped`：115 → 60）
- [x] `ShouldBlock`：总分 ≥ 85 但 `BehaviorScore < 20` 且无硬性标记时为 `false`；`webdriver=true` 叠加行为分使总分 ≥ 85 时为 `true`；`RISK_MODE=log` 时恒为 `false`。（`services/risk_engine_test.go`）
- [x] `Decide`：默认配置下，干净 env + 零失败 → 3；`Successes ≥ 1` 仍为 3；`Env == nil` → 3；`software_renderer` 单独命中（envScore 25）→ 3；`webdriver=true` → ≥ 4；`webdriver + automation_markers` → 5；`Fails ≥ 2` 与环境加难叠加后仍被裁剪到 5。（`services/challenge_policy_test.go`）
- [x] 不变量测试：对随机生成的 200 组 `IssueInput`，`Decide().Level ≥ CAPTCHA_DEFAULT_DIFFICULTY` 恒成立。（`TestDecide_NeverBelowDefault`，base 1-5 各 200 组）
- [x] 配置越界（`RISK_ENV_SCORE_CAP=101`、`RISK_BLOCK_MIN_BEHAVIOR=-1`）启动校验报错。（`config/config_risk_test.go`）
- [x] `go vet ./... && go test ./...` 通过。（`go vet` 无输出；`risk`、`services`、`config`、`middleware`、`console` 等全部 ok；`tests` 包仅 `TestAdmin_ListBackgrounds`、`TestBackgroundStore_GetCoversRequestedSize` 失败，已在干净 HEAD 工作树复现为既有失败，与本任务无关，见 Q8）

#### Risks
- 拆分时漏迁某条规则导致分值漂移：用"改动前后同一输入分值相同"的回归用例覆盖现有 `score_test.go` 全部输入。
- 环境分上限会削弱 `webdriver + automation` 这类明显脚本的总分：由 `HardMarker` 保证拦截能力不下降。

#### Dependencies
无。

#### Notes
- 升级阈值 30 / 55 与现有 `RISK_ESCALATE_THRESHOLD=55` 的语义不同：前者作用于出题前的环境分，后者作用于上一次校验的总分。代码注释中要写清。
- 本阶段结束后行为无外部可见变化（`GET` 路径与原来一致），可独立合入。
- `IssueDecision` 预留了 `Consistent` 字段但不参与计算，只为将来若要做降级时有数据可查；本台账不包含任何降级实现。
- 执行记录（2026-10-26）：
  - `touch_without_touchscreen` 同时依赖 env 与 trace，无法放进只收 env 的 `EvaluateEnv`，改为在 `Evaluate` 中计入环境侧得分（受同一上限约束），分值与名称不变。
  - `IssueInput.State` 实现为 `*ClientRisk`：nil 时策略按 `ClientHash` 从存储读取，单测直接传状态即可不依赖 Redis。
  - `ClientRiskStore.LevelFor` 只被 `DifficultyFor` 使用，一并删除；`LevelFromState` 保留。
  - 指纹重放阈值暂为包级常量 `defaultFPReplayThreshold = 5`，Phase 5 接入配置项。
  - `Decide` 额外产出 `client_history(+n)`、`ip_pressure(+n)`、`env_boost(+n)` 原因，便于观察页展示加难来源。
  - 快照回归：拆分前后 5 组代表性输入的分值与原因集合逐一相同（`TestEvaluate_RegressionSnapshot`）。

---

### Phase 2: POST 出题接口与出题评估接线

#### Status
COMPLETED（2026-10-26）

#### Goal
新增 `POST /captchas/:type` 出题接口，请求体携带 `mode`、`dpr`、`env`，服务端据此在出题时执行 `Decide`，并把出题决策写入遥测日志；`GET` 原样保留。

#### Scope
- `backend/models/request.go`：新增 `IssueRequest{Type (uri); Mode, Dpr (json); Env *risk.Env (json)}`，复用 `CaptchaRequest.Scale()` 的换算；把 `VerifyRequest.ValidateVerifyRequest` 中的 env 长度校验抽成 `validateEnv(env *risk.Env) error` 供两者共用。
- `backend/middleware/validation.go`：新增 `ValidateIssueRequest()`，绑定 URI + JSON，请求体套 `http.MaxBytesReader(VerifyMaxBodySize)`，超限返回 413、JSON 错误返回 400，存入上下文 `issue_request`；新增 `GetIssueRequest(c)`。
- `backend/routes/router.go`：`registerCaptchaRoutes` 增加 `rg.POST("/:type", ipPressure, middleware.ValidateIssueRequest(), controllers.IssueCaptchaHandler)`。
- `backend/controllers/captcha_controller.go`：
  - 抽出 `issue(c, type, mode, scale, env *risk.Env)` 供 `GetCaptchaHandler`（env 传 nil）与新 `IssueCaptchaHandler` 共用。
  - 构造 `IssueInput{ClientHash, IPRequests, Env, Request: nil（Phase 4 填充）, FPReplays: 0（Phase 5 填充）, State: store.Get(clientHash)}`，调用 `policy.Decide`。
  - 调用 `riskEngine.RecordIssue(ctx)` 写遥测：`{"kind":"issue","time","type","client","ip","level","base","boost","envScore","consistent","reasons","fp","viaPost":bool}`。
- `backend/services/risk_engine.go`：新增 `RecordIssue(...)`，走既有 `store.LogTelemetry`，`RISK_ENABLED=false` 时不写。
- `backend/services/service_manager.go`：如控制器需要 `ClientRiskStore`，通过 `GetClientRiskStore()` 获取（已存在）。
- 测试：`backend/tests/security_test.go` 新增 httptest 用例。

#### Out of Scope
- SDK 改动（Phase 3）。
- 请求头信号提取（Phase 4）、fp 计数（Phase 5）。
- README 接口文档（Phase 9）。

#### Inputs
- Phase 1 产出的 `Decide`、`IssueInput`。
- `backend/models/request.go`、`backend/middleware/validation.go`、`backend/routes/router.go`、`backend/controllers/captcha_controller.go`、`backend/services/risk_engine.go`、`backend/services/client_risk.go`（`LogTelemetry`）。
- `backend/middleware/cors.go` 与 `CORS_ALLOW_METHODS` 配置：确认已允许 `POST` 与 `Content-Type` 头（`/attempts` 已依赖）。

#### Execution Requirements
- `POST` 与 `GET` 返回体字段完全一致（`models.CaptchaResponse`），`Cache-Control: no-store` 等头一致。
- `POST` 必须挂 `ipPressure` 中间件（与 `GET` 同一计数窗口）。
- `env == nil` 的 `POST`（`telemetry='off'` 的 SDK）与 `GET` 等价：不跑 env 相关规则。
- 出题日志行格式沿用现有 `生成验证码成功: ...`，追加 `via=post|get level=%d boost=%d envScore=%d`。
- 新 handler 同样提供包级兼容函数 `IssueCaptchaHandler(c *gin.Context)`，与现有 `GetCaptchaHandler` 的 `sync.Once` 模式一致。

#### Expected Outputs
- 修改：`backend/models/request.go`、`backend/middleware/validation.go`、`backend/routes/router.go`、`backend/controllers/captcha_controller.go`、`backend/services/risk_engine.go`
- 测试：`backend/tests/security_test.go` 新增 `TestIssueEndpoint_*` 系列、`backend/tests/models_test.go` 新增 `IssueRequest` 校验用例

#### Acceptance Criteria
- [x] `POST /api/v1/captchas/auto` 带 `{"dpr":2,"env":{<干净 env>}}` 返回 200，`difficulty` 为 3；同一客户端成功校验一次后再 `POST`，`difficulty` 仍为 3。（`tests/issue_endpoint_test.go`：`TestIssueEndpoint_CleanEnvStaysDefaultBeforeAndAfterSuccess`，含一次真实滑块校验通过）
- [x] `GET /api/v1/captchas/auto?dpr=2` 返回 `difficulty` 3，响应结构与改动前逐字段一致。（`TestIssueEndpoint_EmptyBodyMatchesGet` 比对 GET 与 POST 响应字段集合；`TestRouter_SharedEgressEscalatesInsteadOfBlocking` 断言未改仍通过）
- [x] `POST` 带 `env.webdriver=true` 返回 `difficulty ≥ 4`。（`TestIssueEndpoint_WebdriverEscalates`：webdriver → 4，webdriver + automation → 5）
- [x] `POST` 请求体超过 `VERIFY_MAX_BODY_SIZE` 返回 413；`env.languages` 超 32 项返回 400（`code 400`）。（`TestIssueEndpoint_RejectsOversizedBody`、`TestIssueEndpoint_RejectsOversizedEnvList`）
- [x] `POST` 不带 `env` 时 `difficulty` 为 3。（`TestIssueEndpoint_EmptyBodyMatchesGet`）
- [x] Redis `captcha:telemetry` 列表出现 `kind=issue` 的记录，字段齐全；`RISK_ENABLED=false` 时不写。（`TestIssueEndpoint_TelemetryLogged` 校验 level/base/boost/envScore/reasons/fp/viaPost；关闭时不写由 `RecordIssue` 首行 `if !e.Enabled() { return }` 保证，`services/risk_engine.go`）
- [x] CORS 预检 `OPTIONS /api/v1/captchas/auto` 带 `Access-Control-Request-Method: POST` 返回 204。（`TestIssueEndpoint_CORSPreflightAllowsPost`）
- [x] 既有全部测试通过。（`go test ./...`：除 Q8 的两个既有失败外全部 ok）

#### Risks
- `ShouldBindJSON` 对空 body 报错：`POST` 允许空 body 视为无 env，需在中间件里对 `ContentLength == 0` 单独处理。
- 遥测列表容量 `RISK_TELEMETRY_LOG_SIZE=1000` 现在同时存出题与校验两类记录，可见历史缩短一半：在 Phase 9 文档中说明，必要时建议运维调大。

#### Dependencies
Phase 1。

#### Notes
- `IssueRequest.Type` 的 `oneof` 与 `CaptchaRequest` 保持一致（含 `auto`）。
- 不在本阶段标记 `GET` 为 deprecated（见 Open Questions Q2）。
- 执行记录（2026-10-26）：
  - 空请求体判定：`ContentLength == 0` 直接跳过 JSON 绑定；`ContentLength == -1`（chunked）且实际为空时 `ShouldBindJSON` 返回 `io.EOF`，同样视为无环境信号。
  - `RecordIssue` 每次出题都写遥测列表，但只在有加难原因时输出一行 `出题评估:` 日志，避免真人流量刷屏；控制器的 `生成验证码成功:` 日志追加 `via=get|post`。
  - `models/request.go` 抽出 `scaleFromDpr` 与 `validateEnv` 供 GET / POST / 校验三处共用。
  - 遥测 `kind=issue` 条目字段：`time, type, client, ip, key, level, base, boost, envScore, consistent, reasons, viaPost`，带 env 时另有 `fp, ua, webdriver, automation`。

---

### Phase 3: SDK 切换到 POST 出题并前置环境采集

#### Status
COMPLETED（2026-10-26）

#### Goal
SDK 出题改为先采集环境信号、再 `POST` 出题；保持公共 API（`CaptchaProvider` / `Captcha` / `useCaptcha` 的 props）不变，并建立首批 Vitest 测试。

#### Scope
- `sdk/src/Captcha.types.ts`：新增 `CaptchaIssuePayload { mode?: 'light' | 'dark'; dpr: number; env?: CaptchaEnv }`。
- `sdk/src/hooks/use-captcha-request.ts`：`get(env?: CaptchaEnv)` 改为 `axios.post(captchaEndpoint/${type}, payload)`，不再拼查询串；错误归一化逻辑不变；新增纯函数 `buildIssuePayload(option, env)` 便于测试。
- `sdk/src/Captcha.tsx`：`loadCaptcha` 流程改为 `envRef.current = telemetryEnabled ? collectEnv(telemetry) : undefined` → `get(envRef.current)`；题目到达后的 `useEffect` 不再重复采集 env（保留 `markLoaded` 与 PoW 启动），提交时仍把 `envRef.current` 放入 `payload.env`。
- `sdk/src/_internal/telemetry/env.ts`：`collectEnv` 保持同步；预留 `anomalies?: string[]` 字段位（Phase 6 填充），本阶段不采集。
- Vitest：`sdk/src/hooks/use-captcha-request.test.ts`（`buildIssuePayload`、端点拼接、`isCaptchaErrorResponse`）、`sdk/src/_internal/telemetry/env.test.ts`（用 `vi.stubGlobal` 伪造 `window` / `navigator` / `document`，覆盖 `webdriver`、`automation` 标记、`fp` 稳定性、`off` 返回 undefined）。若 `node` 环境下 stub 过于笨重，允许新增开发依赖 `happy-dom` 并在测试文件顶部用 `// @vitest-environment happy-dom`。
- `sdk/playground`：演示站无需改动，但需手动确认出题请求已变为 `POST`。

#### Out of Scope
- 新异常检测器（Phase 6）。
- README「前端接入」文案（Phase 9）。
- `telemetry` 级别语义变更。

#### Inputs
- Phase 2 的 `POST /captchas/:type` 契约。
- `sdk/src/Captcha.tsx`（`loadCaptcha`、`confirm`、env/PoW 相关 `useEffect`）、`sdk/src/hooks/use-captcha-request.ts`、`sdk/src/Captcha.types.ts`、`sdk/src/_internal/telemetry/env.ts`、`sdk/vitest.config.ts`。

#### Execution Requirements
- 刷新题目（`refresh`）时重新调用 `collectEnv`，使 `viewport` 与 `visibilityChanges` 为最新值。
- `telemetry='off'` 时 `POST` 不带 `env` 字段（不要发送 `env: undefined` 之外的占位值）。
- 不新增运行时依赖；`import type` 规范；Biome 通过。
- 保持 `getDevicePixelRatio()` 逻辑与 `dpr` 字段取整规则不变。

#### Expected Outputs
- 修改：`sdk/src/Captcha.tsx`、`sdk/src/hooks/use-captcha-request.ts`、`sdk/src/Captcha.types.ts`、`sdk/src/_internal/telemetry/env.ts`
- 新增：`sdk/src/hooks/use-captcha-request.test.ts`、`sdk/src/_internal/telemetry/env.test.ts`
- 可能修改：`sdk/package.json`（仅 devDependencies）、`sdk/vitest.config.ts`

#### Acceptance Criteria
- [x] `pnpm test`：`buildIssuePayload({type:'auto', mode:'dark'}, env)` 产出 `{mode:'dark', dpr:<1-3>, env}`；`telemetry='off'` 时 payload 无 `env` 键。（`vitest run`：2 个文件 13 个用例通过）
- [x] `pnpm typecheck && pnpm lint && pnpm build` 通过，`dist` 中不出现 `_internal` 的公共导出。（lint 0 错误、16 条既有 `useExhaustiveDependencies` 告警均在未改动的文件或未改动的行；`dist/index.d.ts` 无 `_internal`）
- [x] 本地演示站对接开发后端：浏览器 Network 面板中出题请求为 `POST`，请求体含 `env`，响应 `difficulty` 为 3；通过一次后再次出题仍为 3。（Playwright 驱动 Chromium 访问 `localhost:8010`：`POST /api/v1/captchas/slide-text` 请求体键 `dpr, mode, env`，响应 `difficulty 3`；仿真人拖动通过滑块拿到 `second_key`，提交体含 `point, trace(53 点, untrusted 0), env, pow`；再次出题 `difficulty 3`。该 Chromium 报 SwiftShader 软件渲染器，环境分 25 未达加难阈值，与 VDI 场景预期一致）
- [x] 旧流程回归：点选、滑块、区域滑块、旋转四种题型在演示站均可正常出题与通过。（四种题型按钮各触发一次 POST 200，响应含 image / thumb，弹窗标题正确；"通过"以滑块题真实通过 + 后端四种题型校验单测为证）

#### Risks
- `collectEnv` 在 SSR 场景（`window` 未定义）返回 undefined，已有判断；`get()` 需容忍 `env` 为 undefined。
- 预检请求增加一次往返：出题接口已是跨域 `POST /attempts` 同源策略，`CORS_MAX_AGE` 默认 24h 可缓存预检，影响可忽略。

#### Dependencies
Phase 2。

#### Notes
- SDK 版本号按 semver 升 minor（出题协议新增，非破坏）；是否在本阶段改 `package.json` 版本由用户在提交时决定，台账不强制。
- 执行记录（2026-10-26）：
  - Chrome 扩展未连接，浏览器核对改用 Playwright MCP 的 Chromium（`navigator.webdriver` 为 false，WebGL 为 SwiftShader）；后端与演示站分别以 `./run-dev.sh`（3321）和 `vite --port 8010` 临时实例启动，未占用用户常用的 8000。
  - 测试在 `node` 环境用 `vi.stubGlobal` 伪造 `window` / `navigator` / `document`，未新增 `happy-dom`（Q6 采用默认 A）；`env.ts` 的模块级缓存通过 `vi.resetModules()` 隔离。
  - `CaptchaEnv.anomalies` 类型字段已预留，Phase 6 采集。

---

### Phase 4: 服务端请求头核验与跨层一致性

#### Status
COMPLETED（2026-10-26）

#### Goal
用服务端能独立观测的 HTTP 请求头判断"是否像真实浏览器"，并与自报 env 做跨层比对；不像浏览器或自相矛盾的请求按分值加难。`GET` 与 `POST` 出题、校验提交三条路径都做核验，真实浏览器中的旧版 SDK 不受影响。

#### Scope
- `backend/risk/request.go`（新建，含 `import _ "time/tzdata"`）：
  - 填充 `RequestSignals{UserAgent, AcceptLanguage, SecFetchMode, SecFetchSite, SecFetchDest, SecChUA, SecChUAMobile, SecChUAPlatform string; DPRParam float64; TLSFingerprint, HTTP2Fingerprint string（Phase 7 使用）}`。
  - `EvaluateRequest(sig *RequestSignals, env *Env) RequestResult{Score int; Reasons []string; Consistent bool}`，规则与分值：
    - `non_browser_ua` +20：UA 为空或不含 `Mozilla/`（curl、Python requests、Go http 的默认 UA 都会命中）。
    - `sec_fetch_missing` +25：UA 含 `Chrome/` 或 `Firefox/` 且无 `Sec-Fetch-Mode`。
    - `client_hints_missing` +15：UA 为 Chromium 且主版本 ≥ 89 且无 `Sec-CH-UA`（Brave 等隐私浏览器可能不发送，故权重保守）。
    - `client_hints_mismatch` +25：`Sec-CH-UA-Mobile: ?1` 与 UA 是否含 `Mobile` 不一致；`Sec-CH-UA-Platform` 与 UA 操作系统标记不一致（Windows / macOS / Linux / Android / iOS / Chrome OS 归一化比较）。
    - `accept_language_missing` +15：无 `Accept-Language`。
    - 以下需 `env != nil`：`ua_mismatch` +30（`env.UA` 与头不等）；`language_mismatch` +10（`env.Languages[0]` 主语言子标签不在 `Accept-Language` 中）；`platform_mismatch` +20（`env.Platform` 归一化后与 `Sec-CH-UA-Platform` 不一致）；`mobile_mismatch` +20（`env.Mobile` 与 `Sec-CH-UA-Mobile` 均存在且不一致）；`tz_offset_mismatch` +15（`time.LoadLocation(env.Timezone)` 当前偏移分钟数 ≠ `-env.TzOffset`，时区名无法加载时跳过）；`dpr_mismatch` +10（`round(DPRParam)` ≠ `round(env.DPR)`）。
    - `Consistent` 为真当且仅当：无任何上述原因命中；`Accept-Language` 存在；UA 为 Chrome / Firefox 时 `Sec-Fetch-Mode` 存在；UA 为 Chromium ≥ 89 时 `Sec-CH-UA` 存在。
  - `EvaluateEnv(env, req)` 内部调用 `EvaluateRequest` 并把其分值与原因并入环境侧（受同一上限约束）；`env == nil` 时只跑不依赖 env 的规则。
- `backend/helper/utils.go`：新增 `RequestSignalsFrom(c *gin.Context, dpr float64) *risk.RequestSignals`。
- `backend/controllers/captcha_controller.go`：出题（`GET` 与 `POST`）与校验均填充 `Request`；`GET` 路径 `Env == nil`，只跑请求侧规则并按分值加难。
- `backend/services/challenge_policy.go`：`IssueDecision.Consistent` 取 `RequestResult.Consistent`，仅写入遥测，不影响等级。
- `backend/services/risk_engine.go`：`EvaluationContext` 新增 `Request`，遥测记录追加 `requestReasons`、`consistent`。
- 既有 httptest 用例：新增测试辅助 `withBrowserHeaders(req)`（设置 Chrome UA、`Accept-Language`、`Sec-Fetch-*`、`Sec-CH-UA*`），凡断言默认难度的既有用例改用该辅助发请求。这是行为变更（无 UA 的请求从本阶段起会被加难），在 Change Log 记录。
- 测试：`backend/risk/request_test.go`（表驱动，每条规则至少一正一反）、`backend/tests/security_test.go` 新增 `TestIssueEndpoint_CurlLikeHeadersEscalate`、`TestIssueEndpoint_BrowserLikeHeadersStayDefault`、`TestGetEndpoint_NonBrowserUAEscalates`。

#### Out of Scope
- TLS / HTTP2 指纹的读取与规则（Phase 7，仅预留字段）。
- 行为规则改动。

#### Inputs
- Phase 1 的 `EvaluateEnv` / `RequestSignals` 占位；Phase 2 的控制器与 `IssueInput`。
- `backend/helper/utils.go`、`backend/helper/helper.go`（`GetRealClientIP` 的可信代理判定逻辑）。
- Chromium Client Hints 行为：89+ 默认在所有请求（含跨域 fetch）发送低熵提示 `Sec-CH-UA`、`Sec-CH-UA-Mobile`、`Sec-CH-UA-Platform`；`Sec-Fetch-*` 由 Chrome 76+、Firefox 90+、Safari 16.4+ 发送。

#### Execution Requirements
- UA 解析只做子串与主版本号提取，不引入 UA 解析库。
- Safari / WebView / 未知 UA 不因缺少 `Sec-CH-UA` 或 `Sec-Fetch-*` 扣分（规则只对明确声明 Chrome / Firefox 的 UA 生效）。
- `tz_offset_mismatch` 需考虑 JS `getTimezoneOffset()` 符号相反（UTC+8 为 -480）。
- 所有规则 reason 命名沿用 `snake_case(+分值)` 格式，便于日志对照。

#### Expected Outputs
- 新增：`backend/risk/request.go`、`backend/risk/request_test.go`
- 修改：`backend/risk/score.go`、`backend/risk/types.go`、`backend/helper/utils.go`、`backend/controllers/captcha_controller.go`、`backend/services/challenge_policy.go`、`backend/services/risk_engine.go`、`backend/tests/security_test.go`

#### Acceptance Criteria
- [x] 冒充 Chrome UA 但无 `Sec-Fetch-*`、无 `Sec-CH-UA`、无 `Accept-Language` 的 `POST` 出题：原因含 `sec_fetch_missing`、`client_hints_missing`、`accept_language_missing`，`difficulty` 为 5。（`TestIssueEndpoint_FakeChromeWithoutBrowserHeadersEscalates`）
- [x] `curl` 默认请求头（UA `curl/x.y`、无 `Accept-Language`）的 `GET` 出题：原因含 `non_browser_ua`、`accept_language_missing`，`difficulty` 为 4。（`TestGetEndpoint_NonBrowserUAEscalates`，并断言遥测 `consistent=false`、`viaPost=false`）
- [x] 带完整 Chrome 请求头 + 一致 env 的 `POST` 出题：遥测 `consistent=true`，`difficulty` 为 3。（`TestIssueEndpoint_BrowserLikeHeadersStayDefault`）
- [x] 带完整 Chrome 请求头、无 env 的 `GET` 出题（旧版 SDK 在真实浏览器）：`difficulty` 为 3。（`TestIssueEndpoint_EmptyBodyMatchesGet`）
- [x] 请求头为 Chrome 但 `env.ua` 为 Firefox：命中 `ua_mismatch`，`difficulty` 为 4。（`TestIssueEndpoint_UAMismatchEscalates`）
- [x] `env.tz="Asia/Shanghai"` 且 `tzOffset=-480` 不命中；`tzOffset=0` 命中 `tz_offset_mismatch`；`env.tz="UTC"`、`tzOffset=0`（Firefox resistFingerprinting）不命中。（`risk/request_test.go`：`TestEvaluateRequest_CrossLayerRules`、`TestTimezoneMatches_UsesEmbeddedTzdata`）
- [x] Safari 17 UA 且无 `Sec-CH-UA` 不命中 `client_hints_missing`。（`TestEvaluateRequest_HeaderRules` 的 Safari 用例，Firefox 用例另验证只要求 Sec-Fetch）
- [x] 在 `alpine` 镜像内 `time.LoadLocation("Asia/Shanghai")` 成功。（`risk/request.go` 导入 `_ "time/tzdata"`；`TestTimezoneMatches_UsesEmbeddedTzdata` 断言 `LoadLocation` 不报错。未做 docker build 实跑，镜像内验证留待 Phase 9）
- [x] `go test ./...` 通过。（除 Q8 两个既有失败外全部 ok，见 Phase 5 结束时的全量运行记录）

#### Risks
- 企业代理或 WAF 可能剥离 `Sec-CH-UA`：权重已保守（+15），单独命中达不到加难阈值 30，也不触发拦截。
- 浏览器版本演进可能改变头发送策略：规则集中在一个文件，便于调整。
- 既有测试需补浏览器请求头辅助，改动面较大但机械：先跑一遍测试列出失败用例再逐个套用辅助。

#### Dependencies
Phase 2。

#### Notes
- `Consistent` 本阶段只做记录，为将来若要评估"降级"可行性积累数据，不参与任何决策。
- 执行记录（2026-10-26）：
  - 规则实现于 `backend/risk/request.go`，共 12 条：`non_browser_ua` 20、`sec_fetch_missing` 25、`client_hints_missing` 15、`client_hints_mismatch` 25、`accept_language_missing` 15、`ua_mismatch` 30、`language_mismatch` 10、`platform_mismatch` 20、`mobile_mismatch` 20、`tz_offset_mismatch` 15、`dpr_mismatch` 10；平台归一化覆盖 Windows / macOS / Linux / Android / iOS / Chrome OS，Android 上 `navigator.platform` 为 `Linux armv8l` 时结合 UA 纠正。
  - `dpr_mismatch` 比较前两边都按 SDK 的 `getDevicePixelRatio` 规则四舍五入并裁剪到 1-3，避免 3.5 之类的真实 DPR 误报。
  - 既有用例改造：`tests/browser_headers_test.go` 提供 `browserUA(tag)` 与 `withBrowserHeaders(req, ua)`；`TestRouter_SharedEgressEscalatesInsteadOfBlocking` 与全部 `TestIssueEndpoint_*` 改为经过该辅助；`cleanIssueEnvFor(ua)` 让 env.UA 与请求头一致。
  - 校验提交（`/attempts`）同样传入 `Request`（`DPRParam` 为 0，跳过 dpr 规则），遥测 `kind=verify` 条目新增 `consistent`、`requestReasons`。

---

### Phase 5: 指纹重放计数

#### Status
COMPLETED（2026-10-26）

#### Goal
识别"同一份 env 被复制到多个客户端重放"的行为：同一 `fp` 哈希在时间窗内对应的不同客户端数超过阈值即加难。

#### Scope
- `backend/config/config.go`、`backend/.env.example`：新增 `RiskConfig.FPReplayThreshold`（`RISK_FP_REPLAY_THRESHOLD`，默认 5，0 关闭）、`RiskConfig.FPReplayWindow`（`RISK_FP_REPLAY_WINDOW`，默认 `10m`）。
- `backend/services/client_risk.go`：新增 `TouchFingerprint(fp, clientHash string) int`：Lua 脚本原子执行 `SADD captcha:fp:<fp> clientHash` + `PEXPIRE` + `SCARD`；集合大小达到 `threshold + 1` 后不再 `SADD`（参照 `IPPressureSaturation` 的饱和思路，防止集合膨胀）；Redis 出错返回 0（fail-open）。
- `backend/services/challenge_policy.go`：`Decide` 使用 `in.FPReplays`（Phase 1 已预留），`> threshold` 时追加 `fp_replayed(+30)`，单独命中即加 1 级。
- `backend/controllers/captcha_controller.go`：`POST` 出题且 `env.Fingerprint != ""` 时调用 `TouchFingerprint`，结果填入 `IssueInput.FPReplays`；校验阶段不重复计数。
- 测试：`backend/tests/security_test.go` 新增 `TestFingerprintReplay_*`（依赖 Redis）。

#### Out of Scope
- 按 fp 拒绝或封禁。
- 跨 fp 的设备聚类。

#### Inputs
- Phase 2 的 `POST` 出题流程与 `IssueInput`。
- `backend/services/client_risk.go`（`recordScript` 的 Lua 脚本风格）、`backend/services/rate_limiter.go`（饱和计数思路）。

#### Execution Requirements
- key 命名 `captcha:fp:<fp>`，与现有 `captcha:client:*`、`captcha:ratelimit:*` 并列。
- 同一 clientHash 重复出题不增加计数（SET 语义天然保证）。
- 手机切换网络导致 IP 变化会产生新的 clientHash：默认阈值 5 / 10 分钟对此足够宽容，文档中说明。

#### Expected Outputs
- 修改：`backend/config/config.go`、`backend/.env.example`、`backend/services/client_risk.go`、`backend/services/challenge_policy.go`、`backend/controllers/captcha_controller.go`
- 测试：`backend/tests/security_test.go`、`backend/config/config_test.go`

#### Acceptance Criteria
- [x] 同一 fp 用 6 个不同 clientHash 各出题一次，第 6 次起 `Decide` 原因含 `fp_replayed`，`difficulty ≥ 4`；前 5 次 `difficulty` 为 3。（`tests/fingerprint_replay_test.go`：`TestFingerprintReplay_DistinctClientsEscalate`，另断言饱和后集合成员数停在阈值 + 1）
- [x] 同一 clientHash 用同一 fp 出题 20 次，计数保持 1，`difficulty` 保持 3。（`TestFingerprintReplay_SameClientRepeatsStayDefault`，并断言 key 的 TTL 不超过 `RISK_FP_REPLAY_WINDOW`）
- [x] `RISK_FP_REPLAY_THRESHOLD=0` 时不读写 Redis fp 集合。（`services/fingerprint_replay_test.go`：`TestTouchFingerprint_DisabledOrEmptyInputsSkipRedis` 用指向 `127.0.0.1:1` 的客户端证明阈值 0、空 fp 时不触达 Redis；`TestDecide_FingerprintReplay` 断言阈值 0 时 `FPReplays=99` 也不加难）
- [x] Redis 不可用时出题仍成功，日志有一条警告。（`TestTouchFingerprint_FailOpen`：不可达的 Redis 客户端返回 0、1 秒内返回；控制器据此按未重放处理）
- [x] `go test ./...` 通过。（`tests` 包连续两次 `-count=1` 运行均只剩 Q8 两个既有失败）

#### Risks
- 大量不同 fp 造成 key 数量增长：每个 key 有 `FPReplayWindow` 过期，且只在 `POST` 带 fp 时写入。

#### Dependencies
Phase 2。

#### Notes
- `fp` 由前端 SHA-256 截断 32 位十六进制，服务端不解析其含义。
- 执行记录（2026-10-26）：
  - `ServiceManager` 接口新增 `GetClientRiskStore()`（实现早已存在），控制器据此持有 `clientRisk` 调用 `TouchFingerprint`。
  - Lua 脚本：集合大小达到 `阈值 + 1` 后不再 `SADD`，但对不在集合里的新客户端仍返回 `count + 1`，保证持续重放时判定不变而集合不膨胀。
  - 测试稳定性修复：接口测试此前共用默认出口 `192.0.2.1` 与同一 fp，单 IP 加难计数会跨用例、跨次运行累积而让断言默认难度的用例时而失败。现在 `remoteAddrFor(seed)` 由带唯一后缀的 UA 派生 `198.18.x.y` 出口地址，`cleanIssueEnvFor(ua)` 的 fp 由 UA 派生，各用例互不影响。

---

### Phase 6: SDK headless 痕迹增补与服务端映射

#### Status
COMPLETED（2026-10-26）

#### Goal
借鉴 BotD / CreepJS 的思路，在 SDK 内置一组轻量的无头浏览器与 API 篡改检测，作为 `env.anomalies` 标记上报；服务端映射为环境侧分值。只加难，不拦截。

#### Scope
- `sdk/src/_internal/telemetry/anomalies.ts`（新建）：`collectAnomalies(budgetMs = 150): Promise<string[]>`，所有检测并行、超时即放弃、异常静默。检测器与标记名：
  - `zero_outer_size`：`window.outerWidth === 0 || window.outerHeight === 0`。
  - `permission_mismatch`：`navigator.permissions.query({name:'notifications'})` 为 `denied` 而 `Notification.permission` 为 `default`。
  - `no_media_devices`：`navigator.mediaDevices.enumerateDevices()` 返回空数组（仅在 API 存在时判断）。
  - `worker_ua_mismatch` / `worker_cores_mismatch` / `worker_platform_mismatch`：用 `Blob` URL 内联 Worker 读取 `navigator.userAgent`、`hardwareConcurrency`、`platform` 与主线程比对；CSP 禁止 `blob:` worker 或 100ms 内无响应时跳过。
  - `getter_patched:<prop>`：`Object.getOwnPropertyDescriptor(Navigator.prototype, prop).get` 的 `Function.prototype.toString` 结果不含 `[native code]`，prop 取 `webdriver`、`plugins`、`languages`、`hardwareConcurrency`、`userAgent`。
  - `chrome_object_missing`：UA 为桌面 Chrome 且 `window.chrome` 不存在。
  - 结果去重、最多 16 项、单项 ≤ 64 字符。
- `sdk/src/_internal/telemetry/env.ts`：`collectEnv` 保持同步；新增 `collectEnvWithAnomalies(level, budgetMs): Promise<CaptchaEnv | undefined>`，`level !== 'off'` 时合并 `anomalies`。
- `sdk/src/Captcha.tsx`：出题前改为 `await collectEnvWithAnomalies(telemetry, 150)`。
- `sdk/src/Captcha.types.ts`：`CaptchaEnv.anomalies?: string[]`。
- `sdk/THIRD_PARTY_NOTICES.md`（新建）：BotD（MIT）、CreepJS（MIT）版权与许可全文，注明"检测思路借鉴，代码为本项目实现"；`anomalies.ts` 文件头注释引用该文件。
- 后端：`backend/risk/types.go` `Env.Anomalies []string`；`backend/models/request.go` `validateEnv` 增加 ≤ 16 项、单项 ≤ 64 字符校验；`backend/risk/score.go` `EvaluateEnv` 增加权重表：`zero_outer_size` +20、`permission_mismatch` +15、`no_media_devices` +10、`worker_*_mismatch` +30（`HardMarker=true`）、`getter_patched:*` +30（`HardMarker=true`）、`chrome_object_missing` +10；未知标记忽略；总分受 `RISK_ENV_SCORE_CAP` 约束。
- 测试：`sdk/src/_internal/telemetry/anomalies.test.ts`（stub 全局对象逐项触发与不触发；Worker 不可用时无标记）、`backend/risk/score_test.go` 增加权重与上限用例、`backend/tests/models_test.go` 校验上限。

#### Out of Scope
- Canvas / Audio 指纹（隐私敏感且在 Brave 等浏览器被扰动，误报高）。
- 任何会弹出权限请求的 API 调用（`getUserMedia`、`Notification.requestPermission` 等）。

#### Inputs
- Phase 3 的前置采集流程；Phase 4 的 `EvaluateEnv` 结构。
- BotD 检测点清单（Headless Chrome / Firefox、Selenium、Playwright、PhantomJS、Nightmare、Electron、SlimerJS）与 CreepJS 的"谎言检测"思路（原型篡改、Worker 与主线程不一致）。

#### Execution Requirements
- 全部检测在 150ms 预算内并行执行，`Promise.race` 超时后以已完成结果为准。
- 不新增运行时依赖；Worker 代码以字符串内联，构建后不产生额外 chunk。
- `permission_mismatch` 在 Safari（无 `Notification` 或 `permissions.query` 不支持 `notifications`）时跳过。
- 服务端对 `anomalies` 中的未知标记只记录到遥测日志，不计分。

#### Expected Outputs
- 新增：`sdk/src/_internal/telemetry/anomalies.ts`、`sdk/src/_internal/telemetry/anomalies.test.ts`、`sdk/THIRD_PARTY_NOTICES.md`
- 修改：`sdk/src/_internal/telemetry/env.ts`、`sdk/src/_internal/telemetry/index.ts`、`sdk/src/Captcha.tsx`、`sdk/src/Captcha.types.ts`、`backend/risk/types.go`、`backend/risk/score.go`、`backend/models/request.go`
- 测试：`backend/risk/score_test.go`、`backend/tests/models_test.go`

#### Acceptance Criteria
- [x] Vitest：`outerWidth=0` 产生 `zero_outer_size`；Worker 构造抛错时无 `worker_*` 标记；`getter_patched` 对被覆盖的 `navigator.webdriver` getter 触发、对原生 getter 不触发；总耗时断言 < 200ms。（`anomalies.test.ts` 10 个用例：node 环境无 `Worker` 即为"不可用静默跳过"；超时用例断言 120ms 预算内返回已完成结果且耗时 < 200ms）
- [x] Go：`anomalies=["worker_ua_mismatch"]` 使 `HardMarker=true`；6 个标记叠加后 `EnvScore ≤ RISK_ENV_SCORE_CAP`；17 个标记返回 400。（`TestEvaluateEnv_AnomalyWeights`、`TestEvaluate_AnomaliesStayUnderCap`、`TestIssueRequest_Validate_Anomalies`）
- [x] 真实浏览器出题时 `anomalies` 为空、`difficulty` 仍为 3。（Playwright MCP 的 Chromium 未开自动化标记：POST 请求体无 `anomalies` 字段，遥测 `anomalies: null`、`reasons: [software_renderer(+25)]`、`difficulty 3`。该实例 outerWidth 1280×720、有 window.chrome、有虚拟媒体设备，6 类检测均未误报。真实 headed Chrome / Firefox 待 Phase 9 由用户复核）
- [x] Playwright 无头 Chromium 打开演示站出题：`anomalies` 非空或 `webdriver=true`，`difficulty ≥ 4`。（通过 CDP `Emulation.setAutomationOverride` 开启 Chromium 自带的 `navigator.webdriver=true`，即默认配置的 Playwright / Selenium 状态：遥测 `reasons: [webdriver(+45), software_renderer(+25)]`、envScore 60、`difficulty 5`）
- [x] `pnpm test && pnpm typecheck && pnpm lint && pnpm build` 与 `go test ./...` 通过。（SDK：3 个文件 23 个用例通过、lint 0 错误、typecheck 通过、build 完成；Go 全量除 Q8 两项外全部 ok）

#### Risks
- 某些企业浏览器策略禁用 Worker 或 `permissions` API：全部设计为"不可用即跳过"，不产生标记。
- 新版无头 Chrome 已修复部分经典痕迹（如 `window.chrome`）：权重设低，主要依赖 Worker 一致性与 getter 篡改检测。

#### Dependencies
Phase 3、Phase 4。

#### Notes
- 本阶段的所有标记都是负面信号，只会加难；没有命中任何标记的客户端难度不变。
- 执行记录（2026-10-26）：
  - 权重调整：`software_renderer` 命中时不再叠加 `no_media_devices`。VDI / 虚拟机本来就没有摄像头与麦克风，两条规则指向同一正常原因，叠加（25 + 10 = 35）会把 VDI 用户推过加难阈值，违背"VDI 不高于 3 级"的成功标准；没有软件渲染器时 `no_media_devices` 仍按 10 分计。
  - `getter_patched:<prop>` 无论命中几个属性只计一次 30 分（硬性标记）；实例自有属性与原型 getter 非原生两种篡改方式都覆盖。
  - 检测结果在页面生命周期内缓存（`resetAnomaliesCache` 仅供测试），刷新题目不会重复起 Worker。
  - Biome 会把 `hasOwnProperty.call` 自动改成 ES2022 的 `Object.hasOwn`，与 `lib: ES2020` 冲突，改用 `Object.getOwnPropertyDescriptor(navigator, prop) !== undefined`。
  - 测试里 navigator 的取值放在 `Navigator.prototype` 与实例之间的一层原型上，与真实浏览器"属性来自原型链、实例无自有属性"的结构一致，否则 stub 自身会被判为篡改。

---

### Phase 7: TLS / HTTP2 指纹读头与部署示例（可跳过）

#### Status
COMPLETED（2026-10-26）

#### Goal
支持从可信代理注入的请求头读取 JA3 / JA4 / HTTP2 指纹，记录到遥测并按可配置列表计分；提供 fingerproxy 部署示例。本阶段可整体跳过，不影响其他阶段。

#### Scope
- `backend/config/config.go`、`backend/.env.example`：新增 `RiskConfig.TLSFingerprintHeader`（`RISK_TLS_FINGERPRINT_HEADER`，默认空 = 关闭）、`RiskConfig.HTTP2FingerprintHeader`（`RISK_HTTP2_FINGERPRINT_HEADER`，默认空）、`RiskConfig.TLSFingerprintList`（`RISK_TLS_FINGERPRINT_LIST`，JSON 文件路径，默认空）。
- `backend/helper/helper.go`：新增 `IsTrustedProxyRequest(c) bool`，复用 `GetRealClientIP` 对直连来源与 `TRUSTED_PROXIES` 的判定。
- `backend/helper/utils.go`：`RequestSignalsFrom` 仅在 `IsTrustedProxyRequest` 为真且配置了头名时填充 `TLSFingerprint` / `HTTP2Fingerprint`。
- `backend/risk/tlsfp.go`（新建）：`FingerprintList{Browser map[string]string; Deny []string}` 从 JSON 加载；`EvaluateTLS(sig, list) (reasons, score)`：`tls_denylisted` +40（`HardMarker=true`）；`tls_ua_family_mismatch` +30（指纹在 `Browser` 表中且家族与 UA 家族不符）；未知指纹中性。并入 `EvaluateRequest`。
- 遥测：出题与校验记录追加 `tls`、`h2` 字段（原值）。
- `deploy/fingerproxy/docker-compose.yml`、`deploy/fingerproxy/README.md`：fingerproxy 终结 TLS 并转发到 `captcha:8080`，示例证书挂载路径，`TRUSTED_PROXIES` 设置为 compose 网段，头名以所用 fingerproxy 版本 README 为准（默认 `X-JA3-Fingerprint`、`X-JA4-Fingerprint`、`X-HTTP2-Fingerprint`）。
- `deploy/fingerproxy/tls-fingerprints.example.json`：空 `Browser`、空 `Deny` 的模板，附说明如何从遥测日志中收集。
- 测试：`backend/risk/tlsfp_test.go`；`backend/tests/security_test.go` 新增"非可信来源携带指纹头被忽略"与"可信来源命中 deny 列表加分"。

#### Out of Scope
- 在 Go 进程内直接解析 ClientHello（标准库拿不到扩展顺序，不做）。
- 维护一份随浏览器版本更新的指纹数据库（由运维按遥测日志自行维护）。

#### Inputs
- Phase 4 的 `RequestSignals` 预留字段与 `EvaluateRequest`。
- `backend/helper/helper.go` 可信代理判定、`backend/config/config.go` `Security.TrustedProxies`。
- fingerproxy 项目（Apache-2.0）README。

#### Execution Requirements
- 未配置头名时整条链路零开销、零日志。
- 列表文件启动时加载一次，解析失败则启动报错（与其它配置校验一致）。
- 不新增 Go 依赖。

#### Expected Outputs
- 新增：`backend/risk/tlsfp.go`、`backend/risk/tlsfp_test.go`、`deploy/fingerproxy/docker-compose.yml`、`deploy/fingerproxy/README.md`、`deploy/fingerproxy/tls-fingerprints.example.json`
- 修改：`backend/config/config.go`、`backend/.env.example`、`backend/helper/helper.go`、`backend/helper/utils.go`、`backend/risk/request.go`、`backend/services/risk_engine.go`

#### Acceptance Criteria
- [x] 直连来源不在 `TRUSTED_PROXIES` 时，携带 `X-JA4-Fingerprint` 的请求 `RequestSignals.TLSFingerprint` 为空。（`tests/tls_fingerprint_test.go`：`TestTLSFingerprint_IgnoredFromUntrustedRemote`、`TestIsTrustedProxyRequest`）
- [x] 可信来源携带 deny 列表中的指纹：原因含 `tls_denylisted`，`HardMarker=true`。（`risk/tlsfp_test.go`：`TestEvaluateRequest_TLSRules`；接口层 `TestTLSFingerprint_DeniedFromTrustedProxyEscalates` → difficulty 4）
- [x] 可信来源指纹在 `Browser` 表中标为 `firefox` 而 UA 为 Chrome：命中 `tls_ua_family_mismatch`。（`TestTLSFingerprint_FamilyMismatchEscalates`，家族一致时保持 3）
- [x] 未配置 `RISK_TLS_FINGERPRINT_HEADER` 时，遥测记录无 `tls` 字段。（`TestTLSFingerprint_HeaderDisabledByDefault`；另有 `TestTLSFingerprint_RecordedWithoutList` 验证无列表时只记录不计分）
- [x] `docker compose -f deploy/fingerproxy/docker-compose.yml up` 后，用 `curl -k` 出题，服务日志中的出题记录带 `tls` 字段。（镜像从 Go 模块代理构建 fingerproxy v1.1.0；`FINGERPROXY_USER=$(id -u) FORWARD_URL=http://host.docker.internal:3321 docker compose up -d fingerproxy` 后 `curl -k -d '{"dpr":2}' https://127.0.0.1:8443/api/v1/captchas/click-text` 返回 200，后端日志 `出题评估` 记录含 `"tls":"t13i3111h2_e8f1e7e78f70_b26ce05bbdd6"` 与 `"h2":"3:100;4:10485760;2:0|1048510465|0|m,s,a,p"`，来源 IP 172.20.0.1 命中默认可信网段，curl 因 `non_browser_ua` + `accept_language_missing` 拿到 4 级）
- [x] `go test ./...` 通过。（`-count=1` 全量运行除 Q8 两项外全部 ok）

#### Risks
- 用户部署在 Cloudflare 等 WAF 之后时本阶段意义不大：README 中说明此时直接用 WAF 的 bot score 并通过自定义头传入同一机制。

#### Dependencies
Phase 4。

#### Notes
- 本阶段默认关闭，不影响任何验收标准中的"真人保持 3 级"结论。
- 执行记录（2026-10-26）：
  - fingerproxy 上游不发布容器镜像，main 包在模块的 `cmd` 目录（`go install github.com/wi1dcard/fingerproxy/cmd@v1.1.0`，产物名为 `cmd`），Dockerfile 里改名为 `fingerproxy`。全部参数支持同名环境变量：`LISTEN_ADDR`、`FORWARD_URL`、`CERT_FILENAME`、`CERTKEY_FILENAME`、`PRESERVE_HOST`。
  - 镜像以 uid 1001 运行，0600 的私钥读不到：compose 增加 `user: "${FINGERPROXY_USER:-1001}"`，本地联调用当前用户，正式环境 `chown 1001`。
  - `deploy/fingerproxy/certs/` 加入 `.gitignore`（保留 `.gitkeep`），自签证书不入库。
  - 配置校验阶段即读取并解析指纹列表文件；列表同时要求至少配置一个指纹头名，否则报错提示。
  - 指纹参考列表通过 `ServiceManager.GetTLSFingerprintList / SetTLSFingerprintList` 持有，控制器在提取请求信号后附上，测试可直接注入。
  - 首次 `curl -k` GET 在容器刚起时握手失败（返回 000），紧随其后的 POST 成功；README 的联调步骤已写明先确认容器日志出现 `server listening on :8443`。

---

### Phase 8: 管理后台风控观察页

#### Status
COMPLETED（2026-10-26）

#### Goal
在管理后台增加一个只读页面，展示最近的出题评估与校验评分（分数、原因、难度、是否通过），用于后续调权重；不做图表与筛选。

#### Scope
- 后端：
  - `backend/services/client_risk.go`：新增 `RecentTelemetry(limit int) ([]map[string]interface{}, error)`，`LRANGE captcha:telemetry 0 limit-1` 并反序列化；`limit` 上限 500，默认 100。
  - `backend/services/telemetry_mask.go`（新建）：`MaskTelemetry(entry)`：`ip` IPv4 保留前两段（`1.2.*.*`）、IPv6 保留前三组；`ua` 只保留浏览器家族与主版本及操作系统（如 `Chrome 130 / Windows`）；`client` 保留前 8 位；其余字段原样。
  - `backend/controllers/admin_controller.go`：新增 `RiskTelemetryHandler`，响应 `{ "items": [...] }`。
  - `backend/routes/router.go`：`ready.GET("/risk/telemetry", adminCtrl.RiskTelemetryHandler)`。
- 前端：
  - `sdk/playground/api.ts`：`adminApi.riskTelemetry(token, limit)`。
  - `sdk/playground/pages/RiskPage.tsx`（新建）：表格列为 时间 / 阶段（出题 / 校验） / 题型 / 难度（出题行显示 `level`，含 `base+boost` 悬浮提示） / 分数（校验行；出题行显示 `envScore`） / 结果（通过 / 失败 / —） / 原因（逐项换行） / 客户端；顶部"刷新"按钮与条数选择（50 / 100 / 200）；空态与加载态文案；会话过期沿用 `onSessionExpired`。
  - `sdk/playground/console/ConsoleView.tsx`：导航新增"风控观察"；`sdk/playground/styles.css` 复用现有表格样式，必要时新增原因列的换行样式。
- 测试：`backend/tests/admin_test.go` 新增未登录 401、初始密码未改 403、正常 200 且字段脱敏；`backend/services/telemetry_mask_test.go`。

#### Out of Scope
- 图表、筛选、搜索、导出。
- 修改遥测写入格式（除 Phase 1、2、4、7 已定义的字段）。

#### Inputs
- Phase 2 定义的 `kind=issue` 记录与既有 `kind=verify` 记录字段。
- `backend/controllers/admin_controller.go`、`backend/routes/router.go`（`ready` 组）、`backend/tests/admin_test.go`、`backend/tests/admin_helpers_test.go` 的登录与改密辅助。
- `sdk/playground/pages/AppsPage.tsx`（页面结构与 `token` / `onSessionExpired` 约定）、`sdk/playground/api.ts` `adminApi`、`sdk/playground/console/ConsoleView.tsx`。

#### Execution Requirements
- 接口受 `AdminAuthMiddleware` 与 `RequirePasswordChanged` 保护，与背景图、应用接口同级。
- 原始 IP / UA 不得出现在接口响应中（脱敏在服务端完成，不依赖前端）。
- 页面在 `pnpm start` 演示站与 `pnpm build:console` 内嵌版均可用。

#### Expected Outputs
- 新增：`backend/services/telemetry_mask.go`、`backend/services/telemetry_mask_test.go`、`sdk/playground/pages/RiskPage.tsx`
- 修改：`backend/services/client_risk.go`、`backend/controllers/admin_controller.go`、`backend/routes/router.go`、`backend/tests/admin_test.go`、`sdk/playground/api.ts`、`sdk/playground/console/ConsoleView.tsx`、`sdk/playground/styles.css`

#### Acceptance Criteria
- [x] `GET /api/v1/admin/risk/telemetry` 未带令牌 401；初始密码未改 403（`code 2008`）；正常返回最多 `limit` 条，`ip` 形如 `1.2.*.*`，`ua` 不含完整原始字符串。（`tests/admin_risk_test.go`：`TestAdmin_RiskTelemetryRequiresReadySession`、`TestAdmin_RiskTelemetryReturnsMaskedEntries`，后者断言整个响应体不含原始 IP 与 `AppleWebKit`）
- [x] `limit=1000` 被裁剪为 500；`limit` 缺省为 100。（`TestAdmin_RiskTelemetryLimitClampedAndDefaulted`；`limit=0` 视为缺省，负数 400）
- [x] 演示站登录后"风控观察"页能看到最近的出题与校验记录，原因逐项展示，刷新按钮生效。（Playwright 携带通过 API 登录拿到的会话令牌打开 `localhost:8010/#/console/risk`：导航出现「风控观察」，表头 8 列，100 行，首行 `校验 slide-text 30 通过 trace_missing(+30) f9482d5b 127.0.*.* · Chrome 130 / Windows`，页脚「共 100 条 · 更新于 …」；页面文本不含完整 UA）
- [x] `pnpm build:console` 后重新编译 Go 服务，`/console/` 内同样可见该页。（`127.0.0.1:3321/console/#/risk` 渲染结果与演示站一致，且页面文本不含任何原始 IPv4）
- [x] `go test ./...`、`pnpm typecheck && pnpm lint` 通过。（Go 全量除 Q8 两项外 ok；lint 0 错误 16 条既有告警；typecheck 通过）

#### Risks
- 遥测记录字段随阶段演进不一致（早期记录缺 `kind`）：前端把缺 `kind` 的记录视为校验记录。

#### Dependencies
Phase 2。

#### Notes
- 现有 `RiskEngine.Record` 写入的遥测条目包含原始 `ip` 与 `ua`，与 README「服务端只保存客户端哈希与评分，不落原始 IP / UA」的表述不一致；本阶段只在展示层脱敏，是否改为写入时脱敏见 Open Questions Q5，并在 Phase 9 修正文档措辞。
- 执行记录（2026-10-26）：
  - 脱敏放在服务端 `MaskTelemetry`：IPv4 保留前两段，IPv6 保留前 48 位（`net.ParseIP` 解析，`::ffff:` 映射的 IPv4 按 IPv4 处理），UA 压缩为「家族 主版本 / 操作系统」，非浏览器 UA 显示「非浏览器」，client 保留前 8 位；Q5 仍按推荐默认留待 Phase 9 文案处理（写入侧未改）。
  - `AdminController` 自行构造 `ClientRiskStore`（只依赖配置），`RegisterAdminRoutes` 签名不变，既有管理后台测试基架无需改动。
  - 前端表格用单类选择器（`risk-table__cell` 等）而非 `th` / `td` 后代选择器，避开 Biome 的 `noDescendingSpecificity`；行 key 用「题目 key + 阶段 + 时间 + 出现次数」而非数组下标。
  - 为了在浏览器里核对页面，本地开发环境的管理员密码经历了一次 `reset-admin-password` 与一次 API 改密（原本仍是从未修改过的随机初始密码）；Phase 9 结束时再执行一次 `reset-admin-password` 恢复为「随机初始密码」状态，新密码只写入会话临时目录的日志，不出现在对话里。

---

### Phase 9: 文档更新与手工端到端验收

#### Status
COMPLETED（2026-10-26）

#### Goal
把新协议、新信号、新配置、新页面写进 README 与配置示例，并按固定脚本做一次真人 / 脚本 / 旧版三条路径的端到端核对，把结果记入台账。

#### Scope
- `README.md`：
  - 「RESTful API 路由」表增加 `POST /api/v1/captchas/:type`（出题，携带环境信号）；`GET` 行注明"旧版 SDK 兼容，不携带环境信号，只做请求头核验"。
  - 「接口字段说明」增加 `POST /captchas/:type` 请求体示例（`mode`、`dpr`、`env` 含 `anomalies`）。
  - 「验证流程」补一步"出题前采集环境信号与请求头核验，决定首题是否加难"。
  - 「安全机制」表新增：出题前环境评估与逐级加难（真人保持默认难度）、请求头核验与跨层一致性、指纹重放计数、headless 痕迹检测、TLS / HTTP2 指纹（可选）；修订「行为/环境风控」行为"拦截需行为证据或硬性标记"；修订「采集级别与隐私」说明 `anomalies` 与 Worker 一致性检测，按 Q5 决议修正"不落原始 IP / UA"的表述。
  - 「环境变量参考 / 安全与风控」新增 `RISK_ENV_SCORE_CAP`、`RISK_BLOCK_MIN_BEHAVIOR`、`RISK_FP_REPLAY_THRESHOLD`、`RISK_FP_REPLAY_WINDOW`、`RISK_TLS_FINGERPRINT_HEADER`、`RISK_HTTP2_FINGERPRINT_HEADER`、`RISK_TLS_FINGERPRINT_LIST`；说明 `RISK_TELEMETRY_LOG_SIZE` 现在同时容纳出题与校验记录。
  - 「管理后台」增加"风控观察"小节。
  - 二级标题不改名；若新增二级标题，同步 `sdk/playground/docs/content.ts` 的 `CURATED_GROUPS`。
- `backend/.env.example`：确认各阶段新增变量均已以注释形式列出并有一行中文说明。
- `AGENTS.md`：目录树补 `backend/risk/`（人机判别纯计算）与 `deploy/`（可选的指纹代理示例）。
- `sdk/THIRD_PARTY_NOTICES.md` 在 README「License」处加一句引用。
- 手工端到端核对（结果写入本台账 Change Log）：
  1. 启动 Redis、`./run-dev.sh`、`pnpm start`。
  2. 真实浏览器首次出题：Network 中为 `POST`，响应 `difficulty=3`；通过一次后再出题仍为 `difficulty=3`。
  3. `curl GET`（默认 UA）出题 `difficulty=4`，日志含 `non_browser_ua`；`curl POST` 携带从浏览器复制的 env 与 curl 默认请求头，`difficulty ≥ 4`，日志含 `ua_mismatch`。
  4. 同一份 env 用 6 个不同 UA 的 `curl POST` 出题，日志出现 `fp_replayed`，`difficulty ≥ 4`。
  5. 模拟 VDI：`curl POST` 带完整浏览器请求头与 env（`webgl="llvmpipe"`、`touchPoints=0`、`anomalies=["no_media_devices"]`，`plugins` 保持真实 Chrome 的 5），`difficulty=3`（环境分 25 未达加难阈值；执行时修正：`plugins=0` 配桌面 Chrome UA 本身就是既有的可疑信号 `desktop_chrome_without_plugins(+10)`，真实 VDI 里的 Chrome / Firefox 都带 5 个 PDF 插件，原脚本的 `plugins=0` 不符合实际）。
  6. 连续答错两次后再出题，难度回升（既有逻辑回归）。
  7. 管理后台「风控观察」能看到以上记录，IP 已脱敏。
  8. 若 Phase 7 已执行：经 fingerproxy 出题的记录带 `tls` 字段。
  9. 可选：Playwright 无头 Chromium 出题 `difficulty ≥ 4`。

#### Out of Scope
- 新功能开发；阈值调优（上线后用 `RISK_MODE=log` 观察两周再调）。

#### Inputs
- Phase 1-8 的实现与配置项清单。
- `README.md` 现有章节结构（`## 验证流程`、`### RESTful API 路由`、`### 接口字段说明`、`## 安全机制`、`### 安全与风控`、`## 管理后台`）、`sdk/playground/docs/content.ts`、`AGENTS.md`。

#### Execution Requirements
- README 示例 JSON 与实际响应逐字段核对。
- 端到端每一步记录实际返回的 `difficulty` 与日志关键原因，写入 Change Log 对应版本行。
- 不在 `sdk/playground` 内新增文档内容。

#### Expected Outputs
- 修改：`README.md`、`backend/.env.example`、`AGENTS.md`、（如有）`sdk/playground/docs/content.ts`
- 本台账 Change Log 新增一行端到端核对结果

#### Acceptance Criteria
- [x] README 中出现的每个新环境变量都能在 `backend/config/config.go` 找到对应 `getXxxEnv`。（脚本逐个核对 README 里出现的 13 个 `RISK_*` 变量均在 `config.go` 中有对应 `getEnv`；`.env.example` 已含全部新变量的注释示例）
- [x] 演示站 `/#/docs` 页无"章节被跳过"的控制台告警。（Playwright 打开 `localhost:8010/#/docs`：9 个二级章节全部渲染，控制台无 warning / error；页面文本含「出题前环境评估」「风控观察」与 `/api/v1/captchas/:type`；README 二级标题未改名，`CURATED_GROUPS` 无需改动）
- [x] 端到端步骤 2-7 全部达到预期值，结果已记入 Change Log。（见 v0.11）
- [x] `go test ./...`、`pnpm test && pnpm typecheck && pnpm lint && pnpm build` 通过。（Go 全量 `-count=1` 除 Q8 两项既有失败外全部 ok；SDK 3 个测试文件 23 个用例通过、lint 0 错误 16 条既有告警、typecheck 通过、`pnpm build` 与 `pnpm build:console` 完成）

#### Risks
- 本地浏览器安装了隐私扩展导致步骤 2 被误加难到 4 级：用无扩展的访客窗口复测，并把命中的原因记入 Notes 供阈值调优参考。

#### Dependencies
Phase 3、4、5、6、8；Phase 7 若执行也需完成。

#### Notes
- 验收完成后建议用户以 `RISK_MODE=log` 上线观察，再决定是否调整 30 / 55 升级阈值与各项权重。
- 执行记录（2026-10-26）：
  - README 改动：验证流程图改为 POST 并补"核验请求头 + 环境信号"两步；API 路由表新增 `POST /captchas/:type` 与 `GET /admin/risk/telemetry`；接口字段说明新增 POST 请求体示例；安全机制表新增 5 行并改写「难度分级」「行为/环境风控」；采集级别与隐私按写入即脱敏改写并补 CSP `worker-src blob:` 提示（Q3 采用 A）；环境变量表新增 7 项；管理后台新增「风控观察」小节；License 补第三方声明引用。
  - `AGENTS.md` 目录树补 `backend/risk/`、`deploy/fingerproxy/`、`docs/tasks/`。
  - 端到端核对脚本保存在会话临时目录（`e2e.py`），6 项全部通过，详见 Change Log v0.11；浏览器侧由 Playwright 复核。
  - 读取侧脱敏改为幂等（已脱敏的 IP / UA 摘要原样返回），否则写入侧脱敏后的值会在接口里被二次压成 `*` 与「非浏览器」。
  - 本地环境收尾：本次启动的开发后端（3321）与演示站临时实例（8010）已停止；管理员密码最后一次 `reset-admin-password` 的输出存于会话临时目录 `admin-reset-2.log`（对话中不回显）；Redis 容器 `simple-captcha-redis` 与 fingerproxy 镜像保留。

---

## 5. Current Execution Pointer

Current Phase: All phases completed

Current Objective:
无。全部 9 个阶段已完成，代码已提交到 main。后续若要做降级（真人更易）见 Q7；Q8 的两个背景图测试已于 v0.12 修复。

本台账中的以下内容已被 `docs/tasks/config-center-app-binding.md` 取代，正文作为历史记录保留、不再修改：GET 出题接口已删除（只保留 POST）；指纹集合与遥测由 Redis 改为进程内存与 SQLite；`RISK_*` / `CAPTCHA_*` 策略变量改为管理后台「策略配置」；`reset-admin-password` 改名为 `reset-admin`。

Current Restrictions:
- 不得自动提交代码。

---

## 6. Decision Log

| Date | Decision | Reason | Impact |
|---|---|---|---|
| 2026-10-26 | 新增 `POST /captchas/:type` 出题接口，保留 `GET` 供旧版 SDK，`GET` 视为无环境信号 | 用户选择；GET 查询串不适合承载 env，保留 GET 避免破坏既有接入 | Phase 2、3、9 |
| 2026-10-26 | TLS / HTTP2 指纹只做"读可信代理注入的头 + 列表计分 + fingerproxy 部署示例"，作为可跳过阶段 | 用户选择；Go 标准库拿不到 ClientHello 扩展顺序，进程内解析不可行 | Phase 7 |
| 2026-10-26 | 验收深度为单测 + 手工端到端，不做 Playwright 自动化 | 用户选择 | 各阶段 Acceptance Criteria、Phase 9 |
| 2026-10-26 | 管理后台增加最小只读风控观察页，不做图表与筛选 | 用户选择 | Phase 8 |
| 2026-10-26 | 不引入 BotD / FingerprintJS / ThumbmarkJS 作为依赖，只借鉴检测点并保留 MIT 声明 | AGENTS.md 极简依赖原则；BotD 已进入仅维稳阶段；本项目不需要稳定设备指纹 | Phase 6 |
| 2026-10-26 | 默认值假设：`RISK_ENV_SCORE_CAP=60`、`RISK_BLOCK_MIN_BEHAVIOR=20`、出题环境分 ≥30 加 1 级 / ≥55 加 2 级、`fp_replayed` +30、`non_browser_ua` +20、`RISK_FP_REPLAY_THRESHOLD=5`、`RISK_FP_REPLAY_WINDOW=10m` | 用户未指定，取保守默认并开放为配置；`fp_replayed` 与 `non_browser_ua` 的分值保证单独命中即可加 1 级 | Phase 1、4、5 |
| 2026-10-26 | **本次不做降级**：真人保持 `CAPTCHA_DEFAULT_DIFFICULTY`（3 级），只按异常环境逐级加难；撤销 v0.1 中"干净环境降到 2 级、有历史降到 1 级"的设计与 `RISK_MAX_DOWNGRADE` 配置 | 用户实测 3 级对真人并不复杂；降级要依赖可被重放的正面信号，当前阶段收益小于风险 | 全部阶段的验收预期值；Phase 1 删除降级分支；Q1 关闭 |
| 2026-10-26 | 请求头核验同时作用于 `GET` 与 `POST` 出题及校验提交；既有 httptest 用例改用浏览器请求头辅助发请求 | 旧版 SDK 在真实浏览器中请求头齐全不受影响，而 `curl GET` 是最常见的脚本路径 | Phase 4 |
| 2026-10-26 | 拦截条件改为总分达阈值且（行为分 ≥ 下限 或 命中硬性标记：`webdriver`、`automation_markers`、`untrusted_events`、`worker_*_mismatch`、`getter_patched`、`tls_denylisted`） | 防止 VDI / 隐私浏览器仅凭环境信号被拦截 | Phase 1、6、7 |
| 2026-10-26 | 出题评估与校验评分写入同一 Redis 列表 `captcha:telemetry`，用 `kind` 区分 | 复用既有存储与配置，观察页只读一处 | Phase 2、8 |
| 2026-10-26 | `telemetry='off'` 的 SDK 仍走 `POST` 但不带 `env`，服务端按无环境信号处理 | 协议统一，行为与 `GET` 等价 | Phase 2、3 |
| 2026-10-26 | 时区校验所需 tzdata 通过 `import _ "time/tzdata"` 内嵌 | 运行时镜像 `alpine:3.19` 未安装 tzdata | Phase 4 |
| 2026-10-26 | `tests` 包中 `TestAdmin_ListBackgrounds`、`TestBackgroundStore_GetCoversRequestedSize` 两个既有失败不在本台账内修复，验收时排除 | 在干净 HEAD 工作树复现失败，原因是测试假定 420x420 素材而仓库素材已换成 1536 宽；背景图模块不在本任务范围，且台账禁止为通过测试改断言 | 各阶段 `go test ./...` 验收以"除这两个用例外全部通过"为准；Q8 |
| 2026-10-26 | Phase 1 执行细节：`touch_without_touchscreen` 留在 `Evaluate` 计入环境侧；`IssueInput.State` 为指针；删除 `ClientRiskStore.LevelFor`；fp 阈值暂为常量 | 见 Phase 1 Notes | Phase 1、5 |
| 2026-10-26 | 软件渲染器命中时不叠加 `no_media_devices`；但既有的 `desktop_chrome_without_plugins(+10)` 保持不变，`plugins=0` 的桌面 Chrome 配软件渲染器仍会到 4 级 | 真实 VDI 里的 Chrome / Firefox 都带 5 个 PDF 插件，`plugins=0` 多见于旧版无头 Chrome 或精简内核；成功标准中"VDI 不高于 3 级"以真实 VDI 浏览器为准，无 PDF 插件的发行版 Chromium + 无 GPU 驱动属于已知边界，只加难不拦截 | Phase 6、9 |
| 2026-10-26 | 遥测记录在写入时即脱敏（IP 前两段、UA 摘要），管理接口读取时再脱敏一次以兼容历史记录 | Q5 采用推荐默认 A；README「采集级别与隐私」据此改写 | Phase 8、9；Q5 关闭 |

---

## 7. Open Questions

| ID | Question | Options | Recommended Default | Blocking | Impact | Owner | Status |
|---|---|---|---|---|---|---|---|
| Q1 | ~~真人首题降级上限默认取 2 还是 1？~~ 用户 2026-10-26 决定本次不做降级 | — | 不降级 | NO | 已反映到全部阶段 | User | RESOLVED |
| Q8 | 既有失败的两个背景图测试如何处理？ | A. 单独修复测试夹具：让测试自行生成 420x420 临时图片而不依赖仓库素材；B. 更新断言以匹配当前 1536 宽素材；C. 保持现状 | A | NO | 不影响本台账执行，`go test ./tests/` 在修复前始终有 2 个失败 | User | RESOLVED（按 A：`GetCoversRequestedSize` 自行生成 420x420 PNG；`ListBackgrounds` 改为从文件读取实际宽高作期望值，断言同时覆盖 height；见 v0.12） |
| Q7 | 降级（真人更易）是否作为后续迭代，在 `RISK_MODE=log` 观察一段时间、确认 `consistent=true` 的真人比例后再评估？ | A. 后续迭代单独立台账，本台账只预留 `Consistent` 字段；B. 永不做 | A | NO | 不影响本台账执行 | User | OPEN |
| Q2 | `GET /captchas/:type` 是否标记为 deprecated 并计划在下一个大版本移除？ | A. README 标注"旧版兼容，建议升级 SDK"，不设移除时间；B. 标注 deprecated 并在 3.0 移除；C. 不标注 | A | NO | Phase 9 文案 | User | RESOLVED（按 A：路由表标注"旧版 SDK 兼容"，未设移除时间） |
| Q3 | Worker 一致性检测在 CSP 禁止 `blob:` worker 的站点会静默跳过，是否在 README 前端接入章节提示接入方放行 `worker-src blob:`？ | A. 提示一句，不强制；B. 不提 | A | NO | Phase 9 文案 | User | RESOLVED（按 A：写在「安全机制 / 采集级别与隐私」末尾） |
| Q4 | TLS 指纹 allow / deny 列表的来源与维护方式？ | A. 仓库内空模板 + 运维从遥测日志收集，初期只记录不计分；B. 随代码维护一份常见浏览器 JA4 表 | A | NO | Phase 7 交付物 | User | OPEN |
| Q5 | 遥测日志中的原始 `ip` / `ua` 与 README 隐私表述不一致，如何处理？ | A. 写入时即脱敏（`ip` 前两段、`ua` 家族 + 版本），README 维持原表述；B. 写入保留原值、仅展示层脱敏，README 改为"遥测日志保留 N 条含 IP 的记录用于排障" | A | NO | Phase 8 的脱敏位置、Phase 9 文案 | User | RESOLVED（按 A 实现：`RecordIssue` / `Record` 写入 `MaskIP` 与 `SummarizeUA`，README 已改写） |
| Q6 | Vitest 为 `env.ts` / `anomalies.ts` 测试采用全局 stub 还是新增 `happy-dom` 开发依赖？ | A. 优先 `vi.stubGlobal`，笨重时再加 `happy-dom`（仅 devDependencies）；B. 直接加 `happy-dom` | A | NO | Phase 3、6 测试实现 | Executor | RESOLVED（按 A：全部用 `vi.stubGlobal` + `vi.resetModules`，未新增依赖） |

`Blocking: YES` means an executor must stop instead of proceeding. `Blocking: NO` means an executor should proceed using the Recommended Default and record that choice in the Decision Log.

---

## 8. Change Log

| Version | Date | Change |
|---|---|---|
| v0.1 | 2026-10-26 | Initial ledger created：基于项目现状分析（已有 env / trace / PoW / 规则评分 / 难度分级）与用户确认的四项决策（POST 出题、TLS 读头 + 部署示例、单测 + 手工端到端、最小观察页）拆为 9 个阶段 |
| v0.13 | 2026-10-27 | 执行指针更新：代码已提交；注明被 config-center-app-binding 取代的内容 |
| v0.12 | 2026-10-27 | Q8 RESOLVED：仅改 `backend/tests/admin_test.go` 两个用例，不再依赖仓库素材尺寸（素材现为 1536x1024）。`go test -count=1 ./...` 全部 ok，Q8 两项不再排除 |
| v0.11 | 2026-10-26 | Phase 9 COMPLETED，全部阶段完成：改动 `README.md`、`AGENTS.md`、`services/risk_engine.go`（遥测写入即脱敏）、`services/telemetry_mask.go`（幂等）、`services/telemetry_mask_test.go`、`tests/issue_endpoint_test.go`。端到端核对（最终代码、开发后端 3321）：① Playwright Chromium（无自动化标记）出题 `POST`，`difficulty 3`，`webdriver false`，无 `anomalies`，通过滑块后再出题仍 3（Phase 3 记录）；② `curl GET` 默认 UA → 4，原因 `non_browser_ua(+20) accept_language_missing(+15)`；③ `curl POST` 复制浏览器 env + curl 请求头 → 5，原因含 `ua_mismatch(+30)`；④ 同一 fp 换 6 个 UA 重放 → 前 5 次 3、第 6 次 4，原因 `fp_replayed(+30)`；⑤ VDI 环境（llvmpipe + 无媒体设备 + 完整浏览器请求头）→ 3，原因仅 `software_renderer(+25)`；⑥ 答错两次后再出题 → 4，原因 `client_history(+1)`；⑦ 内嵌控制台「风控观察」100 行，含上述记录，IP 显示 `127.0.*.*`、UA 显示 `Chrome 130 / Windows`，页面文本不含完整 UA；⑧ fingerproxy 实跑见 Phase 7；⑨ CDP 开启 `navigator.webdriver` 后出题 → 5（Phase 6 记录）。指针置为 All phases completed |
| v0.10 | 2026-10-26 | Phase 8 COMPLETED：新增 `services/telemetry_mask.go`、`services/telemetry_mask_test.go`、`tests/admin_risk_test.go`、`sdk/playground/pages/RiskPage.tsx`；改动 `services/client_risk.go`、`models/admin.go`、`controllers/admin_controller.go`、`routes/router.go`、`sdk/playground/api.ts`、`sdk/playground/console/ConsoleView.tsx`、`sdk/playground/styles.css`。指针移到 Phase 9 |
| v0.9 | 2026-10-26 | Phase 7 COMPLETED：新增 `risk/tlsfp.go`、`risk/tlsfp_test.go`、`tests/tls_fingerprint_test.go`、`deploy/fingerproxy/{Dockerfile,docker-compose.yml,README.md,tls-fingerprints.example.json,certs/.gitkeep}`；改动 `risk/request.go`、`risk/score.go`、`config/config.go`、`config/config_risk_test.go`、`.env.example`、`helper/utils.go`、`services/interface.go`、`services/service_manager.go`、`services/risk_engine.go`、`controllers/captcha_controller.go`、根目录 `.gitignore`。指针移到 Phase 8 |
| v0.8 | 2026-10-26 | Phase 6 COMPLETED：新增 `sdk/src/_internal/telemetry/anomalies.ts`、`anomalies.test.ts`、`sdk/THIRD_PARTY_NOTICES.md`；改动 `sdk/src/_internal/telemetry/env.ts`、`index.ts`、`sdk/src/Captcha.tsx`、`sdk/src/Captcha.types.ts`、`backend/risk/types.go`、`risk/score.go`、`risk/score_env_test.go`、`models/request.go`、`services/risk_engine.go`、`tests/models_test.go`。权重决策：软件渲染器命中时不叠加 `no_media_devices`。指针移到 Phase 7 |
| v0.7 | 2026-10-26 | Phase 5 COMPLETED：改动 `config/config.go`、`.env.example`、`services/client_risk.go`、`services/challenge_policy.go`、`services/interface.go`、`controllers/captcha_controller.go`、`services/challenge_policy_test.go`、`config/config_risk_test.go`、`tests/issue_endpoint_test.go`、`tests/browser_headers_test.go`；新增 `services/fingerprint_replay_test.go`、`tests/fingerprint_replay_test.go`。修复接口测试的单 IP 计数串扰。指针移到 Phase 6 |
| v0.6 | 2026-10-26 | Phase 4 COMPLETED：新增 `risk/request.go`、`risk/request_test.go`、`tests/browser_headers_test.go`；改动 `risk/types.go`、`risk/score.go`、`helper/utils.go`、`controllers/captcha_controller.go`、`services/risk_engine.go`、`services/challenge_policy.go`、`tests/security_test.go`、`tests/issue_endpoint_test.go`。行为变更：无浏览器请求头的出题请求从此会被加难，受影响的既有用例已改用浏览器请求头辅助。指针移到 Phase 5 |
| v0.5 | 2026-10-26 | Phase 3 COMPLETED：改动 `sdk/src/Captcha.tsx`、`sdk/src/Captcha.types.ts`、`sdk/src/hooks/use-captcha-request.ts`；新增 `sdk/src/hooks/use-captcha-request.test.ts`、`sdk/src/_internal/telemetry/env.test.ts`。Playwright 实测记录见 Phase 3 验收。指针移到 Phase 4 |
| v0.4 | 2026-10-26 | Phase 2 COMPLETED：改动 `models/request.go`、`middleware/validation.go`、`routes/router.go`、`controllers/captcha_controller.go`、`services/risk_engine.go`、`tests/models_test.go`；新增 `tests/issue_endpoint_test.go`。指针移到 Phase 3 |
| v0.3 | 2026-10-26 | Phase 1 COMPLETED：改动 `risk/types.go`、`risk/score.go`、`services/challenge_policy.go`、`services/client_risk.go`、`services/risk_engine.go`、`controllers/captcha_controller.go`、`config/config.go`、`.env.example`、`tests/security_test.go`；新增 `risk/score_env_test.go`、`services/challenge_policy_test.go`、`services/risk_engine_test.go`、`config/config_risk_test.go`。记录两个既有失败测试（Q8）。指针移到 Phase 2 |
| v0.2 | 2026-10-26 | 按用户决定去掉全部降级设计：真人保持默认 3 级，只做异常环境逐级加难。删除 `RISK_MAX_DOWNGRADE`；`Decide` 只升不降并加不变量测试；Phase 4 请求头核验扩展到 `GET` 并新增 `non_browser_ua` 规则；`fp_replayed` 调为 +30；各阶段验收预期值由 2 / 1 级改为 3 级；Q1 关闭、新增 Q7 |
