package services

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"io"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"simple-captcha/config"
	"simple-captcha/helper"
	"simple-captcha/storage"
	"sort"
	"strings"
	"sync"
	"time"

	// 注册 JPEG / PNG 解码器，供 image.DecodeConfig / image.Decode 识别上传文件
	_ "image/jpeg"
	_ "image/png"

	"golang.org/x/crypto/bcrypt"
)

const (
	adminTokenBytes       = 32 // 会话令牌随机字节数，十六进制后 64 字符
	backgroundStemMaxLen  = 40 // 落盘文件名中保留的原始文件名可读前缀长度
	backgroundNameRetries = 5  // 随机后缀冲突时的重试次数
)

var (
	// backgroundNamePattern 合法的背景图文件名：字母数字开头，仅含 [A-Za-z0-9._-]，扩展名为 jpg/jpeg/png
	backgroundNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}\.(?i:jpe?g|png)$`)
	// unsafeNameChars 可读前缀中不允许的字符，统一折叠为 -
	unsafeNameChars = regexp.MustCompile(`[^A-Za-z0-9_-]+`)
	// allowedUploadFormats image.DecodeConfig 返回的格式名白名单
	allowedUploadFormats = []string{"jpeg", "png"}
)

// AdminSession 管理员登录会话，存于 admin_sessions 表（只存令牌哈希）
type AdminSession struct {
	Username  string
	ExpiresAt time.Time
	// Generation 签发时的账号代次，改密或重置后不再匹配，旧会话随之失效
	Generation string
}

// BackgroundInfo 背景图元信息
type BackgroundInfo struct {
	Name       string    `json:"name"`
	Size       int64     `json:"size"`
	Width      int       `json:"width"`
	Height     int       `json:"height"`
	Format     string    `json:"format"` // jpeg | png
	ModifiedAt time.Time `json:"modifiedAt"`
}

// UploadConstraints 上传约束，随列表接口下发供前端提示与预检
type UploadConstraints struct {
	MinWidth  int `json:"minWidth"`
	MinHeight int `json:"minHeight"`
	// 建议尺寸（3:2），仅用于前端提示，不做校验
	RecommendedWidth  int      `json:"recommendedWidth"`
	RecommendedHeight int      `json:"recommendedHeight"`
	MaxUploadSize     int64    `json:"maxUploadSize"`
	MaxPixels         int      `json:"maxPixels"`
	AllowedFormats    []string `json:"allowedFormats"`
}

// AdminService 管理后台业务：初始化、管理员登录会话 + 背景图增删查
type AdminService struct {
	cfg         config.AdminConfig
	db          *storage.DB
	backgrounds *BackgroundStore
	riskControl RiskControlService

	fsMu sync.Mutex // 串行化写盘与重载，保证"至少保留一张"的判断原子
}

// NewAdminService 创建管理服务。账号与会话每次都查库、不在进程内缓存，
// 因此 reset-admin 子命令在服务运行期间修改数据库也立即生效
func NewAdminService(cfg config.AdminConfig, db *storage.DB, backgrounds *BackgroundStore, riskControl RiskControlService) *AdminService {
	return &AdminService{
		cfg:         cfg,
		db:          db,
		backgrounds: backgrounds,
		riskControl: riskControl,
	}
}

// Initialized 是否已完成首次初始化（已存在管理员账号）
func (s *AdminService) Initialized() (bool, *helper.CaptchaError) {
	acc, err := loadAdminAccount(context.Background(), s.db)
	if err != nil {
		return false, helper.NewCaptchaError(helper.ErrCodeInternalError, "读取管理员账号失败", err.Error())
	}
	return acc != nil, nil
}

// Setup 首次初始化：校验启动日志中的一次性口令，写入管理员账号并作废口令，随后直接签发会话。
// "确认未初始化 + 写入账号 + 作废口令"在同一事务中完成，并发的两次初始化只有一次成功
func (s *AdminService) Setup(code, username, password string) (string, *AdminSession, *helper.CaptchaError) {
	username, msg := normalizeAdminUsername(username)
	if msg == "" {
		msg = validateNewPassword(password)
	}
	if msg != "" {
		return "", nil, helper.NewCaptchaError(helper.ErrCodeInvalidParam, msg)
	}
	// bcrypt 较慢，放在事务之外，避免占住唯一的数据库连接
	acc, err := newAdminAccount(username, password)
	if err != nil {
		return "", nil, helper.NewCaptchaError(helper.ErrCodeInternalError, "生成密码哈希失败", err.Error())
	}

	ctx := context.Background()
	var token string
	var session *AdminSession
	var cerr *helper.CaptchaError
	err = s.db.Tx(ctx, func(tx *sql.Tx) error {
		existing, err := loadAdminAccount(ctx, tx)
		if err != nil {
			return err
		}
		if existing != nil {
			cerr = helper.ErrAlreadyInitialized
			return cerr
		}
		var storedHash string
		err = tx.QueryRowContext(ctx, `SELECT code_hash FROM admin_setup WHERE id = 1`).Scan(&storedHash)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if !setupCodeMatches(code, storedHash) {
			cerr = helper.ErrSetupCodeInvalid
			return cerr
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO admin_account (id, username, password_hash, generation, updated_at) VALUES (1, ?, ?, ?, ?)`,
			acc.Username, acc.PasswordHash, acc.Generation, acc.UpdatedAt.Unix()); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM admin_setup`); err != nil {
			return err
		}
		token, session, err = s.insertSession(ctx, tx, acc)
		return err
	})
	if cerr != nil {
		return "", nil, cerr
	}
	if err != nil {
		return "", nil, helper.NewCaptchaError(helper.ErrCodeInternalError, "初始化管理员账号失败", err.Error())
	}
	return token, session, nil
}

// Login 校验登录前置验证码与账号密码，成功后签发不透明会话令牌。
//
// 验证码令牌先于密码比对被消费：无论密码对错，每次尝试都要重新完成一次行为验证，
// 暴力猜解的成本因此随之抬高。尚未初始化时直接返回 ErrNotInitialized，不消费验证码
func (s *AdminService) Login(username, password, captchaKey string) (string, *AdminSession, *helper.CaptchaError) {
	ctx := context.Background()
	acc, err := loadAdminAccount(ctx, s.db)
	if err != nil {
		return "", nil, helper.NewCaptchaError(helper.ErrCodeInternalError, "读取管理员账号失败", err.Error())
	}
	if acc == nil {
		return "", nil, helper.ErrNotInitialized
	}
	// 只接受管理后台系统应用的验证结果：接入应用里通过的验证码不能拿来登录
	if !s.riskControl.ConsumeSecondVerifyToken(captchaKey, ConsoleAppID) {
		return "", nil, helper.ErrCaptchaRequired
	}

	// 用户名与密码都做常量时间比较，并且无论用户名是否匹配都执行 bcrypt，避免时序侧信道
	userOK := subtle.ConstantTimeCompare([]byte(username), []byte(acc.Username)) == 1
	passOK := bcrypt.CompareHashAndPassword([]byte(acc.PasswordHash), []byte(password)) == nil
	if !userOK || !passOK {
		return "", nil, helper.ErrLoginFailed
	}

	var token string
	var session *AdminSession
	err = s.db.Tx(ctx, func(tx *sql.Tx) error {
		// 顺带清理已过期的会话，表不会无限增长
		if _, err := tx.ExecContext(ctx, `DELETE FROM admin_sessions WHERE expires_at <= ?`, time.Now().Unix()); err != nil {
			return err
		}
		var err error
		token, session, err = s.insertSession(ctx, tx, acc)
		return err
	})
	if err != nil {
		return "", nil, helper.NewCaptchaError(helper.ErrCodeInternalError, "保存会话失败", err.Error())
	}
	return token, session, nil
}

// ChangePassword 校验当前密码后更换密码。账号代次随之更换，包括当前会话在内的全部旧会话立即失效，
// 调用方拿到新签发的会话继续操作
func (s *AdminService) ChangePassword(currentPassword, newPassword string) (string, *AdminSession, *helper.CaptchaError) {
	if msg := validateNewPassword(newPassword); msg != "" {
		return "", nil, helper.NewCaptchaError(helper.ErrCodeInvalidParam, msg)
	}

	ctx := context.Background()
	acc, err := loadAdminAccount(ctx, s.db)
	if err != nil {
		return "", nil, helper.NewCaptchaError(helper.ErrCodeInternalError, "读取管理员账号失败", err.Error())
	}
	if acc == nil {
		return "", nil, helper.ErrUnauthorized
	}
	if bcrypt.CompareHashAndPassword([]byte(acc.PasswordHash), []byte(currentPassword)) != nil {
		return "", nil, helper.ErrWrongPassword
	}
	if newPassword == currentPassword {
		return "", nil, helper.NewCaptchaError(helper.ErrCodeInvalidParam, "新密码不能与当前密码相同")
	}

	next, err := newAdminAccount(acc.Username, newPassword)
	if err != nil {
		return "", nil, helper.NewCaptchaError(helper.ErrCodeInternalError, "生成密码哈希失败", err.Error())
	}

	var token string
	var session *AdminSession
	var cerr *helper.CaptchaError
	err = s.db.Tx(ctx, func(tx *sql.Tx) error {
		// 以读取时的代次为条件更新：比对密码期间账号已被并发修改（另一次改密或重置）则放弃本次修改
		res, err := tx.ExecContext(ctx,
			`UPDATE admin_account SET password_hash = ?, generation = ?, updated_at = ? WHERE id = 1 AND generation = ?`,
			next.PasswordHash, next.Generation, next.UpdatedAt.Unix(), acc.Generation)
		if err != nil {
			return err
		}
		if n, err := res.RowsAffected(); err != nil || n != 1 {
			cerr = helper.ErrUnauthorized
			return cerr
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM admin_sessions`); err != nil {
			return err
		}
		token, session, err = s.insertSession(ctx, tx, next)
		return err
	})
	if cerr != nil {
		return "", nil, cerr
	}
	if err != nil {
		return "", nil, helper.NewCaptchaError(helper.ErrCodeInternalError, "保存管理员密码失败", err.Error())
	}
	return token, session, nil
}

