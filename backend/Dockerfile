# syntax=docker/dockerfile:1
# 构建上下文为仓库根目录（同时需要 backend 与 sdk）：docker build -f backend/Dockerfile .

# ----------------------------
# 第一阶段：构建管理后台前端（sdk/playground/console），产物内嵌进二进制
# ----------------------------
FROM --platform=$BUILDPLATFORM node:24-slim AS console

WORKDIR /src/sdk
RUN npm install -g pnpm@11.21.0

# 先只复制依赖清单，源码变化时依赖层仍可复用缓存
COPY sdk/package.json sdk/pnpm-lock.yaml sdk/pnpm-workspace.yaml ./
RUN pnpm install --frozen-lockfile

COPY sdk/ ./
# 产物输出到 /src/backend/console/dist（见 sdk/vite.console.config.ts）
RUN pnpm build:console

# ----------------------------
# 第二阶段：编译 Go 服务
# ----------------------------
FROM --platform=$BUILDPLATFORM golang:1.27.3-alpine AS builder

ARG TARGETOS=linux
ARG TARGETARCH=amd64

# 安装必要的工具
RUN apk add --no-cache git ca-certificates

# 设置工作目录
WORKDIR /app

# 复制 go.mod 和 go.sum 文件并下载依赖
COPY backend/go.mod backend/go.sum ./
RUN go mod download && go mod verify

# 复制源代码与管理后台前端产物（go:embed 在编译期读取 console/dist）
COPY backend/ ./
COPY --from=console /src/backend/console/dist ./console/dist

# 编译应用程序（优化编译参数）
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build \
    -ldflags='-w -s' \
    -trimpath \
    -o simple-captcha .

# ----------------------------
# 第三阶段：运行阶段
# ----------------------------
FROM alpine:3.19

# 安装运行时依赖并创建用户（合并RUN减少层数）
RUN apk --no-cache add ca-certificates wget && \
    addgroup -g 1001 -S appgroup && \
    adduser -u 1001 -S appuser -G appgroup

# 设置工作目录
WORKDIR /app

# 从构建阶段复制必要文件
COPY --from=builder /app/simple-captcha .
COPY --from=builder /app/images ./images

# 设置文件权限；/app/data 存放 captcha.db（管理员账号、接入应用、策略配置、风控观察），预先创建以便具名卷继承属主
RUN chmod +x simple-captcha && \
    mkdir -p /app/data && chmod 700 /app/data && \
    chown -R appuser:appgroup /app

# 切换到非root用户
USER appuser

# 设置默认环境变量
ENV PORT=3321 \
    GIN_MODE=release \
    HOST=0.0.0.0

# 健康检查
HEALTHCHECK --interval=30s --timeout=10s --start-period=40s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:3321/health || exit 1

# 暴露端口
EXPOSE 3321

# 启动命令
CMD ["./simple-captcha"]
