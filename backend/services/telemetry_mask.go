package services

import (
	"fmt"
	"net"
	"regexp"
	"strings"
)

// 遥测记录里需要脱敏的字段：原始 IP 与完整 UA 不出现在管理接口响应里，客户端哈希只保留前缀便于辨认
const (
	maskedClientPrefix = 8
)

var (
	uaVersionPattern = regexp.MustCompile(`(Edg|Chrome|Firefox|Version|CriOS|FxiOS)/(\d+)`)
	// uaSummaryPattern SummarizeUA 的输出形态；再次脱敏时原样保留，避免把摘要当成原始 UA 二次压缩
	uaSummaryPattern = regexp.MustCompile(`^(Edge|Chrome|Firefox|Safari|其它|非浏览器)( \d+)?( / [A-Za-z ]+)?$`)
)

// MaskTelemetry 返回脱敏后的遥测记录副本：
// ip 只保留 IPv4 前两段 / IPv6 前三组；ua 压缩为"浏览器家族 主版本 / 操作系统"；client 只保留前 8 位。
// 其余字段（分数、原因、难度、指纹等）原样保留
func MaskTelemetry(entry map[string]interface{}) map[string]interface{} {
	if entry == nil {
		return nil
	}
	out := make(map[string]interface{}, len(entry))
	for k, v := range entry {
		out[k] = v
	}
	if ip, ok := entry["ip"].(string); ok {
		out["ip"] = MaskIP(ip)
	}
	if ua, ok := entry["ua"].(string); ok {
		out["ua"] = SummarizeUA(ua)
	}
	if client, ok := entry["client"].(string); ok && len(client) > maskedClientPrefix {
		out["client"] = client[:maskedClientPrefix]
	}
	return out
}

// MaskIP IPv4 保留前两段（1.2.*.*），IPv6 保留前 48 位（2001:db8:1::*），无法解析的返回占位 *。
// 已脱敏的值（含 *）原样返回，写入侧与读取侧可以各做一次而不会把结果继续压成 *
func MaskIP(ip string) string {
	ip = strings.TrimSpace(ip)
	if ip == "" {
		return ""
	}
	if strings.Contains(ip, "*") {
		return ip
	}
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return "*"
	}
	if v4 := parsed.To4(); v4 != nil {
		return fmt.Sprintf("%d.%d.*.*", v4[0], v4[1])
	}
	v6 := parsed.To16()
	group := func(i int) uint16 { return uint16(v6[i])<<8 | uint16(v6[i+1]) }
	return fmt.Sprintf("%x:%x:%x::*", group(0), group(2), group(4))
}

// SummarizeUA 把完整 UA 压缩成"Chrome 130 / Windows"这样的摘要，空 UA 返回空；已是摘要形态的输入原样返回
func SummarizeUA(ua string) string {
	ua = strings.TrimSpace(ua)
	if ua == "" {
		return ""
	}
	if uaSummaryPattern.MatchString(ua) {
		return ua
	}
	family := "其它"
	version := ""
	switch {
	case strings.Contains(ua, "Edg/"):
		family = "Edge"
	case strings.Contains(ua, "Chrome/"), strings.Contains(ua, "CriOS/"):
		family = "Chrome"
	case strings.Contains(ua, "Firefox/"), strings.Contains(ua, "FxiOS/"):
		family = "Firefox"
	case strings.Contains(ua, "Safari/"):
		family = "Safari"
	case !strings.Contains(ua, "Mozilla/"):
		family = "非浏览器"
	}
	if m := uaVersionPattern.FindAllStringSubmatch(ua, -1); len(m) > 0 {
		// Edge 的 UA 同时含 Chrome/ 与 Edg/，取与家族匹配的那个版本
		for _, match := range m {
			if (family == "Edge" && match[1] == "Edg") ||
				(family == "Chrome" && (match[1] == "Chrome" || match[1] == "CriOS")) ||
				(family == "Firefox" && (match[1] == "Firefox" || match[1] == "FxiOS")) ||
				(family == "Safari" && match[1] == "Version") {
				version = match[2]
				break
			}
		}
	}
	os := uaOS(ua)
	summary := family
	if version != "" {
		summary += " " + version
	}
	if os != "" {
		summary += " / " + os
	}
	return summary
}

func uaOS(ua string) string {
	lower := strings.ToLower(ua)
	switch {
	case strings.Contains(lower, "windows"):
		return "Windows"
	case strings.Contains(lower, "android"):
		return "Android"
	case strings.Contains(lower, "iphone"), strings.Contains(lower, "ipad"):
		return "iOS"
	case strings.Contains(lower, "cros"):
		return "Chrome OS"
	case strings.Contains(lower, "mac os x"), strings.Contains(lower, "macintosh"):
		return "macOS"
	case strings.Contains(lower, "linux"):
		return "Linux"
	}
	return ""
}
