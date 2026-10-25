package services

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"log"
	"path/filepath"
	"regexp"
	"simple-captcha/cache"
	"simple-captcha/helper"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	appsFile = "apps.json"

	appIDPrefix     = "app_"
	appIDBytes      = 6 // 十六进制后 12 位
	appSecretPrefix = "sk_"
	appSecretBytes  = 24 // 十六进制后 48 位
	appSecretHint   = 10 // 列表中展示的密钥前缀长度（含 sk_）
	appNameMaxLen   = 40
	maxApps         = 100

	appLastUsedKeyPrefix = "admin:app:last_used:"
)

var appIDPattern = regexp.MustCompile(`^app_[0-9a-f]{12}$`)

// appRecord apps.json 中的一条应用记录；密钥只保存 SHA-256 哈希（高熵随机串，无需慢哈希）
type appRecord struct {
	ID              string    `json:"id"`
	Name            string    `json:"name"`
	SecretHash      string    `json:"secretHash"`
	SecretHint      string    `json:"secretHint"`
	Enabled         bool      `json:"enabled"`
	CreatedAt       time.Time `json:"createdAt"`
	UpdatedAt       time.Time `json:"updatedAt"`
	SecretRotatedAt time.Time `json:"secretRotatedAt"`
}

type appsDocument struct {
	Apps []appRecord `json:"apps"`
}

// AppInfo 接入应用对外信息，不含密钥哈希
type AppInfo struct {
	ID              string     `json:"id"`
	Name            string     `json:"name"`
	SecretHint      string     `json:"secretHint"`
	Enabled         bool       `json:"enabled"`
	CreatedAt       time.Time  `json:"createdAt"`
	UpdatedAt       time.Time  `json:"updatedAt"`
	SecretRotatedAt time.Time  `json:"secretRotatedAt"`
	LastUsedAt      *time.Time `json:"lastUsedAt"`
}

// AppService 接入应用管理：每个业务后端一个应用、一个独立密钥，凭密钥调用 S2S 接口。
// 应用列表持久化在数据目录的 apps.json，只由本服务写入，内存中保留一份副本供鉴权查询
type AppService struct {
	path string

	mu   sync.RWMutex
	apps []appRecord
}

// NewAppService 加载数据目录中的应用列表，文件不存在时视为空列表
func NewAppService(dataDir string) (*AppService, error) {
	if err := ensureDataDir(dataDir); err != nil {
		return nil, err
	}
	path := filepath.Join(dataDir, appsFile)
	var doc appsDocument
	if _, err := readJSONFile(path, &doc); err != nil {
		return nil, err
	}
	return &AppService{path: path, apps: doc.Apps}, nil
}

// Count 返回应用数量
func (s *AppService) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.apps)
}

// List 按创建时间返回全部应用
func (s *AppService) List() []AppInfo {
	s.mu.RLock()
	records := append([]appRecord{}, s.apps...)
	s.mu.RUnlock()

	items := make([]AppInfo, 0, len(records))
	for _, record := range records {
		items = append(items, withLastUsed(record.info()))
	}
	return items
}

// Create 新建应用并返回一次性明文密钥
func (s *AppService) Create(name string) (*AppInfo, string, *helper.CaptchaError) {
	name, cerr := normalizeAppName(name)
	if cerr != nil {
		return nil, "", cerr
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.apps) >= maxApps {
		return nil, "", helper.NewCaptchaError(helper.ErrCodeInvalidParam, fmt.Sprintf("应用数量已达上限 %d", maxApps))
	}
	if s.nameTaken(name, "") {
		return nil, "", helper.ErrAppNameConflict
	}

	id, err := randomHex(appIDBytes)
	if err != nil {
		return nil, "", internalError("生成应用 ID 失败", err)
	}
	secret, hash, hint, err := newAppSecret()
	if err != nil {
		return nil, "", internalError("生成应用密钥失败", err)
	}

	now := time.Now()
	record := appRecord{
		ID:              appIDPrefix + id,
		Name:            name,
		SecretHash:      hash,
		SecretHint:      hint,
		Enabled:         true,
		CreatedAt:       now,
		UpdatedAt:       now,
		SecretRotatedAt: now,
	}
	next := append(append([]appRecord{}, s.apps...), record)
	if err := s.persist(next); err != nil {
		return nil, "", err
	}

	info := record.info()
	return &info, secret, nil
}

