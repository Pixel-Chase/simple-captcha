package risk

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// FingerprintList 可信代理注入的 TLS / HTTP2 指纹的参考数据：
// Browser 把指纹映射到浏览器家族（chrome / firefox / safari / edge），与 UA 声称的家族比对；
// Deny 是已确认属于脚本工具链（curl、Python requests、Go http 等）的指纹。
// 指纹随浏览器版本变化，列表由运维按遥测日志维护，不随代码内置。
type FingerprintList struct {
	Browser map[string]string `json:"browser"`
	Deny    []string          `json:"deny"`

	browser map[string]string
	deny    map[string]bool
}

// ParseFingerprintList 解析 JSON 列表，键值大小写不敏感
func ParseFingerprintList(data []byte) (*FingerprintList, error) {
	var list FingerprintList
	if err := json.Unmarshal(data, &list); err != nil {
		return nil, fmt.Errorf("指纹列表不是合法 JSON: %w", err)
	}
	list.browser = make(map[string]string, len(list.Browser))
	for fp, family := range list.Browser {
		fam := strings.ToLower(strings.TrimSpace(family))
		if fam == "" {
			return nil, fmt.Errorf("指纹 %s 的浏览器家族为空", fp)
		}
		list.browser[strings.ToLower(strings.TrimSpace(fp))] = fam
	}
	list.deny = make(map[string]bool, len(list.Deny))
	for _, fp := range list.Deny {
		if trimmed := strings.ToLower(strings.TrimSpace(fp)); trimmed != "" {
			list.deny[trimmed] = true
		}
	}
	return &list, nil
}

// LoadFingerprintList 从文件加载指纹列表
func LoadFingerprintList(path string) (*FingerprintList, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取指纹列表失败: %w", err)
	}
	return ParseFingerprintList(data)
}

// Denied 指纹是否在拒绝列表中
func (l *FingerprintList) Denied(fp string) bool {
	if l == nil || fp == "" {
		return false
	}
	return l.deny[strings.ToLower(strings.TrimSpace(fp))]
}

// Family 指纹对应的浏览器家族，未知返回空
func (l *FingerprintList) Family(fp string) string {
	if l == nil || fp == "" {
		return ""
	}
	return l.browser[strings.ToLower(strings.TrimSpace(fp))]
}

// uaFamily 从 UA 推断浏览器家族：edge / chrome / firefox / safari，无法判断返回空。
// 其它 Chromium 系浏览器（Brave、Opera、国产浏览器）UA 里都带 Chrome/，归入 chrome
func uaFamily(ua string) string {
	switch {
	case strings.Contains(ua, "Edg/"):
		return "edge"
	case strings.Contains(ua, "Chrome/"), strings.Contains(ua, "CriOS/"):
		return "chrome"
	case strings.Contains(ua, "Firefox/"), strings.Contains(ua, "FxiOS/"):
		return "firefox"
	case strings.Contains(ua, "Safari/"):
		return "safari"
	}
	return ""
}

// evaluateTLS 指纹规则：命中拒绝列表为硬性标记；指纹所属家族与 UA 家族不符为伪装
func evaluateTLS(s *scorer, sig *RequestSignals) {
	list := sig.Fingerprints
	if list == nil || (sig.TLSFingerprint == "" && sig.HTTP2Fingerprint == "") {
		return
	}
	if list.Denied(sig.TLSFingerprint) || list.Denied(sig.HTTP2Fingerprint) {
		s.addHard(40, "tls_denylisted")
	}
	if family := list.Family(sig.TLSFingerprint); family != "" {
		if ua := uaFamily(sig.UserAgent); ua != "" && ua != family {
			s.add(30, "tls_ua_family_mismatch")
		}
	}
}
