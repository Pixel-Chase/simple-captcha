package services

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	"io"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"simple-captcha/cache"
	"simple-captcha/config"
	"simple-captcha/helper"
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
	adminSessionKeyPrefix = "admin:session:"
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

// AdminSession 管理员登录会话
type AdminSession struct {
	Username  string    `json:"username"`
	ExpiresAt time.Time `json:"expiresAt"`
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
	MinWidth       int      `json:"minWidth"`
	MinHeight      int      `json:"minHeight"`
	MaxUploadSize  int64    `json:"maxUploadSize"`
	MaxPixels      int      `json:"maxPixels"`
	AllowedFormats []string `json:"allowedFormats"`
}

// AdminService 管理后台业务：管理员登录会话 + 背景图增删查
type AdminService struct {
	cfg          config.AdminConfig
	passwordHash []byte
	backgrounds  *BackgroundStore
	riskControl  RiskControlService

	fsMu sync.Mutex // 串行化写盘与重载，保证"至少保留一张"的判断原子
}

// NewAdminService 创建管理服务。优先使用 bcrypt 哈希；仅配置明文密码时在启动阶段即时哈希
func NewAdminService(cfg config.AdminConfig, backgrounds *BackgroundStore, riskControl RiskControlService) *AdminService {
	hash := []byte(cfg.PasswordHash)
	if len(hash) > 0 {
		if _, err := bcrypt.Cost(hash); err != nil {
			log.Fatalln("ADMIN_PASSWORD_HASH 不是合法的 bcrypt 哈希:", err)
		}
	} else {
		if cfg.Password == "" {
			log.Fatalln("管理后台已开启但未配置 ADMIN_PASSWORD_HASH 或 ADMIN_PASSWORD")
		}
		generated, err := bcrypt.GenerateFromPassword([]byte(cfg.Password), bcrypt.DefaultCost)
		if err != nil {
			log.Fatalln("管理员密码哈希失败:", err)
		}
		hash = generated
		log.Println("管理后台使用明文 ADMIN_PASSWORD 启动，生产环境建议改用 ADMIN_PASSWORD_HASH")
	}

	return &AdminService{
		cfg:          cfg,
		passwordHash: hash,
		backgrounds:  backgrounds,
		riskControl:  riskControl,
	}
}

// Login 校验登录前置验证码与账号密码，成功后签发不透明会话令牌。
//
// 验证码令牌先于密码比对被消费：无论密码对错，每次尝试都要重新完成一次行为验证，
// 暴力猜解的成本因此随之抬高。
func (s *AdminService) Login(username, password, captchaKey string) (string, *AdminSession, *helper.CaptchaError) {
	if !s.riskControl.ValidateSecondVerifyToken(captchaKey) {
		return "", nil, helper.ErrCaptchaRequired
	}

	// 用户名与密码都做常量时间比较，并且无论用户名是否匹配都执行 bcrypt，避免时序侧信道
	userOK := subtle.ConstantTimeCompare([]byte(username), []byte(s.cfg.Username)) == 1
	passOK := bcrypt.CompareHashAndPassword(s.passwordHash, []byte(password)) == nil
	if !userOK || !passOK {
		return "", nil, helper.ErrLoginFailed
	}

	raw := make([]byte, adminTokenBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", nil, helper.NewCaptchaError(helper.ErrCodeInternalError, "生成会话令牌失败", err.Error())
	}
	token := hex.EncodeToString(raw)

	session := &AdminSession{
		Username:  s.cfg.Username,
		ExpiresAt: time.Now().Add(s.cfg.SessionTTL),
	}
	payload, err := json.Marshal(session)
	if err != nil {
		return "", nil, helper.NewCaptchaError(helper.ErrCodeInternalError, "序列化会话失败", err.Error())
	}
	if err := cache.Set(sessionKey(token), string(payload), s.cfg.SessionTTL); err != nil {
		return "", nil, helper.NewCaptchaError(helper.ErrCodeCacheError, "保存会话失败", err.Error())
	}
	return token, session, nil
}

// Authenticate 校验 Bearer 令牌，返回对应会话
func (s *AdminService) Authenticate(token string) (*AdminSession, bool) {
	if len(token) != adminTokenBytes*2 {
		return nil, false
	}
	if _, err := hex.DecodeString(token); err != nil {
		return nil, false
	}

	raw := cache.Get(sessionKey(token))
	if raw == nil {
		return nil, false
	}

	var session AdminSession
	if err := json.Unmarshal([]byte(*raw), &session); err != nil {
		return nil, false
	}
	if time.Now().After(session.ExpiresAt) {
		return nil, false
	}
	return &session, true
}

// Logout 作废会话令牌
func (s *AdminService) Logout(token string) {
	if token == "" {
		return
	}
	if err := cache.Del(sessionKey(token)); err != nil {
		log.Printf("作废管理会话失败: error=%v", err)
	}
}

// sessionKey Redis 中只保存令牌的哈希，即使缓存内容泄露也无法直接冒用会话
func sessionKey(token string) string {
	sum := sha256.Sum256([]byte(token))
	return adminSessionKeyPrefix + hex.EncodeToString(sum[:])
}

// Constraints 返回上传约束
func (s *AdminService) Constraints() UploadConstraints {
	return UploadConstraints{
		MinWidth:       baseImageWidth,
		MinHeight:      baseImageHeight,
		MaxUploadSize:  s.cfg.MaxUploadSize,
		MaxPixels:      s.cfg.MaxImagePixels,
		AllowedFormats: append([]string{}, allowedUploadFormats...),
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
	if header.Width < baseImageWidth || header.Height < baseImageHeight {
		return nil, helper.NewCaptchaError(helper.ErrCodeInvalidImage,
			fmt.Sprintf("图片尺寸至少需要 %dx%d，当前为 %dx%d", baseImageWidth, baseImageHeight, header.Width, header.Height))
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
