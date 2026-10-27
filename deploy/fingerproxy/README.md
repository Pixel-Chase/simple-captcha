# TLS / HTTP2 指纹代理（fingerproxy）

把 [fingerproxy](https://github.com/wi1dcard/fingerproxy)（Apache-2.0）放在验证码服务前面终结 TLS，它会把每个请求的 JA3、JA4 与 HTTP/2（Akamai）指纹写进请求头再转发给后端：

```
X-JA3-Fingerprint: 579ccef312d18482fc42e2b822ca2430
X-JA4-Fingerprint: t13d1516h2_8daaf6152771_e5627efa2ab1
X-HTTP2-Fingerprint: 1:65536;2:0;4:6291456;6:262144|15663105|0|m,a,s,p
```

这些值来自客户端的网络栈，脚本改 JSON、改 UA 都改不了：Python requests、Go http、curl 的指纹与任何浏览器都不同；无头 Chrome 的指纹则与真 Chrome 相同，抓不到它。所以这一层的价值是识别"根本不是浏览器"的客户端，与请求头核验互补。

## 什么时候不需要它

- 服务部署在 Cloudflare、Akamai 等带 Bot 管理的 WAF 之后：TLS 已在 WAF 终结，直接用它们的 bot score，通过自定义请求头传给后端即可（后端只认 `RISK_TLS_FINGERPRINT_HEADER` 指定的头名，叫什么都行）。
- 没有能力在自己的入口终结 TLS：fingerproxy 必须是客户端 TLS 连接的另一端，放在 Nginx 之后拿到的是 Nginx 的指纹。

## 本地联调

1. 生成自签证书（正式环境换成真实证书，文件名保持 `tls.crt` / `tls.key`）：

   ```bash
   cd deploy/fingerproxy
   openssl req -x509 -newkey rsa:2048 -nodes -days 365 \
     -keyout certs/tls.key -out certs/tls.crt -subj "/CN=captcha.local"
   ```

2. 后端开启指纹头读取后启动（`backend/` 目录）：

   ```bash
   RISK_TLS_FINGERPRINT_HEADER=X-JA4-Fingerprint \
   RISK_HTTP2_FINGERPRINT_HEADER=X-HTTP2-Fingerprint ./run-dev.sh
   ```

3. 只起代理，转发到宿主机上的后端。容器默认以 uid 1001 运行，私钥是 0600 时它读不到：本地联调用 `FINGERPROXY_USER` 指定当前用户，正式环境把证书 `chown 1001` 即可：

   ```bash
   FINGERPROXY_USER=$(id -u) FORWARD_URL=http://host.docker.internal:3321 docker compose up -d --build fingerproxy
   curl -k https://127.0.0.1:8443/api/v1/captchas/slide-text -o /dev/null -w "%{http_code}\n"
   ```

4. 后端日志里这次出题的 `出题评估:` 记录会带 `tls` / `h2` 字段（管理后台「风控观察」同样可见）。curl 没有浏览器请求头，还会命中 `non_browser_ua`、`accept_language_missing` 被加难。

代理 + 后端 + Redis 一起起：`docker compose --profile full up -d --build`，然后访问 `https://127.0.0.1:8443/console/`。

## 可信代理

后端只在**直连来源**命中 `TRUSTED_PROXIES` 时才读取指纹头，否则客户端自己加一个 `X-JA4-Fingerprint` 就能伪造。compose 默认网段落在 `172.16.0.0/12`，已被 `TRUSTED_PROXIES` 的默认值（`10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16`）覆盖；代理与后端跨主机部署时把代理的出口地址加进去。

## 指纹列表

不配置 `RISK_TLS_FINGERPRINT_LIST` 时指纹只记录到遥测，不参与评分。观察一段时间后，把 `tls-fingerprints.example.json` 复制为正式文件、填入内容并通过环境变量指向它：

- `browser`：指纹 → 浏览器家族（`chrome` / `firefox` / `safari` / `edge`）。指纹声称的家族与 UA 声称的不符时 `tls_ua_family_mismatch(+30)`。
- `deny`：已确认属于脚本工具链的指纹，命中即 `tls_denylisted(+40)`，属于硬性标记。

未出现在列表里的指纹保持中性。浏览器每次大版本更新指纹都可能变化，列表要跟着遥测日志维护；文件在启动时加载一次并做格式校验，不合法会拒绝启动。
