package services

import (
	"log"
	"os"
	"path/filepath"
)

// legacyDataFiles 改用 SQLite（data/captcha.db）之前的 JSON 持久化文件，已不再读取
var legacyDataFiles = []string{"admin.json", "apps.json"}

// warnLegacyDataFiles 数据目录中残留旧版 JSON 文件时提示一次：既不读取也不删除，由运维自行清理
func warnLegacyDataFiles(dataDir string) {
	for _, name := range legacyDataFiles {
		path := filepath.Join(dataDir, name)
		if _, err := os.Stat(path); err == nil {
			log.Printf("警告: %s 已不再使用（管理员账号与接入应用改存 captcha.db，需在控制台重新初始化与创建），可手动删除", path)
		}
	}
}
