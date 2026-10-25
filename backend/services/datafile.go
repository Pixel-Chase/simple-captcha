package services

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// 管理后台的持久化数据（管理员凭据、接入应用）以 JSON 文件保存在数据目录（工作目录下的 data）。
// 只考虑单实例部署，不引入数据库；文件内容只含哈希，权限仍收紧为仅属主可读写。
const dataDirMode = 0o700

// ensureDataDir 创建数据目录并确认可写，部署时挂载卷权限不对能在启动阶段直接暴露
func ensureDataDir(dir string) error {
	if err := os.MkdirAll(dir, dataDirMode); err != nil {
		return fmt.Errorf("创建数据目录 %s 失败: %w", dir, err)
	}
	probe, err := os.CreateTemp(dir, ".probe-*")
	if err != nil {
		return fmt.Errorf("数据目录 %s 不可写: %w", dir, err)
	}
	name := probe.Name()
	_ = probe.Close()
	_ = os.Remove(name)
	return nil
}

// readJSONFile 读取并解析 JSON 文件；文件不存在时返回 (false, nil)
func readJSONFile(path string, v interface{}) (bool, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if err := json.Unmarshal(data, v); err != nil {
		return true, fmt.Errorf("解析 %s 失败: %w", path, err)
	}
	return true, nil
}

// writeJSONFile 先写同目录临时文件再原子重命名，读者只会看到完整的旧文件或新文件
func writeJSONFile(path string, v interface{}) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}

	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpPath) }

	if _, err := tmp.Write(append(data, '\n')); err != nil {
		_ = tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return err
	}
	// os.CreateTemp 创建的文件即为 0600，重命名后权限保持不变
	if err := os.Rename(tmpPath, path); err != nil {
		cleanup()
		return err
	}
	return nil
}