// insertSession 签发随机会话令牌，库中只保存令牌哈希
func (s *AdminService) insertSession(ctx context.Context, tx *sql.Tx, acc *adminAccount) (string, *AdminSession, error) {
	token, err := randomHex(adminTokenBytes)
	if err != nil {
		return "", nil, err
	}
	now := time.Now()
	session := &AdminSession{
		Username:   acc.Username,
		ExpiresAt:  now.Add(s.cfg.SessionTTL),
		Generation: acc.Generation,
	}
	_, err = tx.ExecContext(ctx,
		`INSERT INTO admin_sessions (token_hash, username, generation, created_at, expires_at) VALUES (?, ?, ?, ?, ?)`,
		sessionHash(token), session.Username, session.Generation, now.Unix(), session.ExpiresAt.Unix())
	if err != nil {
		return "", nil, err
	}
	return token, session, nil
}

// Authenticate 校验 Bearer 令牌，返回对应会话；已过期或账号代次不一致（已改密或重置）的会话视为失效
func (s *AdminService) Authenticate(token string) (*AdminSession, bool) {
	if len(token) != adminTokenBytes*2 {
		return nil, false
	}
	if _, err := hex.DecodeString(token); err != nil {
		return nil, false
	}

	var session AdminSession
	var expires int64
	err := s.db.QueryRowContext(context.Background(),
		`SELECT s.username, s.generation, s.expires_at
		   FROM admin_sessions s
		   JOIN admin_account a ON a.id = 1 AND a.generation = s.generation
		  WHERE s.token_hash = ? AND s.expires_at > ?`,
		sessionHash(token), time.Now().Unix(),
	).Scan(&session.Username, &session.Generation, &expires)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			log.Printf("查询管理会话失败: error=%v", err)
		}
		return nil, false
	}
	session.ExpiresAt = time.Unix(expires, 0)
	return &session, true
}

