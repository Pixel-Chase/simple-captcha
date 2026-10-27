package services

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"regexp"
	"simple-captcha/helper"
	"simple-captcha/storage"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	appIDPrefix     = "app_"
	appIDBytes      = 6 // 十六进制后 12 位
	appSecretPrefix = "sk_"
	appSecretBytes  = 24 // 十六进制后 48 位
	appSecretHint   = 10 // 列表中展示的密钥前缀长度（含 sk_）
	appNameMaxLen   = 40
	maxApps         = 100

	// lastUsedWriteInterval 同一应用的最近使用时间最多每分钟写一次库，S2S 高频调用不会放大成写入
	lastUsedWriteInterval = time.Minute
)

var appIDPattern = regexp.MustCompile(`^app_[0-9a-f]{12}$`)

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
	// Origins 允许发起出题与提交的来源（已规范化）；系统应用为空，只接受同源
	Origins []string `json:"origins"`
	// IsSystem 内置系统应用（管理后台），不可删除、停用、改名、轮换密钥或修改来源
	IsSystem bool `json:"isSystem"`
}

// AppService 接入应用管理：每个业务后端一个应用、一个独立密钥，凭密钥调用 S2S 接口。
// 应用存于 apps 表，密钥只保存 SHA-256 哈希（高熵随机串，无需慢哈希）
type AppService struct {
	db *storage.DB

	// writeMu 串行化"检查数量与重名 + 写入"，管理操作频率很低
	writeMu sync.Mutex

	lastUsedMu      sync.Mutex
	lastUsedWritten map[string]time.Time // 各应用上次把最近使用时间写库的时刻（进程内存）
}

// NewAppService 创建接入应用服务
func NewAppService(db *storage.DB) *AppService {
	return &AppService{db: db, lastUsedWritten: make(map[string]time.Time)}
}

const appColumns = `id, name, secret_hint, enabled, created_at, updated_at, secret_rotated_at, last_used_at, is_system`

func scanApp(row interface{ Scan(...any) error }) (AppInfo, error) {
	var info AppInfo
	var created, updated, rotated int64
	var lastUsed sql.NullInt64
	if err := row.Scan(&info.ID, &info.Name, &info.SecretHint, &info.Enabled, &created, &updated, &rotated, &lastUsed, &info.IsSystem); err != nil {
		return AppInfo{}, err
	}
	info.Origins = []string{}
	info.CreatedAt = time.Unix(created, 0)
	info.UpdatedAt = time.Unix(updated, 0)
	info.SecretRotatedAt = time.Unix(rotated, 0)
	if lastUsed.Valid {
		at := time.Unix(lastUsed.Int64, 0)
		info.LastUsedAt = &at
	}
	return info, nil
}

// Count 返回接入应用数量（不含内置系统应用）
func (s *AppService) Count() int {
	var n int
	if err := s.db.QueryRowContext(context.Background(), `SELECT COUNT(1) FROM apps WHERE is_system = 0`).Scan(&n); err != nil {
		log.Printf("统计接入应用失败: error=%v", err)
	}
	return n
}

// List 返回全部应用：系统应用在前，其余按创建时间
func (s *AppService) List() ([]AppInfo, *helper.CaptchaError) {
	ctx := context.Background()
	rows, err := s.db.QueryContext(ctx, `SELECT `+appColumns+` FROM apps ORDER BY is_system DESC, created_at, rowid`)
	if err != nil {
		return nil, internalError("读取应用列表失败", err)
	}
	items := make([]AppInfo, 0)
	for rows.Next() {
		info, err := scanApp(rows)
		if err != nil {
			rows.Close()
			return nil, internalError("读取应用列表失败", err)
		}
		items = append(items, info)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, internalError("读取应用列表失败", err)
	}
	rows.Close()

	origins, err := s.allOrigins(ctx)
	if err != nil {
		return nil, internalError("读取应用允许来源失败", err)
	}
	for i := range items {
		if list, ok := origins[items[i].ID]; ok {
			items[i].Origins = list
		}
	}
	return items, nil
}

func (s *AppService) get(ctx context.Context, q queryer, id string) (*AppInfo, error) {
	info, err := scanApp(q.QueryRowContext(ctx, `SELECT `+appColumns+` FROM apps WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	origins, err := s.originsOf(ctx, q, id)
	if err != nil {
		return nil, err
	}
	info.Origins = origins
	return &info, nil
}

// originsOf 某个应用的允许来源，按添加顺序
func (s *AppService) originsOf(ctx context.Context, q queryer, id string) ([]string, error) {
	rows, err := q.QueryContext(ctx, `SELECT origin FROM app_origins WHERE app_id = ? ORDER BY rowid`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []string{}
	for rows.Next() {
		var origin string
		if err := rows.Scan(&origin); err != nil {
			return nil, err
		}
		list = append(list, origin)
	}
	return list, rows.Err()
}

func (s *AppService) allOrigins(ctx context.Context) (map[string][]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT app_id, origin FROM app_origins ORDER BY rowid`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string][]string)
	for rows.Next() {
		var id, origin string
		if err := rows.Scan(&id, &origin); err != nil {
			return nil, err
		}
		out[id] = append(out[id], origin)
	}
	return out, rows.Err()
}

