package services

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"
)

const (
	adminCredentialFile = "admin.json"

	initialPasswordLength = 20
	// initialPasswordAlphabet 去掉 0/O、1/l/I 等易混字符，便于从控制台抄录；双击即可整段选中
	initialPasswordAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnpqrstuvwxyz23456789"

	minAdminPasswordLength   = 10 // 新密码最少字符数
	maxAdminPasswordBytes    = 72 // bcrypt 只处理前 72 字节，超出部分直接拒绝
	credentialGenerationSize = 16
)

// adminCredential 管理员凭据，持久化在数据目录的 admin.json
type adminCredential struct {
	PasswordHash string `json:"passwordHash"`
	// MustChangePassword 随机生成的初始密码会出现在控制台输出里（docker logs、日志采集），改密之前只开放改密接口
	MustChangePassword bool `json:"mustChangePassword"`
	// Generation 每次改密或重置都会更换；会话绑定签发时的值，凭据一变旧会话全部失效
	Generation string    `json:"generation"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

// credentialStore 读写 admin.json。
// 读取结果按文件身份缓存：reset-admin-password 子命令在服务运行期间替换文件后，下一次请求即读到新凭据，无需重启
type credentialStore struct {
	path string

	mu     sync.Mutex
	cached *adminCredential
	stat   os.FileInfo
}

func newCredentialStore(dataDir string) *credentialStore {
	return &credentialStore{path: filepath.Join(dataDir, adminCredentialFile)}
}

// load 返回当前凭据；文件缺失时返回错误（服务启动阶段已保证它存在）
func (s *credentialStore) load() (*adminCredential, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	stat, err := os.Stat(s.path)
	if err != nil {
		return nil, err
	}
	if s.cached != nil && os.SameFile(s.stat, stat) &&
		s.stat.ModTime().Equal(stat.ModTime()) && s.stat.Size() == stat.Size() {
		return s.cached, nil
	}

	var cred adminCredential
	if _, err := readJSONFile(s.path, &cred); err != nil {
		return nil, err
	}
	if _, err := bcrypt.Cost([]byte(cred.PasswordHash)); err != nil || cred.Generation == "" {
		return nil, fmt.Errorf("%s 内容无效，可执行 reset-admin-password 重新生成", s.path)
	}
	s.cached, s.stat = &cred, stat
	return &cred, nil
}

func (s *credentialStore) save(cred *adminCredential) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := writeJSONFile(s.path, cred); err != nil {
		return err
	}
	// 写入后丢弃缓存，下一次 load 按新文件重新读取
	s.cached, s.stat = nil, nil
	return nil
}

// exists 凭据文件是否已存在
func (s *credentialStore) exists() (bool, error) {
	_, err := os.Stat(s.path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

// EnsureAdminCredential 数据目录中没有管理员凭据时生成随机初始密码并落盘，返回该明文密码；
// 凭据已存在时返回空串。明文只在这一次返回，之后无法再取回
func EnsureAdminCredential(dataDir string) (string, error) {
	if err := ensureDataDir(dataDir); err != nil {
		return "", err
	}
	store := newCredentialStore(dataDir)
	ok, err := store.exists()
	if err != nil {
		return "", err
	}
	if ok {
		_, err := store.load()
		return "", err
	}
	return resetCredential(store)
}

// ResetAdminPassword 重新生成随机密码（下次登录后必须修改），并作废全部已有会话。
// 供 reset-admin-password 子命令在遗失密码时使用
func ResetAdminPassword(dataDir string) (string, error) {
	if err := ensureDataDir(dataDir); err != nil {
		return "", err
	}
	return resetCredential(newCredentialStore(dataDir))
}

func resetCredential(store *credentialStore) (string, error) {
	password, err := randomPassword()
	if err != nil {
		return "", err
	}
	cred, err := newCredential(password, true)
	if err != nil {
		return "", err
	}
	if err := store.save(cred); err != nil {
		return "", err
	}
	return password, nil
}

func newCredential(password string, mustChange bool) (*adminCredential, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}
	generation := make([]byte, credentialGenerationSize)
	if _, err := rand.Read(generation); err != nil {
		return nil, err
	}
	return &adminCredential{
		PasswordHash:       string(hash),
		MustChangePassword: mustChange,
		Generation:         hex.EncodeToString(generation),
		UpdatedAt:          time.Now(),
	}, nil
}

// randomPassword 从易抄录字母表中均匀采样
func randomPassword() (string, error) {
	size := big.NewInt(int64(len(initialPasswordAlphabet)))
	var b strings.Builder
	b.Grow(initialPasswordLength)
	for i := 0; i < initialPasswordLength; i++ {
		n, err := rand.Int(rand.Reader, size)
		if err != nil {
			return "", err
		}
		b.WriteByte(initialPasswordAlphabet[n.Int64()])
	}
	return b.String(), nil
}

// validateNewPassword 新密码规则：至少 10 个字符、不超过 72 字节（bcrypt 上限）、不能全是空白
func validateNewPassword(password string) string {
	switch {
	case strings.TrimSpace(password) == "":
		return "新密码不能为空"
	case utf8.RuneCountInString(password) < minAdminPasswordLength:
		return fmt.Sprintf("新密码至少需要 %d 个字符", minAdminPasswordLength)
	case len(password) > maxAdminPasswordBytes:
		return fmt.Sprintf("新密码不能超过 %d 字节", maxAdminPasswordBytes)
	default:
		return ""
	}
}

// AdminPasswordNotice 打印到控制台的密码提示框，首次启动与 reset-admin-password 子命令共用
func AdminPasswordNotice(username, password string) string {
	line := strings.Repeat("=", 68)
	return fmt.Sprintf(`
%[1]s
  管理后台初始密码（只显示这一次，请立即妥善保存）

      用户名: %[2]s
      密  码: %[3]s

  首次登录后必须修改密码；遗失可执行 ./simple-captcha reset-admin-password
%[1]s

`, line, username, password)
}