// Logout 作废会话令牌
func (s *AdminService) Logout(token string) {
	if token == "" {
		return
	}
	if _, err := s.db.ExecContext(context.Background(), `DELETE FROM admin_sessions WHERE token_hash = ?`, sessionHash(token)); err != nil {
		log.Printf("作废管理会话失败: error=%v", err)
	}
}

// sessionHash 库中只保存令牌的哈希，即使数据库文件泄露也无法直接冒用会话
func sessionHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// Constraints 返回上传约束
func (s *AdminService) Constraints() UploadConstraints {
	return UploadConstraints{
		MinWidth:          minBackgroundWidth,
		MinHeight:         minBackgroundHeight,
		RecommendedWidth:  recommendedBackgroundWidth,
		RecommendedHeight: recommendedBackgroundHeight,
		MaxUploadSize:     s.cfg.MaxUploadSize,
		MaxPixels:         s.cfg.MaxImagePixels,
		AllowedFormats:    append([]string{}, allowedUploadFormats...),
	}
}

// ListBackgrounds 列出素材目录中的全部背景图，按修改时间倒序、同时间按文件名升序
func (s *AdminService) ListBackgrounds() ([]BackgroundInfo, *helper.CaptchaError) {
	dir := s.backgrounds.Dir()
	entries, err := listBackgroundEntries(dir)
	if err != nil {
		return nil, helper.NewCaptchaError(helper.ErrCodeInternalError, "读取素材目录失败", err.Error())
	}

	items := make([]BackgroundInfo, 0, len(entries))
	for _, entry := range entries {
		info, err := describeBackground(dir, entry.Name())
		if err != nil {
			log.Printf("跳过无法识别的素材: name=%s, error=%v", entry.Name(), err)
			continue
		}
		items = append(items, info)
	}

	sort.Slice(items, func(i, j int) bool {
		if !items[i].ModifiedAt.Equal(items[j].ModifiedAt) {
			return items[i].ModifiedAt.After(items[j].ModifiedAt)
		}
		return items[i].Name < items[j].Name
	})
	return items, nil
}