// replaceOrigins 整体替换应用的允许来源
func replaceOrigins(ctx context.Context, tx *sql.Tx, id string, origins []string) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM app_origins WHERE app_id = ?`, id); err != nil {
		return err
	}
	for _, origin := range origins {
		if _, err := tx.ExecContext(ctx, `INSERT INTO app_origins (app_id, origin) VALUES (?, ?)`, id, origin); err != nil {
			return err
		}
	}
	return nil
}

// Create 新建应用并返回一次性明文密钥；origins 为允许发起出题与提交的来源
func (s *AppService) Create(name string, origins []string) (*AppInfo, string, *helper.CaptchaError) {
	name, cerr := normalizeAppName(name)
	if cerr != nil {
		return nil, "", cerr
	}
	origins, cerr = normalizeOrigins(origins)
	if cerr != nil {
		return nil, "", cerr
	}

	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	ctx := context.Background()
	if n := s.Count(); n >= maxApps {
		return nil, "", helper.NewCaptchaError(helper.ErrCodeInvalidParam, fmt.Sprintf("应用数量已达上限 %d", maxApps))
	}
	taken, err := s.nameTaken(ctx, name, "")
	if err != nil {
		return nil, "", internalError("读取应用列表失败", err)
	}
	if taken {
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

	now := time.Now().Unix()
	err = s.db.Tx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO apps (id, name, secret_hash, secret_hint, enabled, created_at, updated_at, secret_rotated_at)
			 VALUES (?, ?, ?, ?, 1, ?, ?, ?)`,
			appIDPrefix+id, name, hash, hint, now, now, now); err != nil {
			return err
		}
		return replaceOrigins(ctx, tx, appIDPrefix+id, origins)
	})
	if err != nil {
		return nil, "", internalError("保存应用失败", err)
	}

	info, err := s.get(ctx, s.db, appIDPrefix+id)
	if err != nil || info == nil {
		return nil, "", internalError("读取应用失败", fmt.Errorf("%v", err))
	}
	return info, secret, nil
}

// Update 修改应用名称、启用状态与允许来源（origins 为 nil 时保持不变）；停用后该应用的密钥与来源立即失效
func (s *AppService) Update(id, name string, enabled bool, origins *[]string) (*AppInfo, *helper.CaptchaError) {
	name, cerr := normalizeAppName(name)
	if cerr != nil {
		return nil, cerr
	}
	var normalized []string
	if origins != nil {
		if normalized, cerr = normalizeOrigins(*origins); cerr != nil {
			return nil, cerr
		}
	}

	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	ctx := context.Background()
	existing, err := s.get(ctx, s.db, id)
	if err != nil {
		return nil, internalError("读取应用失败", err)
	}
	if existing == nil {
		return nil, helper.ErrAppNotFound
	}
	if existing.IsSystem {
		return nil, helper.ErrSystemAppReadonly
	}
	taken, err := s.nameTaken(ctx, name, id)
	if err != nil {
		return nil, internalError("读取应用列表失败", err)
	}
	if taken {
		return nil, helper.ErrAppNameConflict
	}
	err = s.db.Tx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `UPDATE apps SET name = ?, enabled = ?, updated_at = ? WHERE id = ?`,
			name, enabled, time.Now().Unix(), id); err != nil {
			return err
		}
		if origins == nil {
			return nil
		}
		return replaceOrigins(ctx, tx, id, normalized)
	})
	if err != nil {
		return nil, internalError("保存应用失败", err)
	}
	info, err := s.get(ctx, s.db, id)
	if err != nil || info == nil {
		return nil, helper.ErrAppNotFound
	}
	return info, nil
}

// readonly 系统应用不可修改；应用不存在时返回 ErrAppNotFound
func (s *AppService) readonly(ctx context.Context, id string) *helper.CaptchaError {
	var system bool
	err := s.db.QueryRowContext(ctx, `SELECT is_system FROM apps WHERE id = ?`, id).Scan(&system)
	if errors.Is(err, sql.ErrNoRows) {
		return helper.ErrAppNotFound
	}
	if err != nil {
		return internalError("读取应用失败", err)
	}
	if system {
		return helper.ErrSystemAppReadonly
	}
	return nil
}

