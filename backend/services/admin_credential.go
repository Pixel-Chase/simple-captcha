package services

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"simple-captcha/storage"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"
)

const (
	minAdminPasswordLength   = 10 // 密码最少字符数
	maxAdminPasswordBytes    = 72 // bcrypt 只处理前 72 字节，超出部分直接拒绝
	maxAdminUsernameLength   = 64
	credentialGenerationSize = 16

	setupCodeGroups    = 3
	setupCodeGroupSize = 4
	// setupCodeAlphabet 大写字母与数字去掉 0/O、1/I 等易混字符，便于从日志抄录；32 个字符，12 位约 60 比特
	setupCodeAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
)

// adminAccount 唯一的管理员账号，存于 admin_account 表（id = 1）
type adminAccount struct {
	Username     string
	PasswordHash string
	// Generation 每次改密都会更换；会话记录签发时的值，账号一变旧会话全部失效
	Generation string
	UpdatedAt  time.Time
}

// loadAdminAccount 读取管理员账号；尚未初始化时返回 (nil, nil)
func loadAdminAccount(ctx context.Context, q queryer) (*adminAccount, error) {
	var acc adminAccount
	var updated int64
	err := q.QueryRowContext(ctx,
		`SELECT username, password_hash, generation, updated_at FROM admin_account WHERE id = 1`,
	).Scan(&acc.Username, &acc.PasswordHash, &acc.Generation, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	acc.UpdatedAt = time.Unix(updated, 0)
	return &acc, nil
}

// queryer 同时适配 *sql.DB 与 *sql.Tx
type queryer interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// newAdminAccount 由明文密码生成账号记录（bcrypt 哈希 + 新的代次）
func newAdminAccount(username, password string) (*adminAccount, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}
	generation, err := randomHex(credentialGenerationSize)
	if err != nil {
		return nil, err
	}
	return &adminAccount{
		Username:     username,
		PasswordHash: string(hash),
		Generation:   generation,
		UpdatedAt:    time.Now(),
	}, nil
}

// PrepareSetup 尚未初始化时生成新的初始化口令（覆盖旧口令）并返回明文；已初始化时返回空串。
// 服务每次启动调用一次，因此有效口令总在最近一次启动日志里；库中只存哈希
func PrepareSetup(ctx context.Context, db *storage.DB) (string, error) {
	var code string
	err := db.Tx(ctx, func(tx *sql.Tx) error {
		acc, err := loadAdminAccount(ctx, tx)
		if err != nil || acc != nil {
			return err
		}
		code, err = writeSetupCode(ctx, tx)
		return err
	})
	if err != nil {
		return "", err
	}
	return code, nil
}

// ResetAdmin 删除管理员账号与全部会话并生成新的初始化口令，服务回到未初始化状态。
// 供 reset-admin 子命令在遗失密码时使用；服务运行中执行也立即生效（账号与会话每次都查库）。
// 接入应用、策略与背景图不受影响
func ResetAdmin(ctx context.Context, db *storage.DB) (string, error) {
	var code string
	err := db.Tx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `DELETE FROM admin_account`); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM admin_sessions`); err != nil {
			return err
		}
		var err error
		code, err = writeSetupCode(ctx, tx)
		return err
	})
	if err != nil {
		return "", err
	}
	return code, nil
}

func writeSetupCode(ctx context.Context, tx *sql.Tx) (string, error) {
	code, err := newSetupCode()
	if err != nil {
		return "", err
	}
	_, err = tx.ExecContext(ctx,
		`INSERT INTO admin_setup (id, code_hash, created_at) VALUES (1, ?, ?)
		 ON CONFLICT (id) DO UPDATE SET code_hash = excluded.code_hash, created_at = excluded.created_at`,
		hashSetupCode(code), time.Now().Unix())
	if err != nil {
		return "", err
	}
	return code, nil
}

// newSetupCode 从易抄录字母表中均匀采样，按 XXXX-XXXX-XXXX 分组
func newSetupCode() (string, error) {
	size := big.NewInt(int64(len(setupCodeAlphabet)))
	var b strings.Builder
	for g := 0; g < setupCodeGroups; g++ {
		if g > 0 {
			b.WriteByte('-')
		}
		for i := 0; i < setupCodeGroupSize; i++ {
			n, err := rand.Int(rand.Reader, size)
			if err != nil {
				return "", err
			}
			b.WriteByte(setupCodeAlphabet[n.Int64()])
		}
	}
	return b.String(), nil
}

// normalizeSetupCode 忽略大小写、空白与连字符，用户从日志复制时多带的空格不影响校验
func normalizeSetupCode(code string) string {
	return strings.Map(func(r rune) rune {
		if r == '-' || unicode.IsSpace(r) {
			return -1
		}
		return unicode.ToUpper(r)
	}, code)
}

// hashSetupCode 口令为高熵随机串，SHA-256 即可，无需慢哈希
func hashSetupCode(code string) string {
	sum := sha256.Sum256([]byte(normalizeSetupCode(code)))
	return hex.EncodeToString(sum[:])
}

// setupCodeMatches 常量时间比对口令哈希
func setupCodeMatches(code, storedHash string) bool {
	if storedHash == "" || normalizeSetupCode(code) == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(hashSetupCode(code)), []byte(storedHash)) == 1
}

// normalizeAdminUsername 去掉首尾空白后要求 1-64 个字符，且不含控制字符
func normalizeAdminUsername(username string) (string, string) {
	username = strings.TrimSpace(username)
	switch {
	case username == "":
		return "", "用户名不能为空"
	case utf8.RuneCountInString(username) > maxAdminUsernameLength:
		return "", fmt.Sprintf("用户名不能超过 %d 个字符", maxAdminUsernameLength)
	case strings.IndexFunc(username, unicode.IsControl) >= 0:
		return "", "用户名包含非法字符"
	default:
		return username, ""
	}
}

// validateNewPassword 密码规则：至少 10 个字符、不超过 72 字节（bcrypt 上限）、不能全是空白
func validateNewPassword(password string) string {
	switch {
	case strings.TrimSpace(password) == "":
		return "密码不能为空"
	case utf8.RuneCountInString(password) < minAdminPasswordLength:
		return fmt.Sprintf("密码至少需要 %d 个字符", minAdminPasswordLength)
	case len(password) > maxAdminPasswordBytes:
		return fmt.Sprintf("密码不能超过 %d 字节", maxAdminPasswordBytes)
	default:
		return ""
	}
}

// SetupNotice 打印到控制台的初始化提示框，服务启动与 reset-admin 子命令共用
func SetupNotice(code string) string {
	line := strings.Repeat("=", 68)
	return fmt.Sprintf(`
%[1]s
  管理后台尚未初始化

      打开控制台:   /console/（与验证码服务同一地址）
      初始化口令:   %[2]s

  在初始化页面填写口令并设置管理员用户名与密码；初始化完成后口令自动作废。
  每次启动都会换新口令，请以最近一次启动日志为准。
%[1]s

`, line, code)
}