// SaveBackground 校验并保存上传的背景图，成功后热重载素材。
//
// 校验顺序：体积 → 解码头部（格式 / 最小尺寸 / 像素总数）→ 完整解码，
// 保证损坏或伪装的文件不会落盘，解压炸弹也在完整解码之前就被拦下。
func (s *AdminService) SaveBackground(originalName string, r io.Reader) (*BackgroundInfo, *helper.CaptchaError) {
	data, err := io.ReadAll(io.LimitReader(r, s.cfg.MaxUploadSize+1))
	if err != nil {
		return nil, helper.NewCaptchaError(helper.ErrCodeInvalidParam, "读取上传文件失败", err.Error())
	}
	if int64(len(data)) > s.cfg.MaxUploadSize {
		return nil, helper.NewCaptchaError(helper.ErrCodeImageTooLarge,
			fmt.Sprintf("图片体积不能超过 %s", formatBytes(s.cfg.MaxUploadSize)))
	}
	if len(data) == 0 {
		return nil, helper.NewCaptchaError(helper.ErrCodeInvalidImage, "上传文件为空")
	}

	header, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || !isAllowedUploadFormat(format) {
		return nil, helper.NewCaptchaError(helper.ErrCodeInvalidImage, "仅支持 JPEG 或 PNG 图片")
	}
	if header.Width < minBackgroundWidth || header.Height < minBackgroundHeight {
		return nil, helper.NewCaptchaError(helper.ErrCodeInvalidImage,
			fmt.Sprintf("图片尺寸至少需要 %dx%d，当前为 %dx%d", minBackgroundWidth, minBackgroundHeight, header.Width, header.Height))
	}
	if header.Width*header.Height > s.cfg.MaxImagePixels {
		return nil, helper.NewCaptchaError(helper.ErrCodeInvalidImage,
			fmt.Sprintf("图片像素总数不能超过 %d", s.cfg.MaxImagePixels))
	}
	if _, _, err := image.Decode(bytes.NewReader(data)); err != nil {
		return nil, helper.NewCaptchaError(helper.ErrCodeInvalidImage, "图片数据损坏，无法解码")
	}

	s.fsMu.Lock()
	defer s.fsMu.Unlock()

	name, err := s.writeBackground(originalName, format, data)
	if err != nil {
		return nil, helper.NewCaptchaError(helper.ErrCodeInternalError, "保存图片失败", err.Error())
	}
	if err := s.backgrounds.Reload(); err != nil {
		return nil, helper.NewCaptchaError(helper.ErrCodeInternalError, "重载背景图失败", err.Error())
	}

	info, err := describeBackground(s.backgrounds.Dir(), name)
	if err != nil {
		return nil, helper.NewCaptchaError(helper.ErrCodeInternalError, "读取图片信息失败", err.Error())
	}
	return &info, nil
}

// DeleteBackground 删除指定背景图并热重载；拒绝删除最后一张
func (s *AdminService) DeleteBackground(name string) *helper.CaptchaError {
	if !validBackgroundName(name) {
		return helper.NewCaptchaError(helper.ErrCodeInvalidParam, "文件名不合法")
	}

	s.fsMu.Lock()
	defer s.fsMu.Unlock()

	dir := s.backgrounds.Dir()
	path := filepath.Join(dir, name)
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return helper.ErrBackgroundNotFound
		}
		return helper.NewCaptchaError(helper.ErrCodeInternalError, "读取图片失败", err.Error())
	}

	entries, err := listBackgroundEntries(dir)
	if err != nil {
		return helper.NewCaptchaError(helper.ErrCodeInternalError, "读取素材目录失败", err.Error())
	}
	if len(entries) <= 1 {
		return helper.ErrLastBackground
	}

	if err := os.Remove(path); err != nil {
		return helper.NewCaptchaError(helper.ErrCodeInternalError, "删除图片失败", err.Error())
	}
	if err := s.backgrounds.Reload(); err != nil {
		return helper.NewCaptchaError(helper.ErrCodeInternalError, "重载背景图失败", err.Error())
	}
	return nil
}

