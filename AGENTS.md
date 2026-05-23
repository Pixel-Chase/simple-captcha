## 1. 项目概述

**simple-captcha** 是一个包含前后端的 Monorepo（单体仓库）项目，提供完整的行为验证码解决方案。
项目分为 `backend`（Go 后端服务）和 `sdk`（React 前端 SDK）两个核心子工程，以实现统一维护、全链路联调以及更便捷的后续功能迭代。

## 2. 目录规范与架构

项目根目录采用典型的 Monorepo 结构，职责划分如下：

```text
simple-captcha/
├── backend/                     # Go 验证码后端服务
│   ├── controllers/             # 路由控制层（接收请求、参数校验）
│   ├── services/                # 业务逻辑层（调用 Go-Captcha 引擎生成图形验证数据）
│   ├── models/                  # 数据模型与响应结构定义
│   ├── routes/                  # API 路由注册
│   ├── middleware/              # 通用中间件（请求拦截、CORS等）
│   ├── cache/                   # 缓存配置 (Redis)
│   ├── config/                  # 应用配置项
│   ├── helper/                  # 全局工具函数
│   └── main.go                  # 后端服务入口
│
└── sdk/                         # React 验证码前端组件库 / SDK
    ├── src/
    │   ├── index.tsx            # SDK 唯一公共导出入口
    │   ├── Captcha.tsx          # 暴露给用户的核心组件
    │   ├── _internal/           # 内部核心渲染引擎（对外严格保密，处理点选、滑块等视图）
    │   ├── hooks/               # 供用户引用的公共 Hooks (如用以触发请求和弹出框)
    │   └── utils/               # 内部工具函数
    ├── docs/                    # 基于 dumi 的文档与演示站点目录
    └── package.json
```

## 3. `backend` (后端) 规范

### 3.1 技术栈
*   **Web 框架**: Gin (`github.com/gin-gonic/gin`)
*   **核心图形引擎**: Go-Captcha v2 (`github.com/wenlng/go-captcha/v2`)
*   **缓存与持久化**: Redis (`github.com/go-redis/redis/v8`)
*   **开发语言**: Go 1.23+

### 3.2 架构与编码准则
1. **分层架构**: 遵循 `Controller -> Service` 的线性调用链路。Controller 仅负责参数绑定、校验与组装响应，复杂的验证码生产、切图比对等硬核业务逻辑需统一定义在 Service 层。
2. **错误处理机制**: 采用标准化且结构一致的 JSON 响应格式（例如包含 code/msg/data），确保前端 SDK `use-captcha-request.ts` 能够无缝兼容错误捕获。
3. **接口高内聚**: 新增或调整特定验证码类型相关的 API（例如点选、滑块）时，保持 `routes` 的职责边界清晰，验证与生成接口分离。

## 4. `sdk` (前端) 规范

### 4.1 技术栈
*   **核心框架**: React 19, 原生 DOM API (`react-dom`)
*   **开发语言**: TypeScript
*   **样式方案**: 原生 CSS-in-JS / 内联样式配置 (`React.CSSProperties`)
*   **构建工具**: Umi / Father

### 4.2 架构与编码准则 (Agent 必读)
1. **隔离性原则**: 位于 `sdk/src/_internal/` 下的组件、Hook 或类型，**绝对禁止**被导出到 `sdk/src/index.tsx` 或对外暴露。它们仅为 `Captcha.tsx` 提供底层支持引擎。
2. **就近原则**: 每个验证模式（如 `click`, `slide`）在其自身的目录下闭环，包含自己的组件、类型定义（`*.types.ts`）和独占比对逻辑 Hook。
3. **针对 React 18/19 的现代开发规范**:
    *   ❌ 移除旧版 `import * as React from "react"`。
    *   ✅ 使用解构导入 `import { useState, useCallback, memo } from 'react'`，类型一律使用 `import type`。
    *   ❌ 绝不使用传统的 `e.cancelBubble = true` 。
    *   ✅ **必须使用**现代 DOM API 处理事件防冒泡与阻止默认行为：`e.stopPropagation()` 和 `e.preventDefault()`。
4. **极简依赖**: 构建目标 SDK 时，`sdk/package.json` 中的 `pnpm.onlyBuiltDependencies` 等勿随意更动，保持产物轻量纯净，规避引入臃肿的第三方 UI 库。

## 5. 开发流程与联调任务池

当您（Agent）接到一项跨端新需求时，请按如下基本思路拆解执行：
1. **定位受影响象限**: 确认改动是纯请求逻辑扩容（`backend/`），还是渲染层更新（`sdk/`），亦或是需要新增通信协议（双端联动修改）。
2. **新增功能场景联动**: 如果要增加一种全新验证交互模式：
   - 首先在 `backend/services` 里新增对应图形组装与校验规则。
   - 其次同步在 `sdk/src/_internal/` 内创建对应的视图控制台模块和内部 Hook。
3. **接口约定对齐**: 若后端调整了参数入参或返回值，必须当即去 `sdk/src/hooks/use-captcha-request.ts` 和相关 `types` 中进行同步修复。

> **日常调试指北：**
> * 启动后端服务 (位于 `backend/`)： `go run main.go` 或执行 `sh run-dev.sh`
> * 启动前端文档及 SDK 热更新器 (位于 `sdk/`)： `pnpm start`
