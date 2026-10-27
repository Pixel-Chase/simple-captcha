package services

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net/url"
	"simple-captcha/helper"
	"strconv"
	"strings"
)

const (
	// ConsoleAppID 内置系统应用：管理后台自己的验证码，只接受同源请求
	ConsoleAppID = "app_console"
	// maxAppOrigins 每个应用最多配置的允许来源数
	maxAppOrigins = 20
)

// NormalizeOrigin 把来源规范化为 scheme://host[:port]：只接受 http / https；去掉默认端口；统一小写；
// 允许末尾单个斜杠，拒绝路径、查询串、片段、用户信息与通配符（只做精确匹配）
func NormalizeOrigin(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	switch {
	case s == "":
		return "", errors.New("来源不能为空")
	case strings.Contains(s, "*"):
		return "", fmt.Errorf("来源 %q 不支持通配符，请逐个填写完整来源", raw)
	case strings.ContainsAny(s, "#?"):
		return "", fmt.Errorf("来源 %q 不能包含查询串或片段", raw)
	}
	u, err := url.Parse(s)
	if err != nil || u.Opaque != "" {
		return "", fmt.Errorf("来源 %q 格式错误，应为 https://example.com 形式", raw)
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", fmt.Errorf("来源 %q 只能使用 http 或 https", raw)
	}
	if u.User != nil {
		return "", fmt.Errorf("来源 %q 不能包含用户信息", raw)
	}
	if u.Path != "" && u.Path != "/" {
		return "", fmt.Errorf("来源 %q 不能包含路径", raw)
	}
	host := strings.ToLower(u.Hostname())
	if host == "" {
		return "", fmt.Errorf("来源 %q 缺少主机名", raw)
	}
	port := u.Port()
	if port == "" && strings.HasSuffix(u.Host, ":") {
		return "", fmt.Errorf("来源 %q 端口为空", raw)
	}
	if port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return "", fmt.Errorf("来源 %q 端口无效", raw)
		}
		if (scheme == "http" && n == 80) || (scheme == "https" && n == 443) {
			port = ""
		} else {
			port = strconv.Itoa(n)
		}
	}
	if strings.Contains(host, ":") {
		host = "[" + host + "]" // IPv6
	}
	if port != "" {
		host += ":" + port
	}
	return scheme + "://" + host, nil
}

// normalizeOrigins 逐个规范化并去重（保持输入顺序），最多 maxAppOrigins 个
func normalizeOrigins(raw []string) ([]string, *helper.CaptchaError) {
	out := make([]string, 0, len(raw))
	seen := make(map[string]bool, len(raw))
	for _, item := range raw {
		origin, err := NormalizeOrigin(item)
		if err != nil {
			return nil, helper.NewCaptchaError(helper.ErrCodeInvalidParam, err.Error())
		}
		if !seen[origin] {
			seen[origin] = true
			out = append(out, origin)
		}
	}
	if len(out) > maxAppOrigins {
		return nil, helper.NewCaptchaError(helper.ErrCodeInvalidParam, fmt.Sprintf("每个应用最多配置 %d 个允许来源", maxAppOrigins))
	}
	return out, nil
}

// CheckOrigin 判定用户侧请求（出题、提交）能否以 appID 的身份发起：
//   - 应用不存在或已停用 → ErrAppInvalid（1010）
//   - 系统应用只接受同源：origin 必须等于请求自身的 scheme + host（selfOrigin，由中间件按可信代理规则计算）
//   - 普通应用：origin 必须在允许名单内
//   - 缺少 Origin 或不匹配 → ErrOriginNotAllowed（1011）
//
// 拒绝时不透露名单内容
func (s *AppService) CheckOrigin(appID, origin, selfOrigin string) *helper.CaptchaError {
	if !ValidAppID(appID) {
		return helper.ErrAppInvalid
	}
	ctx := context.Background()
	var enabled, system bool
	err := s.db.QueryRowContext(ctx, `SELECT enabled, is_system FROM apps WHERE id = ?`, appID).Scan(&enabled, &system)
	if errors.Is(err, sql.ErrNoRows) {
		return helper.ErrAppInvalid
	}
	if err != nil {
		log.Printf("查询接入应用失败: app=%s, error=%v", appID, err)
		return helper.NewCaptchaError(helper.ErrCodeInternalError, "查询接入应用失败")
	}
	if !enabled {
		return helper.ErrAppInvalid
	}

	normalized, nerr := NormalizeOrigin(origin)
	if nerr != nil {
		return helper.ErrOriginNotAllowed
	}
	if system {
		if self, err := NormalizeOrigin(selfOrigin); err != nil || self != normalized {
			return helper.ErrOriginNotAllowed
		}
		return nil
	}

	var found int
	err = s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM app_origins WHERE app_id = ? AND origin = ?`, appID, normalized).Scan(&found)
	if err != nil {
		log.Printf("查询应用允许来源失败: app=%s, error=%v", appID, err)
		return helper.NewCaptchaError(helper.ErrCodeInternalError, "查询应用允许来源失败")
	}
	if found == 0 {
		return helper.ErrOriginNotAllowed
	}
	return nil
}
