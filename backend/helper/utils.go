package helper

import (
	"github.com/gin-gonic/gin"
)

// GetRealClientIP 获取客户端 IP。
//
// 代理头（默认 X-Forwarded-For / X-Real-IP，可通过 REMOTE_IP_HEADERS 扩展）
// 只有在直连来源命中 TrustedProxies 配置时才会被采信，这部分逻辑由 Gin 在
// router 初始化时通过 SetTrustedProxies + RemoteIPHeaders 统一处理。
// 旧实现无条件信任这些头，攻击者改头即可绕过限流与客户端绑定。
func GetRealClientIP(c *gin.Context) string {
	return c.ClientIP()
}

// ClientFingerprint 基于真实 IP 与 User-Agent 生成客户端哈希，
// 用于验证码会话绑定（出题与提交必须来自同一客户端）以及风控状态聚合。
func ClientFingerprint(c *gin.Context) string {
	return HashClient(GetRealClientIP(c), c.GetHeader("User-Agent"))
}