// Update 修改应用名称与启用状态；停用后该应用的密钥立即无法通过 S2S 鉴权
func (s *AppService) Update(id, name string, enabled bool) (*AppInfo, *helper.CaptchaError) {
	name, cerr := normalizeAppName(name)
	if cerr != nil {
		return nil, cerr
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	index := s.indexOf(id)
	if index < 0 {
		return nil, helper.ErrAppNotFound
	}
	if s.nameTaken(name, id) {
		return nil, helper.ErrAppNameConflict
	}

	next := append([]appRecord{}, s.apps...)
	record := &next[index]
	record.Name = name
	record.Enabled = enabled
	record.UpdatedAt = time.Now()
	if err := s.persist(next); err != nil {
		return nil, err
	}

	info := withLastUsed(record.info())
	return &info, nil
}

// RotateSecret 重置应用密钥并返回新的明文，旧密钥立即失效
func (s *AppService) RotateSecret(id string) (*AppInfo, string, *helper.CaptchaError) {
	s.mu.Lock()
	defer s.mu.Unlock()

	index := s.indexOf(id)
	if index < 0 {
		return nil, "", helper.ErrAppNotFound
	}

	secret, hash, hint, err := newAppSecret()
	if err != nil {
		return nil, "", internalError("生成应用密钥失败", err)
	}

	next := append([]appRecord{}, s.apps...)
	record := &next[index]
	now := time.Now()
	record.SecretHash = hash
	record.SecretHint = hint
	record.SecretRotatedAt = now
	record.UpdatedAt = now
	if err := s.persist(next); err != nil {
		return nil, "", err
	}

	info := withLastUsed(record.info())
	return &info, secret, nil
}

// Delete 删除应用，其密钥立即失效
func (s *AppService) Delete(id string) *helper.CaptchaError {
	s.mu.Lock()
	defer s.mu.Unlock()

	index := s.indexOf(id)
	if index < 0 {
		return helper.ErrAppNotFound
	}

	next := make([]appRecord, 0, len(s.apps)-1)
	next = append(next, s.apps[:index]...)
	next = append(next, s.apps[index+1:]...)
	if err := s.persist(next); err != nil {
		return err
	}

	if err := cache.Del(appLastUsedKeyPrefix + id); err != nil {
		log.Printf("清理应用最近使用时间失败: app=%s, error=%v", id, err)
	}
	return nil
}

// Authenticate 按密钥查找已启用的应用，命中后记录最近使用时间。
// 没有任何应用时一律失败：S2S 接口不存在"未配置则放行"的分支
func (s *AppService) Authenticate(secret string) (*AppInfo, bool) {
	if !strings.HasPrefix(secret, appSecretPrefix) || len(secret) != len(appSecretPrefix)+appSecretBytes*2 {
		return nil, false
	}
	sum := sha256.Sum256([]byte(secret))
	hash := []byte(hex.EncodeToString(sum[:]))

	s.mu.RLock()
	var matched *appRecord
	for i := range s.apps {
		if subtle.ConstantTimeCompare(hash, []byte(s.apps[i].SecretHash)) == 1 {
			matched = &s.apps[i]
		}
	}
	var info AppInfo
	ok := matched != nil && matched.Enabled
	if ok {
		info = matched.info()
	}
	s.mu.RUnlock()

	if !ok {
		return nil, false
	}

	now := time.Now()
	if err := cache.Set(appLastUsedKeyPrefix+info.ID, strconv.FormatInt(now.Unix(), 10), 0); err != nil {
		log.Printf("记录应用最近使用时间失败: app=%s, error=%v", info.ID, err)
	}
	info.LastUsedAt = &now
	return &info, true
}

// ValidAppID 路径参数中的应用 ID 格式校验
func ValidAppID(id string) bool {
	return appIDPattern.MatchString(id)
}

// persist 先落盘再替换内存副本，写盘失败时内存状态保持不变
func (s *AppService) persist(next []appRecord) *helper.CaptchaError {
	if err := writeJSONFile(s.path, appsDocument{Apps: next}); err != nil {
		return internalError("保存应用列表失败", err)
	}
	s.apps = next
	return nil
}

func (s *AppService) indexOf(id string) int {
	for i := range s.apps {
		if s.apps[i].ID == id {
			return i
		}
	}
	return -1
}

// nameTaken 名称不区分大小写去重，exceptID 用于改名时排除自身
func (s *AppService) nameTaken(name, exceptID string) bool {
	for _, record := range s.apps {
		if record.ID != exceptID && strings.EqualFold(record.Name, name) {
			return true
		}
	}
	return false
}

func (r appRecord) info() AppInfo {
	return AppInfo{
		ID:              r.ID,
		Name:            r.Name,
		SecretHint:      r.SecretHint,
		Enabled:         r.Enabled,
		CreatedAt:       r.CreatedAt,
		UpdatedAt:       r.UpdatedAt,
		SecretRotatedAt: r.SecretRotatedAt,
	}
}

// withLastUsed 补上 Redis 中记录的最近使用时间；Redis 数据丢失只影响展示，不影响鉴权
func withLastUsed(info AppInfo) AppInfo {
	if raw := cache.Get(appLastUsedKeyPrefix + info.ID); raw != nil {
		if unix, err := strconv.ParseInt(*raw, 10, 64); err == nil {
			at := time.Unix(unix, 0)
			info.LastUsedAt = &at
		}
	}
	return info
}

// normalizeAppName 去掉首尾空白后要求 1-40 个字符，且不含控制字符
func normalizeAppName(name string) (string, *helper.CaptchaError) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", helper.NewCaptchaError(helper.ErrCodeInvalidParam, "应用名称不能为空")
	}
	if utf8.RuneCountInString(name) > appNameMaxLen {
		return "", helper.NewCaptchaError(helper.ErrCodeInvalidParam, fmt.Sprintf("应用名称不能超过 %d 个字符", appNameMaxLen))
	}
	if strings.IndexFunc(name, unicode.IsControl) >= 0 {
		return "", helper.NewCaptchaError(helper.ErrCodeInvalidParam, "应用名称包含非法字符")
	}
	return name, nil
}

// newAppSecret 生成应用密钥，返回明文、哈希与列表展示用的前缀
func newAppSecret() (secret, hash, hint string, err error) {
	random, err := randomHex(appSecretBytes)
	if err != nil {
		return "", "", "", err
	}
	secret = appSecretPrefix + random
	sum := sha256.Sum256([]byte(secret))
	return secret, hex.EncodeToString(sum[:]), secret[:appSecretHint], nil
}

func randomHex(n int) (string, error) {
	raw := make([]byte, n)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw), nil
}

func internalError(message string, err error) *helper.CaptchaError {
	return helper.NewCaptchaError(helper.ErrCodeInternalError, message, err.Error())
}
