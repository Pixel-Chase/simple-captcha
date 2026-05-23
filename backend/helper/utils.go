package helper

import (
	"net"
	"strings"

	"github.com/gin-gonic/gin"
)

// GetRealClientIP 获取真实的客户端IP地址
// 支持从反向代理（如Spring Cloud Gateway）中获取真实IP
func GetRealClientIP(c *gin.Context) string {
	// 按优先级检查各种代理头
	headers := []string{
		"X-Forwarded-For",
		"X-Real-IP",
		"X-Client-IP",
		"CF-Connecting-IP", // Cloudflare
		"True-Client-IP",   // Akamai
	}

	for _, header := range headers {
		ip := c.GetHeader(header)
		if ip != "" {
			// X-Forwarded-For 可能包含多个IP，取第一个
			if header == "X-Forwarded-For" {
				ips := strings.Split(ip, ",")
				if len(ips) > 0 {
					ip = strings.TrimSpace(ips[0])
				}
			}

			// 验证IP格式是否有效
			if isValidIP(ip) {
				return ip
			}
		}
	}

	// 如果没有找到代理头，使用Gin的默认方法
	return c.ClientIP()
}

// isValidIP 验证IP地址格式是否有效
func isValidIP(ip string) bool {
	// 排除内网IP和无效IP
	if ip == "" || ip == "unknown" || ip == "127.0.0.1" || ip == "::1" {
		return false
	}

	// 解析IP地址
	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return false
	}

	// 排除私有IP地址（可选，根据需求决定）
	// 如果你的应用部署在内网环境，可能需要注释掉这部分
	if parsedIP.IsPrivate() || parsedIP.IsLoopback() {
		return false
	}

	return true
}