// ResolveBackgroundPath 校验文件名并返回素材的磁盘路径，供预览接口直接回传文件
func (s *AdminService) ResolveBackgroundPath(name string) (string, *helper.CaptchaError) {
	if !validBackgroundName(name) {
		return "", helper.NewCaptchaError(helper.ErrCodeInvalidParam, "文件名不合法")
	}
	path := filepath.Join(s.backgrounds.Dir(), name)
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return "", helper.ErrBackgroundNotFound
	}
	return path, nil
}

// writeBackground 先写临时文件再原子重命名，避免半写入的文件被重载逻辑读到。
// 临时文件以 . 开头且无图片扩展名，目录扫描会自然跳过它
func (s *AdminService) writeBackground(originalName, format string, data []byte) (string, error) {
	dir := s.backgrounds.Dir()

	var name string
	for i := 0; i < backgroundNameRetries; i++ {
		candidate, err := makeBackgroundName(originalName, format)
		if err != nil {
			return "", err
		}
		if _, err := os.Stat(filepath.Join(dir, candidate)); os.IsNotExist(err) {
			name = candidate
			break
		}
	}
	if name == "" {
		return "", fmt.Errorf("无法生成唯一文件名")
	}

	tmp, err := os.CreateTemp(dir, ".upload-*")
	if err != nil {
		return "", err
	}
	tmpPath := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpPath) }

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		cleanup()
		return "", err
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return "", err
	}
	if err := os.Chmod(tmpPath, 0o644); err != nil {
		cleanup()
		return "", err
	}
	if err := os.Rename(tmpPath, filepath.Join(dir, name)); err != nil {
		cleanup()
		return "", err
	}
	return name, nil
}

// makeBackgroundName 由原始文件名生成安全的落盘文件名：可读前缀 + 随机后缀 + 按真实格式确定的扩展名。
// 前缀只保留 ASCII 字母数字与 _-，因此后续按字节截断不会切坏多字节字符
func makeBackgroundName(originalName, format string) (string, error) {
	base := filepath.Base(originalName)
	stem := strings.TrimSuffix(base, filepath.Ext(base))
	stem = strings.Trim(unsafeNameChars.ReplaceAllString(stem, "-"), "-_")
	if len(stem) > backgroundStemMaxLen {
		stem = strings.TrimRight(stem[:backgroundStemMaxLen], "-_")
	}
	if stem == "" {
		stem = "bg"
	}

	suffix := make([]byte, 4)
	if _, err := rand.Read(suffix); err != nil {
		return "", err
	}

	ext := ".png"
	if format == "jpeg" {
		ext = ".jpg"
	}
	return fmt.Sprintf("%s-%s%s", stem, hex.EncodeToString(suffix), ext), nil
}

// validBackgroundName 文件名必须是不含路径成分的裸文件名
func validBackgroundName(name string) bool {
	return backgroundNamePattern.MatchString(name) &&
		!strings.Contains(name, "..") &&
		filepath.Base(name) == name
}

// listBackgroundEntries 返回目录中扩展名受支持的普通文件
func listBackgroundEntries(dir string) ([]os.DirEntry, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	result := make([]os.DirEntry, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !helper.IsSupportedImageFile(entry.Name()) {
			continue
		}
		result = append(result, entry)
	}
	return result, nil
}

// describeBackground 读取单个素材的文件信息与图片头部
func describeBackground(dir, name string) (BackgroundInfo, error) {
	f, err := os.Open(filepath.Join(dir, name))
	if err != nil {
		return BackgroundInfo{}, err
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		return BackgroundInfo{}, err
	}
	header, format, err := image.DecodeConfig(f)
	if err != nil {
		return BackgroundInfo{}, err
	}
	return BackgroundInfo{
		Name:       name,
		Size:       stat.Size(),
		Width:      header.Width,
		Height:     header.Height,
		Format:     format,
		ModifiedAt: stat.ModTime(),
	}, nil
}

func isAllowedUploadFormat(format string) bool {
	for _, allowed := range allowedUploadFormats {
		if format == allowed {
			return true
		}
	}
	return false
}

// formatBytes 以人类可读形式输出体积，用于错误提示
func formatBytes(n int64) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/float64(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.0f KB", float64(n)/float64(1<<10))
	default:
		return fmt.Sprintf("%d B", n)
	}
}
