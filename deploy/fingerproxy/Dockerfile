# fingerproxy（Apache-2.0）：终结 TLS，计算 JA3 / JA4 / HTTP2 指纹并以请求头转发给后端。
# 上游不发布容器镜像，这里从 Go 模块代理按固定版本构建；升级时同时更新 FINGERPROXY_VERSION 与 README 中的头名说明。
FROM golang:1.27.3-alpine AS build
ARG FINGERPROXY_VERSION=v1.1.0
ENV CGO_ENABLED=0 GOBIN=/out
RUN go install "github.com/wi1dcard/fingerproxy/cmd@${FINGERPROXY_VERSION}" && mv /out/cmd /out/fingerproxy

FROM alpine:3.19
RUN apk add --no-cache ca-certificates && adduser -D -u 1001 fingerproxy
COPY --from=build /out/fingerproxy /usr/local/bin/fingerproxy
USER fingerproxy
# 参数全部支持同名环境变量：LISTEN_ADDR / FORWARD_URL / CERT_FILENAME / CERTKEY_FILENAME / PRESERVE_HOST / VERBOSE
ENTRYPOINT ["fingerproxy"]
