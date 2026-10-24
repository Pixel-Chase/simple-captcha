package helper

import (
	"crypto/md5"
	crand "crypto/rand" // 使用别名避免冲突
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"image"
	"math/rand" // 添加 math/rand 包
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/wenlng/go-captcha-assets/helper"
)

// GetPWD .
func GetPWD() string {
	path, err := os.Getwd()
	if err != nil {
		return ""
	}
	return path
}

// StringToMD5 MD5
func StringToMD5(src string) string {
	m := md5.New()
	m.Write([]byte(src))
	res := hex.EncodeToString(m.Sum(nil))
	return res
}

// GenerateSecureKey 生成 128 位加密随机 key（32 位十六进制）。
// 验证码 key 必须与答案无关：旧实现以答案的 MD5 作为 key，旋转/滑块答案空间很小，
// 攻击者可离线建字典直接反推答案。
func GenerateSecureKey() (string, error) {
	buf := make([]byte, 16)
	if _, err := crand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

// HashClient 将若干客户端特征拼接后做 SHA-256 并截断为 32 位十六进制，
// 用于验证码会话绑定与风控聚合，不落原始 IP / UA。
func HashClient(parts ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(parts, "|")))
	return hex.EncodeToString(sum[:16])
}

// GenerateTimestampedID 生成一个结合时间戳和随机数的唯一ID
func GenerateTimestampedID() (string, error) {
	// 获取当前时间戳（纳秒）
	timestamp := time.Now().UnixNano()
	timestampHex := fmt.Sprintf("%x", timestamp) // 转换为十六进制

	// 生成8字节（64位）的随机数
	randomBytes := make([]byte, 8)
	if _, err := crand.Read(randomBytes); err != nil { // 使用 crand 代替 rand
		return "", err
	}
	randomHex := hex.EncodeToString(randomBytes)

	// 合并时间戳和随机数
	uniqueID := fmt.Sprintf("%s-%s", timestampHex, randomHex)
	return uniqueID, nil
}

// GenerateRandomString 生成指定长度的随机字符串
// length: 要生成的字符串长度
// 返回一个包含字母和数字的随机字符串
func GenerateRandomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	seededRand := rand.New(rand.NewSource(time.Now().UnixNano()))

	b := make([]byte, length)
	for i := range b {
		b[i] = charset[seededRand.Intn(len(charset))]
	}
	return string(b)
}

// supportedImageExts 背景图支持的扩展名（小写）
var supportedImageExts = []string{".jpeg", ".jpg", ".png"}

// IsSupportedImageFile 按扩展名判断文件是否为受支持的背景图
func IsSupportedImageFile(name string) bool {
	return isSupportedImageExt(strings.ToLower(filepath.Ext(name)), supportedImageExts)
}

// ResolveImageDir 解析素材目录：相对路径依次尝试当前工作目录及其上级目录
// （go test 以包目录为工作目录，上级目录即项目根目录）
func ResolveImageDir(configured string) string {
	if configured == "" {
		configured = "images"
	}
	candidates := []string{configured}
	if !filepath.IsAbs(configured) {
		if wd, err := os.Getwd(); err == nil {
			candidates = append(candidates, filepath.Join(filepath.Dir(wd), configured))
		}
	}
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate
		}
	}
	return configured
}

// GetImagesFromDir 从指定目录读取所有支持的图片文件
func GetImagesFromDir(imageDir string) ([]image.Image, error) {
	var images []image.Image

	// 读取目录中的所有文件
	files, err := os.ReadDir(imageDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory %s: %w", imageDir, err)
	}

	// 过滤并处理图片文件
	for _, file := range files {
		if file.IsDir() {
			continue
		}

		// 检查文件扩展名
		fileName := file.Name()
		ext := strings.ToLower(filepath.Ext(fileName))

		if !isSupportedImageExt(ext, supportedImageExts) {
			continue
		}

		// 构建完整文件路径
		filePath := filepath.Join(imageDir, fileName)

		// 读取并解码图片
		img, err := loadImageFile(filePath, ext)
		if err != nil {
			// 记录错误但继续处理其他文件
			fmt.Printf("Warning: failed to load image %s: %v\n", fileName, err)
			continue
		}

		images = append(images, img)
	}

	if len(images) == 0 {
		return nil, fmt.Errorf("no valid images found in directory %s", imageDir)
	}

	return images, nil
}

// isSupportedImageExt 检查文件扩展名是否支持
func isSupportedImageExt(ext string, supportedExts []string) bool {
	for _, supportedExt := range supportedExts {
		if ext == supportedExt {
			return true
		}
	}
	return false
}

// loadImageFile 根据文件扩展名加载图片文件
func loadImageFile(filePath, ext string) (image.Image, error) {
	// 读取文件内容
	asset, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	// 根据文件类型解码图片
	switch ext {
	case ".jpeg", ".jpg":
		return helper.DecodeByteToJpeg(asset)
	case ".png":
		return helper.DecodeByteToPng(asset)
	default:
		return nil, fmt.Errorf("unsupported image format: %s", ext)
	}
}