func (s *AppService) afterUpdate(ctx context.Context, id string, res sql.Result, err error) (*AppInfo, *helper.CaptchaError) {
	if err != nil {
		return nil, internalError("保存应用失败", err)
	}
	if n, err := res.RowsAffected(); err != nil || n == 0 {
		return nil, helper.ErrAppNotFound
	}
	info, err := s.get(ctx, s.db, id)
	if err != nil || info == nil {
		return nil, helper.ErrAppNotFound
	}
	return info, nil
}

// RotateSecret 重置应用密钥并返回新的明文，旧密钥立即失效
func (s *AppService) RotateSecret(id string) (*AppInfo, string, *helper.CaptchaError) {
	secret, hash, hint, err := newAppSecret()
	if err != nil {
		return nil, "", internalError("生成应用密钥失败", err)
	}
	ctx := context.Background()
	if cerr := s.readonly(ctx, id); cerr != nil {
		return nil, "", cerr
	}
	now := time.Now().Unix()
	res, err := s.db.ExecContext(ctx,
		`UPDATE apps SET secret_hash = ?, secret_hint = ?, secret_rotated_at = ?, updated_at = ? WHERE id = ?`,
		hash, hint, now, now, id)
	info, cerr := s.afterUpdate(ctx, id, res, err)
	if cerr != nil {
		return nil, "", cerr
	}
	return info, secret, nil
}

// Delete 删除应用，其密钥立即失效
func (s *AppService) Delete(id string) *helper.CaptchaError {
	if cerr := s.readonly(context.Background(), id); cerr != nil {
		return cerr
	}
	res, err := s.db.ExecContext(context.Background(), `DELETE FROM apps WHERE id = ?`, id)
	if err != nil {
		return internalError("删除应用失败", err)
	}
	if n, err := res.RowsAffected(); err != nil || n == 0 {
		return helper.ErrAppNotFound
	}
	s.lastUsedMu.Lock()
	delete(s.lastUsedWritten, id)
	s.lastUsedMu.Unlock()
	return nil
}

// Authenticate 按密钥查找已启用的应用，命中后记录最近使用时间。
// 没有任何应用时一律失败：S2S 接口不存在"未配置则放行"的分支
func (s *AppService) Authenticate(secret string) (*AppInfo, bool) {
	if !strings.HasPrefix(secret, appSecretPrefix) || len(secret) != len(appSecretPrefix)+appSecretBytes*2 {
		return nil, false
	}
	sum := sha256.Sum256([]byte(secret))
	hash := hex.EncodeToString(sum[:])

	ctx := context.Background()
	// 按哈希查索引：比较的是高熵密钥的 SHA-256，时序差异不会泄露密钥本身
	info, err := scanApp(s.db.QueryRowContext(ctx, `SELECT `+appColumns+` FROM apps WHERE secret_hash = ?`, hash))
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			log.Printf("查询接入应用失败: error=%v", err)
		}
		return nil, false
	}
	// 系统应用没有密钥，不能用于 S2S
	if !info.Enabled || info.IsSystem {
		return nil, false
	}

	now := time.Now()
	s.touchLastUsed(ctx, info.ID, now)
	info.LastUsedAt = &now
	return &info, true
}

// touchLastUsed 记录最近使用时间，按应用节流写库；写入失败只影响展示，不影响鉴权
func (s *AppService) touchLastUsed(ctx context.Context, id string, now time.Time) {
	s.lastUsedMu.Lock()
	last, ok := s.lastUsedWritten[id]
	due := !ok || now.Sub(last) >= lastUsedWriteInterval
	if due {
		s.lastUsedWritten[id] = now
	}
	s.lastUsedMu.Unlock()
	if !due {
		return
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE apps SET last_used_at = ? WHERE id = ?`, now.Unix(), id); err != nil {
		log.Printf("记录应用最近使用时间失败: app=%s, error=%v", id, err)
	}
}

// ValidAppID 应用 ID 格式校验：app_ + 12 位十六进制，或内置系统应用 app_console
func ValidAppID(id string) bool {
	return id == ConsoleAppID || appIDPattern.MatchString(id)
}

// nameTaken 名称不区分大小写去重（Unicode 大小写折叠），exceptID 用于改名时排除自身
func (s *AppService) nameTaken(ctx context.Context, name, exceptID string) (bool, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name FROM apps`)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var id, existing string
		if err := rows.Scan(&id, &existing); err != nil {
			return false, err
		}
		if id != exceptID && strings.EqualFold(existing, name) {
			return true, nil
		}
	}
	return false, rows.Err()
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
